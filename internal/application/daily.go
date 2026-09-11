package application

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
	"github.com/okamyuji/egov-law-sync/internal/domain/sync"
)

// sec3LookbackDays sec3 zipが置かれている期間。これを過ぎた404日は照合しない
const sec3LookbackDays = 90

// ErrNoBaseline bootstrapも適用済みの日次も無く、対象日の開始を決められない
var ErrNoBaseline = errors.New("application: no bootstrap or applied daily run")

// ErrInvalidRange --fromが--toより後で、対象日の範囲にならない
var ErrInvalidRange = errors.New("application: from is after to")

// ErrInvalidStoredDate runs/の日付が壊れている。そのまま起点にすると対象日の範囲が数十万日に広がる
var ErrInvalidStoredDate = errors.New("application: stored date in runs/ is not YYYY-MM-DD")

// DailyOptions 日次の実行時オプション。FromとToが空なら自動で決める
type DailyOptions struct {
	From, To   law.Date
	Force      bool
	ReleaseTag string
	XMLDir     string
	ZipPath    string
}

// DailySyncer 日次同期
type DailySyncer interface {
	Run(ctx context.Context, o DailyOptions) (Result, error)
}

type dailySyncer struct {
	d Deps
}

// NewDailySyncer Depsを束ねたDailySyncerを返す
func NewDailySyncer(d Deps) DailySyncer {
	return &dailySyncer{d: d}
}

// dailyRange 対象日の範囲。prevToは前回のto、prevTotalは総件数の比較対象
type dailyRange struct {
	from, to, prevTo law.Date
	prevTotal        int
}

// empty 前日まで適用済みなら真
func (r dailyRange) empty() bool {
	return r.to.Before(r.from)
}

func (s *dailySyncer) Run(ctx context.Context, o DailyOptions) (Result, error) {
	start := s.d.Clock.Now()
	rec := newRecord("daily", start)
	lg := &fetchLog{}
	defer lg.flush()
	rg, err := s.resolveRange(o)
	if err != nil {
		return Result{}, err
	}
	setRange(&rec, rg)

	updated, ok := s.collectUpdated(ctx, rg, &rec)
	if !ok {
		return saveRun(s.d, rec, start, 3)
	}
	cur, total, anomaly := s.fetchCatalog(ctx)
	rec.TotalCount = total
	if anomaly != "" {
		rec.Anomalies = append(rec.Anomalies, anomaly)
		return saveRun(s.d, rec, start, 3)
	}
	prev, err := s.d.Repo.LoadLaws()
	if err != nil {
		return Result{}, err
	}
	revisions, err := s.d.Repo.LoadRevisions()
	if err != nil {
		return Result{}, err
	}
	known := knownRevisions(revisions)
	changes := sync.Classify(prev, cur)
	countChanges(&rec, changes)
	rec.Changes = changes

	targets, err := s.revisionTargets(updated, changes)
	if err != nil {
		return Result{}, err
	}
	fetchRevisions(ctx, s.d.Revisions, targets, revisions, law.DateOf(start), &rec, lg)

	rec.Counts["unexpected"] = sync.CountUnexpected(changes, known)
	if as := sync.CheckAnomalies(rec.Counts["unexpected"], rg.prevTotal, total, s.d.Threshold, o.Force); len(as) > 0 {
		return s.abort(rec, start, as, targets)
	}
	return s.apply(ctx, cur, revisions, o, rec, start, changes, lg)
}

