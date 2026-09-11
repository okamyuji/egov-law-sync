package application

import (
	"context"
	"errors"
	"maps"
	"slices"
	stdsync "sync"
	"time"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
	"github.com/okamyuji/egov-law-sync/internal/domain/sync"
)

type fakeCatalog struct {
	current     []law.Law
	future      []law.Law
	total       int
	futureTotal int
	err         error
	calls       int
}

func (f *fakeCatalog) ListAll(_ context.Context, asof string) ([]law.Law, int, error) {
	f.calls++
	if f.err != nil {
		return nil, 0, f.err
	}
	if asof != "" {
		if f.futureTotal != 0 {
			return f.future, f.futureTotal, nil
		}
		return f.future, len(f.future), nil
	}
	total := f.total
	if total == 0 {
		total = len(f.current)
	}
	return f.current, total, nil
}

type fakeRevisions struct {
	byLaw map[law.LawID][]law.Revision
	errs  map[law.LawID]error
	gone  map[law.LawID]bool
	calls map[law.LawID]int
}

func (f *fakeRevisions) Revisions(_ context.Context, id law.LawID) ([]law.Revision, bool, error) {
	f.calls[id]++
	if err := f.errs[id]; err != nil {
		return nil, false, err
	}
	if f.gone[id] {
		return nil, false, nil
	}
	return f.byLaw[id], true, nil
}

type fakeXML struct {
	mu    stdsync.Mutex
	sha   map[law.RevisionID]string
	errs  map[law.RevisionID]error
	calls int
}

func (f *fakeXML) FetchXML(_ context.Context, id law.RevisionID, _ string) (string, int64, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if err := f.errs[id]; err != nil {
		return "", 0, err
	}
	sum, ok := f.sha[id]
	if !ok {
		return "", 0, errors.New("xml not found: " + string(id))
	}
	return sum, int64(len(sum)), nil
}

type fakeUpdates struct {
	byDate   map[string][]law.LawID
	notFound bool
	err      error
	calls    []string
}

func (f *fakeUpdates) UpdatedLawIDs(_ context.Context, d law.Date) ([]law.LawID, bool, error) {
	f.calls = append(f.calls, d.String())
	if f.err != nil {
		return nil, false, f.err
	}
	ids, ok := f.byDate[d.String()]
	if f.notFound || !ok {
		return nil, false, nil
	}
	return ids, true, nil
}

type fakeDailyArchive struct {
	dirs  map[string][]string
	err   error
	calls []string
}

func (f *fakeDailyArchive) RevisionDirs(_ context.Context, d law.Date) ([]string, bool, error) {
	f.calls = append(f.calls, d.String())
	if f.err != nil {
		return nil, false, f.err
	}
	dirs, ok := f.dirs[d.String()]
	if !ok {
		return nil, false, nil
	}
	return dirs, true, nil
}

type fakeBulk struct {
	sha     map[law.RevisionID]string
	lookups map[law.RevisionID]int
	err     error
	closed  bool
}

func (f *fakeBulk) FetchAll(_ context.Context) (Archive, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f, nil
}

func (f *fakeBulk) SHA256(id law.RevisionID) (string, bool, error) {
	f.lookups[id]++
	sum, ok := f.sha[id]
	return sum, ok, nil
}

func (f *fakeBulk) RevisionIDs() []law.RevisionID {
	ids := slices.Collect(maps.Keys(f.sha))
	slices.Sort(ids)
	return ids
}

type bundleCall struct {
	xmlDir  string
	outPath string
	index   []law.XMLRecord
}

type textCall struct {
	textDir string
	outPath string
	index   []law.TextRecord
}

type fakeBundler struct {
	calls     []bundleCall
	textCalls []textCall
	err       error
	textErr   error
}

func (f *fakeBundler) BundleText(textDir, outPath string, index []law.TextRecord) error {
	f.textCalls = append(f.textCalls, textCall{textDir: textDir, outPath: outPath, index: index})
	return f.textErr
}

func (f *fakeBundler) Bundle(xmlDir, outPath string, index []law.XMLRecord) error {
	f.calls = append(f.calls, bundleCall{xmlDir: xmlDir, outPath: outPath, index: index})
	return f.err
}

