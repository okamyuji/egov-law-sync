package application

import (
	"cmp"
	"context"
	"log"
	"slices"
	"time"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
	"github.com/okamyuji/egov-law-sync/internal/domain/sync"
)

// Deps 3つのユースケースが共有するport一式
type Deps struct {
	Catalog     LawCatalog
	Revisions   RevisionSource
	XML         XMLSource
	Updates     UpdateList
	Daily       DailyArchive
	Bulk        BulkArchive
	Repo        ManifestRepository
	Bundler     ReleaseBundler
	Text        TextRenderer
	Source      ChunkSource
	Sink        ChunkSink
	Clock       Clock
	Threshold   sync.Thresholds
	Concurrency int
}

// workers 本文取得の並列度。0以下は1として扱う
func (d Deps) workers() int {
	if d.Concurrency < 1 {
		return 1
	}
	return d.Concurrency
}

// newRecord Counts付きの空のRunRecordを作る。StartedAtはUTC、DateJSTはJSTの暦日
func newRecord(kind string, start time.Time) RunRecord {
	return RunRecord{
		Kind:      kind,
		StartedAt: start.UTC().Format(time.RFC3339),
		DateJST:   law.DateOf(start).String(),
		Counts:    map[string]int{},
	}
}

// saveRun 経過秒を詰めてruns/へ書き、Resultにする
func saveRun(d Deps, rec RunRecord, start time.Time, code int) (Result, error) {
	rec.Seconds = d.Clock.Now().Sub(start).Seconds()
	if err := d.Repo.SaveRun(rec); err != nil {
		return Result{}, err
	}
	return Result{ExitCode: code, Record: rec}, nil
}

// maxFetchLogLines 1回の実行でstderrに出す取得失敗の行数。原因の把握には先頭だけで足り、全件出すとログが読めなくなる
const maxFetchLogLines = 10

// fetchLog 取得失敗の原因をstderrへ出す。1回の実行で1つ作る
type fetchLog struct {
	n int
}

// add 失敗を1件記録する。上限を超えた分は件数だけ数える
func (f *fetchLog) add(kind, id string, err error) {
	f.n++
	if f.n <= maxFetchLogLines {
		log.Printf("%s fetch failed: %s: %v", kind, id, err)
	}
}

// flush 出さなかった件数を1行にまとめる。実行の終わりに必ず呼ぶ
func (f *fetchLog) flush() {
	if f.n > maxFetchLogLines {
		log.Printf("...and %d more fetch errors", f.n-maxFetchLogLines)
	}
}

// warnXMLFailures 本文取得の失敗が閾値を超えたら警告にする
func warnXMLFailures(d Deps, rec *RunRecord) {
	if rec.Counts["xml_failed"] > d.Threshold.MaxFetchFailures {
		addWarning(rec, "xml_failures")
	}
}

// addWarning 同じ理由を重ねて書かない
func addWarning(rec *RunRecord, w string) {
	if !slices.Contains(rec.Warnings, w) {
		rec.Warnings = append(rec.Warnings, w)
	}
}

// fetchRevisions targetsの/law_revisionsを引いてdstにマージする。失敗はpending_law_idsに残して次回に引き直す。404の法令はAPIから消えているので引き直さない
func fetchRevisions(ctx context.Context, src RevisionSource, targets []law.LawID, dst map[law.RevisionID]law.Revision, today law.Date, rec *RunRecord, lg *fetchLog) {
	for _, id := range targets {
		fetched, found, err := src.Revisions(ctx, id)
		if err != nil {
			rec.PendingLawIDs = append(rec.PendingLawIDs, id)
			addWarning(rec, "revision_fetch_failed")
			lg.add("revision", string(id), err)
			continue
		}
		if !found {
			rec.Counts["revision_gone"]++
			continue
		}
		for _, r := range fetched {
			r.FirstSeen = today.String()
			if old, ok := dst[r.ID]; ok && old.FirstSeen != "" {
				r.FirstSeen = old.FirstSeen
			}
			dst[r.ID] = r
		}
	}
}

type xmlOutcome struct {
	rec law.XMLRecord
	id  law.RevisionID
	err error
}

// applyXML targetsの本文を取ってindexを置き換え、取得できた分を返す
func applyXML(ctx context.Context, d Deps, targets []law.Law, dir, tag string, index map[law.RevisionID]law.XMLRecord, rec *RunRecord, lg *fetchLog) []law.XMLRecord {
	fetched := make([]law.XMLRecord, 0, len(targets))
	if len(targets) == 0 {
		return fetched
	}
	sem := make(chan struct{}, d.workers())
	out := make(chan xmlOutcome, len(targets))
	for _, t := range targets {
		sem <- struct{}{}
		go func(t law.Law) {
			defer func() { <-sem }()
			sum, n, err := d.XML.FetchXML(ctx, t.RevisionID, dir)
			if err != nil {
				out <- xmlOutcome{id: t.RevisionID, err: err}
				return
			}
			out <- xmlOutcome{rec: law.XMLRecord{RevisionID: t.RevisionID, Updated: t.Updated, SHA256: sum, Bytes: n, ReleaseTag: tag}, id: t.RevisionID}
		}(t)
	}
	for range targets {
		o := <-out
		if o.err != nil {
			rec.Counts["xml_failed"]++
			// 記録側に排他を持たせていないので、取得のgoroutineからではなくこの直列ループで記録する
			lg.add("xml", string(o.id), o.err)
			continue
		}
		fetched = append(fetched, o.rec)
		index[o.rec.RevisionID] = o.rec
		rec.Bytes += o.rec.Bytes
		progress("xml", len(fetched)+rec.Counts["xml_failed"], len(targets))
	}
	rec.Counts["xml_ok"] += len(fetched)
	return fetched
}

// bundleXML 今回取得した本文をzipにまとめる。zipは正本ではないので、失敗しても警告にとどめて実行は成功させる
func bundleXML(d Deps, xmlDir, zipPath string, fetched []law.XMLRecord, rec *RunRecord) {
	if zipPath == "" || len(fetched) == 0 {
		return
	}
	index := slices.Clone(fetched)
	slices.SortFunc(index, func(a, b law.XMLRecord) int { return cmp.Compare(a.RevisionID, b.RevisionID) })
	if err := d.Bundler.Bundle(xmlDir, zipPath, index); err != nil {
		addWarning(rec, "bundle_failed")
	}
}
