package application

import (
	"cmp"
	"context"
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

// addWarning 同じ理由を重ねて書かない
func addWarning(rec *RunRecord, w string) {
	if !slices.Contains(rec.Warnings, w) {
		rec.Warnings = append(rec.Warnings, w)
	}
}

// fetchRevisions targetsの/law_revisionsを引いてdstにマージする。失敗はpending_law_idsに残して次回に引き直す
func fetchRevisions(ctx context.Context, src RevisionSource, targets []law.LawID, dst map[law.RevisionID]law.Revision, today law.Date, rec *RunRecord) {
	for _, id := range targets {
		fetched, err := src.Revisions(ctx, id)
		if err != nil {
			rec.PendingLawIDs = append(rec.PendingLawIDs, id)
			addWarning(rec, "revision_fetch_failed")
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
	ok  bool
}

// applyXML targetsの本文を取ってindexを置き換え、取得できた分を返す
func applyXML(ctx context.Context, d Deps, targets []law.Law, dir, tag string, index map[law.RevisionID]law.XMLRecord, rec *RunRecord) []law.XMLRecord {
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
				out <- xmlOutcome{}
				return
			}
			out <- xmlOutcome{rec: law.XMLRecord{RevisionID: t.RevisionID, Updated: t.Updated, SHA256: sum, Bytes: n, ReleaseTag: tag}, ok: true}
		}(t)
	}
	for range targets {
		o := <-out
		if !o.ok {
			rec.Counts["xml_failed"]++
			continue
		}
		fetched = append(fetched, o.rec)
		index[o.rec.RevisionID] = o.rec
		rec.Bytes += o.rec.Bytes
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