func (f *fakeBulk) Close() error {
	f.closed = true
	return nil
}

type fakeRepo struct {
	laws        []law.Law
	revisions   map[law.RevisionID]law.Revision
	xml         map[law.RevisionID]law.XMLRecord
	runs        []RunRecord
	savedLaws   bool
	savedXML    bool
	bootstrap   *RunRecord
	lastApplied *RunRecord
	lastDaily   *RunRecord
}

func (f *fakeRepo) LoadLaws() ([]law.Law, error) {
	return append([]law.Law(nil), f.laws...), nil
}

func (f *fakeRepo) SaveLaws(l []law.Law) error {
	f.savedLaws = true
	f.laws = l
	return nil
}

func (f *fakeRepo) LoadRevisions() (map[law.RevisionID]law.Revision, error) {
	out := make(map[law.RevisionID]law.Revision, len(f.revisions))
	maps.Copy(out, f.revisions)
	return out, nil
}

func (f *fakeRepo) SaveRevisions(m map[law.RevisionID]law.Revision) error {
	f.revisions = m
	return nil
}

func (f *fakeRepo) LoadXMLIndex() (map[law.RevisionID]law.XMLRecord, error) {
	out := make(map[law.RevisionID]law.XMLRecord, len(f.xml))
	maps.Copy(out, f.xml)
	return out, nil
}

func (f *fakeRepo) SaveXMLIndex(m map[law.RevisionID]law.XMLRecord) error {
	f.savedXML = true
	f.xml = m
	return nil
}

func (f *fakeRepo) SaveRun(r RunRecord) error {
	f.runs = append(f.runs, r)
	return nil
}

func (f *fakeRepo) LastAppliedDaily() (RunRecord, bool, error) {
	return deref(f.lastApplied)
}

func (f *fakeRepo) LastBootstrap() (RunRecord, bool, error) {
	return deref(f.bootstrap)
}

func (f *fakeRepo) LastDaily() (RunRecord, bool, error) {
	return deref(f.lastDaily)
}

func deref(r *RunRecord) (RunRecord, bool, error) {
	if r == nil {
		return RunRecord{}, false, nil
	}
	return *r, true, nil
}

type fakeClock struct {
	now time.Time
}

func (f *fakeClock) Now() time.Time {
	if f.now.IsZero() {
		return time.Date(2026, 9, 12, 0, 0, 0, 0, law.JST)
	}
	return f.now
}

type fakes struct {
	catalog   *fakeCatalog
	revisions *fakeRevisions
	xml       *fakeXML
	updates   *fakeUpdates
	daily     *fakeDailyArchive
	bulk      *fakeBulk
	repo      *fakeRepo
	bundler   *fakeBundler
	clock     *fakeClock
}

func newFakes() *fakes {
	return &fakes{
		catalog:   &fakeCatalog{},
		revisions: &fakeRevisions{byLaw: map[law.LawID][]law.Revision{}, errs: map[law.LawID]error{}, gone: map[law.LawID]bool{}, calls: map[law.LawID]int{}},
		xml:       &fakeXML{sha: map[law.RevisionID]string{}, errs: map[law.RevisionID]error{}},
		updates:   &fakeUpdates{byDate: map[string][]law.LawID{}},
		daily:     &fakeDailyArchive{dirs: map[string][]string{}},
		bulk:      &fakeBulk{sha: map[law.RevisionID]string{}, lookups: map[law.RevisionID]int{}},
		repo:      &fakeRepo{revisions: map[law.RevisionID]law.Revision{}, xml: map[law.RevisionID]law.XMLRecord{}},
		bundler:   &fakeBundler{},
		clock:     &fakeClock{},
	}
}

func (f *fakes) deps() Deps {
	return Deps{
		Catalog:     f.catalog,
		Revisions:   f.revisions,
		XML:         f.xml,
		Updates:     f.updates,
		Daily:       f.daily,
		Bulk:        f.bulk,
		Repo:        f.repo,
		Bundler:     f.bundler,
		Clock:       f.clock,
		Threshold:   sync.DefaultThresholds(),
		Concurrency: 1,
	}
}