// resolveRange 開始は前回適用済みのtoの翌日、無ければ最新bootstrapの実行日
func (s *dailySyncer) resolveRange(o DailyOptions) (dailyRange, error) {
	var rg dailyRange
	applied, ok, err := s.d.Repo.LastAppliedDaily()
	if err != nil {
		return rg, err
	}
	if ok {
		prevTo, err := law.ParseDate(applied.To)
		if err != nil {
			return rg, ErrInvalidStoredDate
		}
		rg.prevTo = prevTo
		rg.from = prevTo.Add(1)
		rg.prevTotal = applied.TotalCount
	} else {
		bs, found, err := s.d.Repo.LastBootstrap()
		if err != nil {
			return rg, err
		}
		if !found {
			return rg, ErrNoBaseline
		}
		from, err := law.ParseDate(bs.DateJST)
		if err != nil {
			return rg, ErrInvalidStoredDate
		}
		rg.from = from
		rg.prevTo = from
		rg.prevTotal = bs.TotalCount
	}
	rg.to = law.DateOf(s.d.Clock.Now()).Add(-1)
	if o.From != "" {
		rg.from = o.From
	}
	if o.To != "" {
		rg.to = o.To
	}
	if o.From != "" && o.To != "" && o.To.Before(o.From) {
		return rg, ErrInvalidRange
	}
	return rg, nil
}

// setRange 範囲が空の実行はfromとtoに前回のtoをそのまま書く
func setRange(rec *RunRecord, rg dailyRange) {
	if rg.empty() {
		rec.From, rec.To = rg.prevTo.String(), rg.prevTo.String()
		return
	}
	rec.From, rec.To = rg.from.String(), rg.to.String()
}

// collectUpdated 手順1。v1が404以外で失敗したら異常として偽を返す
func (s *dailySyncer) collectUpdated(ctx context.Context, rg dailyRange, rec *RunRecord) (map[law.LawID]bool, bool) {
	ids := map[law.LawID]bool{}
	if rg.empty() {
		return ids, true
	}
	limit := law.DateOf(s.d.Clock.Now()).Add(-sec3LookbackDays)
	for _, d := range sync.V1Range(rg.from, rg.to) {
		updated, found, err := s.d.Updates.UpdatedLawIDs(ctx, d)
		if err != nil {
			rec.Anomalies = append(rec.Anomalies, "v1_unavailable")
			return nil, false
		}
		if found {
			for _, id := range updated {
				ids[id] = true
			}
			continue
		}
		if !d.Before(limit) {
			s.crossCheckSec3(ctx, d, ids, rec)
		}
	}
	return ids, true
}

// crossCheckSec3 v1が404の日をsec3 zipと突き合わせる
func (s *dailySyncer) crossCheckSec3(ctx context.Context, d law.Date, ids map[law.LawID]bool, rec *RunRecord) {
	dirs, found, err := s.d.Daily.RevisionDirs(ctx, d)
	if err != nil {
		addWarning(rec, "sec3_unavailable")
		return
	}
	if !found || len(dirs) == 0 {
		return
	}
	addWarning(rec, "sec3_has_updates_v1_missing")
	for _, dir := range dirs {
		ids[law.LawID(strings.Split(dir, "_")[0])] = true
	}
}

// fetchCatalog 手順2。失敗や件数不一致はもう1回だけ取り直す。戻り値の文字列が空でなければ異常
func (s *dailySyncer) fetchCatalog(ctx context.Context) ([]law.Law, int, string) {
	var laws []law.Law
	var total int
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		laws, total, err = s.d.Catalog.ListAll(ctx, "")
		if err == nil && len(laws) == total {
			return laws, total, ""
		}
	}
	if err != nil {
		return nil, 0, "catalog_unavailable"
	}
	return nil, total, "count_mismatch"
}

// knownRevisions 実行開始時のrevisions.csvの鍵集合。同じ実行で引いた分は想定内に含めない
func knownRevisions(revisions map[law.RevisionID]law.Revision) map[law.RevisionID]bool {
	known := make(map[law.RevisionID]bool, len(revisions))
	for id := range revisions {
		known[id] = true
	}
	return known
}

func countChanges(rec *RunRecord, changes []sync.Change) {
	for _, c := range changes {
		rec.Counts[string(c.Kind)]++
	}
}

