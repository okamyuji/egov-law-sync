package application

import (
	"context"
	"time"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

// futureAsof 未施行改正を持つ法令を洗い出すための十分に遠い日付
const futureAsof = "2099-12-31"

// BootstrapOptions bootstrapの実行時オプション
type BootstrapOptions struct {
	ReleaseTag string
	XMLDir     string
	ZipPath    string
}

// Bootstrapper 初回構築。再実行しても既存のCSVにマージする
type Bootstrapper interface {
	Run(ctx context.Context, o BootstrapOptions) (Result, error)
}

type bootstrapper struct {
	d Deps
}

// NewBootstrapper Depsを束ねたBootstrapperを返す
func NewBootstrapper(d Deps) Bootstrapper {
	return &bootstrapper{d: d}
}

func (b *bootstrapper) Run(ctx context.Context, o BootstrapOptions) (Result, error) {
	start := b.d.Clock.Now()
	rec := newRecord("bootstrap", start)
	lg := &fetchLog{}
	defer lg.flush()

	laws, total, err := b.d.Catalog.ListAll(ctx, "")
	if err != nil {
		return Result{}, err
	}
	rec.TotalCount = total
	if len(laws) != total {
		return b.countMismatch(rec, start)
	}
	targets, complete, err := b.futureTargets(ctx, laws)
	if err != nil {
		return Result{}, err
	}
	if !complete {
		return b.countMismatch(rec, start)
	}
	revisions, err := b.d.Repo.LoadRevisions()
	if err != nil {
		return Result{}, err
	}
	fetchRevisions(ctx, b.d.Revisions, targets, revisions, law.DateOf(start), &rec, lg)

	index, fetched, err := b.collectXML(ctx, laws, o, &rec, lg)
	if err != nil {
		return Result{}, err
	}
	return b.persist(laws, revisions, index, fetched, o, rec, start)
}

// countMismatch 3つのCSVを書かずにruns/だけ残す
func (b *bootstrapper) countMismatch(rec RunRecord, start time.Time) (Result, error) {
	rec.Anomalies = append(rec.Anomalies, "count_mismatch")
	return saveRun(b.d, rec, start, 3)
}

// futureTargets asof指定の一覧と現在の一覧でrevision_idが異なる法令。第2戻り値は一覧が全件揃っているか
func (b *bootstrapper) futureTargets(ctx context.Context, laws []law.Law) ([]law.LawID, bool, error) {
	future, total, err := b.d.Catalog.ListAll(ctx, futureAsof)
	if err != nil {
		return nil, false, err
	}
	if len(future) != total {
		return nil, false, nil
	}
	current := make(map[law.LawID]law.RevisionID, len(laws))
	for _, l := range laws {
		current[l.ID] = l.RevisionID
	}
	targets := make([]law.LawID, 0)
	for _, fl := range future {
		if cur, ok := current[fl.ID]; ok && cur != fl.RevisionID {
			targets = append(targets, fl.ID)
		}
	}
	return targets, true, nil
}

// collectXML ReleaseTagが空なら取得もxml_index.csvの読み込みもしない
func (b *bootstrapper) collectXML(ctx context.Context, laws []law.Law, o BootstrapOptions, rec *RunRecord, lg *fetchLog) (map[law.RevisionID]law.XMLRecord, []law.XMLRecord, error) {
	if o.ReleaseTag == "" {
		return nil, nil, nil
	}
	index, err := b.d.Repo.LoadXMLIndex()
	if err != nil {
		return nil, nil, err
	}
	fetched := applyXML(ctx, b.d, laws, o.XMLDir, o.ReleaseTag, index, rec, lg)
	warnXMLFailures(b.d, rec)
	return index, fetched, nil
}

func (b *bootstrapper) persist(laws []law.Law, revisions map[law.RevisionID]law.Revision, index map[law.RevisionID]law.XMLRecord, fetched []law.XMLRecord, o BootstrapOptions, rec RunRecord, start time.Time) (Result, error) {
	if err := b.d.Repo.SaveLaws(laws); err != nil {
		return Result{}, err
	}
	if err := b.d.Repo.SaveRevisions(revisions); err != nil {
		return Result{}, err
	}
	if o.ReleaseTag != "" {
		if err := b.d.Repo.SaveXMLIndex(index); err != nil {
			return Result{}, err
		}
	}
	bundleXML(b.d, o.XMLDir, o.ZipPath, fetched, &rec)
	rec.Applied = true
	return saveRun(b.d, rec, start, 0)
}
