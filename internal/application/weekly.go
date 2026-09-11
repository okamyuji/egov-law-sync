package application

import (
	"context"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

// WeeklyOptions 週次の実行時オプション
type WeeklyOptions struct {
	ReleaseTag string
	XMLDir     string
	ZipPath    string
}

// WeeklyChecker 週次のsec1 zipとの照合。異常判定は持たず、xml_index.csvを直接直す
type WeeklyChecker interface {
	Run(ctx context.Context, o WeeklyOptions) (Result, error)
}

type weeklyChecker struct {
	d Deps
}

// NewWeeklyChecker Depsを束ねたWeeklyCheckerを返す
func NewWeeklyChecker(d Deps) WeeklyChecker {
	return &weeklyChecker{d: d}
}

func (w *weeklyChecker) Run(ctx context.Context, o WeeklyOptions) (Result, error) {
	start := w.d.Clock.Now()
	rec := newRecord("weekly", start)

	laws, err := w.d.Repo.LoadLaws()
	if err != nil {
		return Result{}, err
	}
	index, err := w.d.Repo.LoadXMLIndex()
	if err != nil {
		return Result{}, err
	}
	archive, err := w.d.Bulk.FetchAll(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = archive.Close() }()

	mismatched, err := w.compare(archive, laws, index, &rec)
	if err != nil {
		return Result{}, err
	}
	countZipOnly(archive, laws, &rec)
	if o.ReleaseTag != "" {
		fetched := w.repair(ctx, mismatched, index, o, &rec)
		if err := w.d.Repo.SaveXMLIndex(index); err != nil {
			return Result{}, err
		}
		bundleXML(w.d, o.XMLDir, o.ZipPath, fetched, &rec)
	}
	rec.Applied = true
	return saveRun(w.d, rec, start, 0)
}

// compare 手順2。廃止と失効の行とxml_index.csvに無い行は飛ばす
func (w *weeklyChecker) compare(archive Archive, laws []law.Law, index map[law.RevisionID]law.XMLRecord, rec *RunRecord) ([]law.Law, error) {
	mismatched := make([]law.Law, 0)
	for _, l := range laws {
		known, hasIndex := index[l.RevisionID]
		if l.RepealStatus != "None" || !hasIndex {
			continue
		}
		sum, ok, err := archive.SHA256(l.RevisionID)
		if err != nil {
			return nil, err
		}
		if !ok {
			rec.Counts["missing_in_zip"]++
			continue
		}
		if sum != known.SHA256 {
			mismatched = append(mismatched, l)
		}
	}
	return mismatched, nil
}

// countZipOnly zipにあってlaws.csvに無いrevisionの件数
func countZipOnly(archive Archive, laws []law.Law, rec *RunRecord) {
	current := make(map[law.RevisionID]bool, len(laws))
	for _, l := range laws {
		current[l.RevisionID] = true
	}
	for _, id := range archive.RevisionIDs() {
		if !current[id] {
			rec.Counts["zip_only"]++
		}
	}
}

// repair 手順3。取り直したsha256が今の値と同じならzipが古いだけなので件数だけ数える
func (w *weeklyChecker) repair(ctx context.Context, mismatched []law.Law, index map[law.RevisionID]law.XMLRecord, o WeeklyOptions, rec *RunRecord) []law.XMLRecord {
	fetched := make([]law.XMLRecord, 0, len(mismatched))
	for _, l := range mismatched {
		sum, n, err := w.d.XML.FetchXML(ctx, l.RevisionID, o.XMLDir)
		if err != nil {
			rec.Counts["xml_failed"]++
			continue
		}
		if sum == index[l.RevisionID].SHA256 {
			rec.Counts["zip_stale"]++
			continue
		}
		repaired := law.XMLRecord{RevisionID: l.RevisionID, Updated: l.Updated, SHA256: sum, Bytes: n, ReleaseTag: o.ReleaseTag}
		index[l.RevisionID] = repaired
		fetched = append(fetched, repaired)
		rec.Bytes += n
		rec.Counts["repaired"]++
	}
	return fetched
}