// revisionTargets 手順4の対象。v1の法令ID、新規と切替と再登録、前回のpending_law_ids
func (s *dailySyncer) revisionTargets(updated map[law.LawID]bool, changes []sync.Change) ([]law.LawID, error) {
	set := make(map[law.LawID]bool, len(updated))
	for id := range updated {
		set[id] = true
	}
	for _, c := range changes {
		if c.Kind != sync.Removed {
			set[c.LawID] = true
		}
	}
	pending, err := s.pendingLawIDs()
	if err != nil {
		return nil, err
	}
	for _, id := range pending {
		set[id] = true
	}
	targets := make([]law.LawID, 0, len(set))
	for id := range set {
		targets = append(targets, id)
	}
	slices.Sort(targets)
	return targets, nil
}

// pendingLawIDs 引き直す法令ID。日次とbootstrapのうちStartedAtが後の記録を採る。bootstrapの失敗分も次の日次が拾う
func (s *dailySyncer) pendingLawIDs() ([]law.LawID, error) {
	daily, hasDaily, err := s.d.Repo.LastDaily()
	if err != nil {
		return nil, err
	}
	bs, hasBootstrap, err := s.d.Repo.LastBootstrap()
	if err != nil {
		return nil, err
	}
	if hasBootstrap && (!hasDaily || daily.StartedAt < bs.StartedAt) {
		return bs.PendingLawIDs, nil
	}
	if hasDaily {
		return daily.PendingLawIDs, nil
	}
	return nil, nil
}

// abort 手順5の異常。3つのCSVを書かず、引いた法令はすべてpending_law_idsに残す
func (s *dailySyncer) abort(rec RunRecord, start time.Time, as []sync.Anomaly, targets []law.LawID) (Result, error) {
	for _, a := range as {
		rec.Anomalies = append(rec.Anomalies, string(a))
	}
	rec.PendingLawIDs = targets
	rec.Applied = false
	return saveRun(s.d, rec, start, 3)
}

// apply 手順6から8。ReleaseTagが空なら本文取得とxml_index.csvの書き込みを飛ばす
func (s *dailySyncer) apply(ctx context.Context, cur []law.Law, revisions map[law.RevisionID]law.Revision, o DailyOptions, rec RunRecord, start time.Time, changes []sync.Change, lg *fetchLog) (Result, error) {
	var index map[law.RevisionID]law.XMLRecord
	var fetched []law.XMLRecord
	if o.ReleaseTag != "" {
		loaded, err := s.d.Repo.LoadXMLIndex()
		if err != nil {
			return Result{}, err
		}
		index = loaded
		fetched = applyXML(ctx, s.d, sync.XMLTargets(cur, index), o.XMLDir, o.ReleaseTag, index, &rec, lg)
		warnXMLFailures(s.d, &rec)
	}
	if err := s.d.Repo.SaveLaws(cur); err != nil {
		return Result{}, err
	}
	if err := s.d.Repo.SaveRevisions(revisions); err != nil {
		return Result{}, err
	}
	if index != nil {
		if err := s.d.Repo.SaveXMLIndex(index); err != nil {
			return Result{}, err
		}
	}
	bundleXML(s.d, o.XMLDir, o.ZipPath, fetched, &rec)
	rec.Applied = true
	res, err := saveRun(s.d, rec, start, 0)
	if err != nil {
		return Result{}, err
	}
	notifyChanges(changes)
	return res, nil
}

// notifyChanges 差分通知の呼び出し位置。初回スコープでは何もしない。
// 依頼者の指示により、Slack Incoming Webhookを使う場合の形をここに残す。
// Webhook URLはSecretのSLACK_WEBHOOK_URLに置く。
// 差分があった日は、law_id、法令名、施行日、未施行の有無を1メッセージで投稿する。
// 異常や警告のIssueを作った日は、そのIssueのURLを投稿する。
func notifyChanges(changes []sync.Change) {
	_ = changes
}
