package application

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
	"github.com/okamyuji/egov-law-sync/internal/domain/sync"
)

func TestDailyAppliesKnownSwitchAndFetchesXML(t *testing.T) {
	f := newFakes()
	f.clock.now = time.Date(2026, 9, 12, 0, 0, 0, 0, law.JST)
	f.repo.laws = []law.Law{{ID: "A", RevisionID: "A_1", Updated: "u1"}}
	f.repo.revisions = map[law.RevisionID]law.Revision{"A_2": {ID: "A_2", LawID: "A", FirstSeen: "2026-09-01"}}
	f.repo.xml = map[law.RevisionID]law.XMLRecord{"A_1": {RevisionID: "A_1", Updated: "u1", ReleaseTag: "b"}}
	f.repo.bootstrap = &RunRecord{Kind: "bootstrap", DateJST: "2026-09-10", TotalCount: 1, Applied: true}
	f.catalog.current = []law.Law{{ID: "A", RevisionID: "A_2", Updated: "u2"}}
	f.updates.notFound = true
	f.revisions.byLaw["A"] = []law.Revision{{ID: "A_1", LawID: "A"}, {ID: "A_2", LawID: "A"}}
	f.xml.sha["A_2"] = "sha2"
	res, err := NewDailySyncer(f.deps()).Run(context.Background(), DailyOptions{ReleaseTag: "sync-1", XMLDir: t.TempDir()})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if res.Record.From != "2026-09-10" || res.Record.To != "2026-09-11" || !res.Record.Applied {
		t.Fatalf("record=%+v", res.Record)
	}
	if res.Record.Counts["unexpected"] != 0 || f.repo.xml["A_2"].SHA256 != "sha2" || f.repo.xml["A_2"].ReleaseTag != "sync-1" {
		t.Fatalf("counts=%v xml=%+v", res.Record.Counts, f.repo.xml["A_2"])
	}
	if len(f.updates.calls) != 4 || f.updates.calls[0] != "2026-09-08" {
		t.Fatalf("v1 calls=%v", f.updates.calls)
	}
	want := sync.Change{LawID: "A", Kind: sync.Switched, OldRevision: "A_1", NewRevision: "A_2"}
	if len(res.Record.Changes) != 1 || res.Record.Changes[0] != want {
		t.Fatalf("changes=%+v", res.Record.Changes)
	}
}

func TestDailyUnexpectedOverThresholdIsAnomaly(t *testing.T) {
	f := newFakes()
	f.clock.now = time.Date(2026, 9, 12, 0, 0, 0, 0, law.JST)
	f.repo.bootstrap = &RunRecord{Kind: "bootstrap", DateJST: "2026-09-11", TotalCount: 201, Applied: true}
	f.updates.notFound = true
	for i := range 201 {
		id := fmt.Sprintf("L%03d", i)
		f.repo.laws = append(f.repo.laws, law.Law{ID: law.LawID(id), RevisionID: law.RevisionID(id + "_1"), Updated: "u1"})
		f.catalog.current = append(f.catalog.current, law.Law{ID: law.LawID(id), RevisionID: law.RevisionID(id + "_1"), Updated: "u2"})
	}
	res, _ := NewDailySyncer(f.deps()).Run(context.Background(), DailyOptions{ReleaseTag: "t", XMLDir: t.TempDir()})
	if res.ExitCode != 3 || f.repo.savedLaws || f.xml.calls != 0 {
		t.Fatalf("res=%+v savedLaws=%v xmlCalls=%d", res.ExitCode, f.repo.savedLaws, f.xml.calls)
	}
	if len(f.repo.runs) != 1 || f.repo.runs[0].Applied || len(f.repo.runs[0].Changes) != 201 {
		t.Fatalf("runs=%+v", f.repo.runs)
	}
}

func TestDailySwitchToUnknownRevisionCountsAsUnexpected(t *testing.T) {
	f := newFakes()
	f.clock.now = time.Date(2026, 9, 12, 0, 0, 0, 0, law.JST)
	f.repo.bootstrap = &RunRecord{Kind: "bootstrap", DateJST: "2026-09-11", TotalCount: 1, Applied: true}
	f.repo.laws = []law.Law{{ID: "A", RevisionID: "A_1"}}
	f.catalog.current = []law.Law{{ID: "A", RevisionID: "A_2"}}
	f.revisions.byLaw["A"] = []law.Revision{{ID: "A_2", LawID: "A"}}
	f.updates.notFound = true
	res, _ := NewDailySyncer(f.deps()).Run(context.Background(), DailyOptions{ReleaseTag: "t", XMLDir: t.TempDir()})
	if res.Record.Counts["unexpected"] != 1 {
		t.Fatalf("counts=%v", res.Record.Counts)
	}
}

func TestDailyV1ErrorIsAnomaly(t *testing.T) {
	f := newFakes()
	f.clock.now = time.Date(2026, 9, 12, 0, 0, 0, 0, law.JST)
	f.repo.bootstrap = &RunRecord{Kind: "bootstrap", DateJST: "2026-09-11", TotalCount: 0, Applied: true}
	f.updates.err = errors.New("503")
	res, _ := NewDailySyncer(f.deps()).Run(context.Background(), DailyOptions{ReleaseTag: "t", XMLDir: t.TempDir()})
	if res.ExitCode != 3 || f.repo.savedLaws {
		t.Fatalf("res=%+v", res)
	}
}

func TestDailySec3CrossCheckWarns(t *testing.T) {
	f := newFakes()
	f.clock.now = time.Date(2026, 9, 12, 0, 0, 0, 0, law.JST)
	f.repo.bootstrap = &RunRecord{Kind: "bootstrap", DateJST: "2026-09-11", TotalCount: 0, Applied: true}
	f.updates.notFound = true
	f.daily.dirs["2026-09-11"] = []string{"322AC0000000049_20270401_508AC0000000060"}
	f.revisions.byLaw["322AC0000000049"] = []law.Revision{{ID: "322AC0000000049_20270401_508AC0000000060", LawID: "322AC0000000049"}}
	res, _ := NewDailySyncer(f.deps()).Run(context.Background(), DailyOptions{ReleaseTag: "t", XMLDir: t.TempDir()})
	if res.ExitCode != 0 || len(res.Record.Warnings) == 0 || f.revisions.calls["322AC0000000049"] != 1 {
		t.Fatalf("res=%+v calls=%v", res.Record, f.revisions.calls)
	}
}

func TestDailyEmptyReleaseTagSkipsXML(t *testing.T) {
	f := newFakes()
	f.clock.now = time.Date(2026, 9, 12, 0, 0, 0, 0, law.JST)
	f.repo.bootstrap = &RunRecord{Kind: "bootstrap", DateJST: "2026-09-11", TotalCount: 1, Applied: true}
	f.catalog.current = []law.Law{{ID: "A", RevisionID: "A_1", Updated: "u1"}}
	f.updates.notFound = true
	res, _ := NewDailySyncer(f.deps()).Run(context.Background(), DailyOptions{XMLDir: t.TempDir()})
	if res.ExitCode != 0 || f.xml.calls != 0 || len(f.repo.xml) != 0 {
		t.Fatalf("xml must be skipped: calls=%d", f.xml.calls)
	}
}

func TestDailyWithoutBaselineReturnsError(t *testing.T) {
	f := newFakes()
	if _, err := NewDailySyncer(f.deps()).Run(context.Background(), DailyOptions{}); !errors.Is(err, ErrNoBaseline) {
		t.Fatalf("err=%v", err)
	}
}

func TestDailyEmptyRangeSkipsV1AndKeepsPreviousTo(t *testing.T) {
	f := newFakes()
	f.clock.now = time.Date(2026, 9, 12, 0, 0, 0, 0, law.JST)
	f.repo.lastApplied = &RunRecord{Kind: "daily", To: "2026-09-11", TotalCount: 0, Applied: true}
	res, err := NewDailySyncer(f.deps()).Run(context.Background(), DailyOptions{XMLDir: t.TempDir()})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if len(f.updates.calls) != 0 {
		t.Fatalf("v1 must be skipped: %v", f.updates.calls)
	}
	if res.Record.From != "2026-09-11" || res.Record.To != "2026-09-11" {
		t.Fatalf("record=%+v", res.Record)
	}
}

func TestDailyExplicitRangeOverridesAuto(t *testing.T) {
	f := newFakes()
	f.clock.now = time.Date(2026, 9, 12, 0, 0, 0, 0, law.JST)
	f.repo.bootstrap = &RunRecord{Kind: "bootstrap", DateJST: "2026-09-11", Applied: true}
	f.updates.notFound = true
	res, _ := NewDailySyncer(f.deps()).Run(context.Background(), DailyOptions{From: "2026-09-05", To: "2026-09-05", XMLDir: t.TempDir()})
	if res.Record.From != "2026-09-05" || res.Record.To != "2026-09-05" || len(f.updates.calls) != 3 {
		t.Fatalf("record=%+v calls=%v", res.Record, f.updates.calls)
	}
}

func TestDailyCatalogErrorIsAnomaly(t *testing.T) {
	f := newFakes()
	f.clock.now = time.Date(2026, 9, 12, 0, 0, 0, 0, law.JST)
	f.repo.bootstrap = &RunRecord{Kind: "bootstrap", DateJST: "2026-09-11", Applied: true}
	f.updates.notFound = true
	f.catalog.err = errors.New("500")
	res, _ := NewDailySyncer(f.deps()).Run(context.Background(), DailyOptions{XMLDir: t.TempDir()})
	if res.ExitCode != 3 || f.catalog.calls != 2 || res.Record.Anomalies[0] != "catalog_unavailable" {
		t.Fatalf("res=%+v calls=%d", res.Record, f.catalog.calls)
	}
}

func TestDailyCountMismatchRetriesThenIsAnomaly(t *testing.T) {
	f := newFakes()
	f.clock.now = time.Date(2026, 9, 12, 0, 0, 0, 0, law.JST)
	f.repo.bootstrap = &RunRecord{Kind: "bootstrap", DateJST: "2026-09-11", Applied: true}
	f.updates.notFound = true
	f.catalog.current = []law.Law{{ID: "A", RevisionID: "A_1"}}
	f.catalog.total = 5
	res, _ := NewDailySyncer(f.deps()).Run(context.Background(), DailyOptions{XMLDir: t.TempDir()})
	if res.ExitCode != 3 || f.catalog.calls != 2 || res.Record.Anomalies[0] != "count_mismatch" {
		t.Fatalf("res=%+v calls=%d", res.Record, f.catalog.calls)
	}
}

func TestDailyForceAppliesOverThreshold(t *testing.T) {
	f := newFakes()
	f.clock.now = time.Date(2026, 9, 12, 0, 0, 0, 0, law.JST)
	f.repo.bootstrap = &RunRecord{Kind: "bootstrap", DateJST: "2026-09-11", TotalCount: 201, Applied: true}
	f.updates.notFound = true
	for i := range 201 {
		id := fmt.Sprintf("L%03d", i)
		f.repo.laws = append(f.repo.laws, law.Law{ID: law.LawID(id), RevisionID: law.RevisionID(id + "_1"), Updated: "u1"})
		f.catalog.current = append(f.catalog.current, law.Law{ID: law.LawID(id), RevisionID: law.RevisionID(id + "_1"), Updated: "u2"})
	}
	res, _ := NewDailySyncer(f.deps()).Run(context.Background(), DailyOptions{Force: true, XMLDir: t.TempDir()})
	if res.ExitCode != 0 || !f.repo.savedLaws || res.Record.Counts["reregistered"] != 201 {
		t.Fatalf("res=%+v counts=%v", res.ExitCode, res.Record.Counts)
	}
}

func TestDailyRevisionFetchFailureWarnsAndPends(t *testing.T) {
	f := newFakes()
	f.clock.now = time.Date(2026, 9, 12, 0, 0, 0, 0, law.JST)
	f.repo.bootstrap = &RunRecord{Kind: "bootstrap", DateJST: "2026-09-11", Applied: true}
	f.repo.lastDaily = &RunRecord{Kind: "daily", PendingLawIDs: []law.LawID{"Z"}}
	f.updates.notFound = true
	f.revisions.errs["Z"] = errors.New("503")
	res, _ := NewDailySyncer(f.deps()).Run(context.Background(), DailyOptions{XMLDir: t.TempDir()})
	if res.ExitCode != 0 || len(res.Record.PendingLawIDs) != 1 || res.Record.Warnings[0] != "revision_fetch_failed" {
		t.Fatalf("record=%+v", res.Record)
	}
}

func TestDailySec3ErrorWarns(t *testing.T) {
	f := newFakes()
	f.clock.now = time.Date(2026, 9, 12, 0, 0, 0, 0, law.JST)
	f.repo.bootstrap = &RunRecord{Kind: "bootstrap", DateJST: "2026-09-11", Applied: true}
	f.updates.notFound = true
	f.daily.err = errors.New("timeout")
	res, _ := NewDailySyncer(f.deps()).Run(context.Background(), DailyOptions{XMLDir: t.TempDir()})
	if res.ExitCode != 0 || res.Record.Warnings[0] != "sec3_unavailable" {
		t.Fatalf("record=%+v", res.Record)
	}
}

func TestDailyXMLFailuresOverThresholdWarn(t *testing.T) {
	f := newFakes()
	f.clock.now = time.Date(2026, 9, 12, 0, 0, 0, 0, law.JST)
	f.repo.bootstrap = &RunRecord{Kind: "bootstrap", DateJST: "2026-09-11", Applied: true}
	f.updates.notFound = true
	for i := range 21 {
		id := fmt.Sprintf("L%03d", i)
		f.catalog.current = append(f.catalog.current, law.Law{ID: law.LawID(id), RevisionID: law.RevisionID(id + "_1")})
	}
	d := f.deps()
	d.Concurrency = 4
	res, _ := NewDailySyncer(d).Run(context.Background(), DailyOptions{ReleaseTag: "t", XMLDir: t.TempDir()})
	if res.ExitCode != 0 || res.Record.Counts["xml_failed"] != 21 || res.Record.Warnings[0] != "xml_failures" {
		t.Fatalf("record=%+v", res.Record)
	}
}

func TestDailyRetriesBootstrapPendingWhenNewerThanLastDaily(t *testing.T) {
	f := newFakes()
	f.clock.now = time.Date(2026, 9, 12, 0, 0, 0, 0, law.JST)
	f.repo.bootstrap = &RunRecord{Kind: "bootstrap", StartedAt: "2026-09-11T22:00:00Z", DateJST: "2026-09-11", Applied: true, PendingLawIDs: []law.LawID{"X"}}
	f.repo.lastDaily = &RunRecord{Kind: "daily", StartedAt: "2026-09-10T22:00:00Z", PendingLawIDs: []law.LawID{"Y"}}
	f.updates.notFound = true
	res, _ := NewDailySyncer(f.deps()).Run(context.Background(), DailyOptions{XMLDir: t.TempDir()})
	if res.ExitCode != 0 || f.revisions.calls["X"] != 1 || f.revisions.calls["Y"] != 0 {
		t.Fatalf("res=%+v calls=%v", res.Record, f.revisions.calls)
	}
}

func TestDailyFromAfterToIsError(t *testing.T) {
	f := newFakes()
	f.clock.now = time.Date(2026, 9, 12, 0, 0, 0, 0, law.JST)
	f.repo.bootstrap = &RunRecord{Kind: "bootstrap", DateJST: "2026-09-11", Applied: true}
	_, err := NewDailySyncer(f.deps()).Run(context.Background(), DailyOptions{From: "2026-09-09", To: "2026-09-08", XMLDir: t.TempDir()})
	if !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("err=%v", err)
	}
}

func TestDailyBundlesFetchedXML(t *testing.T) {
	f := newFakes()
	f.clock.now = time.Date(2026, 9, 12, 0, 0, 0, 0, law.JST)
	f.repo.bootstrap = &RunRecord{Kind: "bootstrap", DateJST: "2026-09-11", TotalCount: 1, Applied: true}
	f.catalog.current = []law.Law{{ID: "A", RevisionID: "A_2", Updated: "u2"}}
	f.updates.notFound = true
	f.xml.sha["A_2"] = "sha2"
	dir := t.TempDir()
	res, _ := NewDailySyncer(f.deps()).Run(context.Background(), DailyOptions{ReleaseTag: "sync-1", XMLDir: dir, ZipPath: "bin/laws-xml.zip"})
	if res.ExitCode != 0 || len(f.bundler.calls) != 1 {
		t.Fatalf("res=%+v calls=%+v", res.Record, f.bundler.calls)
	}
	got := f.bundler.calls[0]
	if got.xmlDir != dir || got.outPath != "bin/laws-xml.zip" || len(got.index) != 1 || got.index[0].RevisionID != "A_2" {
		t.Fatalf("call=%+v", got)
	}
}

func TestDailyDoesNotBundleWhenNothingFetched(t *testing.T) {
	f := newFakes()
	f.clock.now = time.Date(2026, 9, 12, 0, 0, 0, 0, law.JST)
	f.repo.bootstrap = &RunRecord{Kind: "bootstrap", DateJST: "2026-09-11", TotalCount: 1, Applied: true}
	f.repo.laws = []law.Law{{ID: "A", RevisionID: "A_1", Updated: "u1"}}
	f.repo.xml = map[law.RevisionID]law.XMLRecord{"A_1": {RevisionID: "A_1", Updated: "u1"}}
	f.catalog.current = []law.Law{{ID: "A", RevisionID: "A_1", Updated: "u1"}}
	f.updates.notFound = true
	res, _ := NewDailySyncer(f.deps()).Run(context.Background(), DailyOptions{ReleaseTag: "t", XMLDir: t.TempDir(), ZipPath: "bin/laws-xml.zip"})
	if res.ExitCode != 0 || len(f.bundler.calls) != 0 {
		t.Fatalf("calls=%+v", f.bundler.calls)
	}
}

func TestDailyDoesNotBundleWithoutReleaseTag(t *testing.T) {
	f := newFakes()
	f.clock.now = time.Date(2026, 9, 12, 0, 0, 0, 0, law.JST)
	f.repo.bootstrap = &RunRecord{Kind: "bootstrap", DateJST: "2026-09-11", TotalCount: 1, Applied: true}
	f.catalog.current = []law.Law{{ID: "A", RevisionID: "A_2", Updated: "u2"}}
	f.updates.notFound = true
	f.xml.sha["A_2"] = "sha2"
	res, _ := NewDailySyncer(f.deps()).Run(context.Background(), DailyOptions{XMLDir: t.TempDir(), ZipPath: "bin/laws-xml.zip"})
	if res.ExitCode != 0 || len(f.bundler.calls) != 0 {
		t.Fatalf("calls=%+v", f.bundler.calls)
	}
}

func TestDailyBundleErrorIsWarningOnly(t *testing.T) {
	f := newFakes()
	f.clock.now = time.Date(2026, 9, 12, 0, 0, 0, 0, law.JST)
	f.repo.bootstrap = &RunRecord{Kind: "bootstrap", DateJST: "2026-09-11", TotalCount: 1, Applied: true}
	f.catalog.current = []law.Law{{ID: "A", RevisionID: "A_2", Updated: "u2"}}
	f.updates.notFound = true
	f.xml.sha["A_2"] = "sha2"
	f.bundler.err = errors.New("disk full")
	res, err := NewDailySyncer(f.deps()).Run(context.Background(), DailyOptions{ReleaseTag: "t", XMLDir: t.TempDir(), ZipPath: "bin/laws-xml.zip"})
	if err != nil || res.ExitCode != 0 || !res.Record.Applied {
		t.Fatalf("res=%+v err=%v", res.Record, err)
	}
	if res.Record.Warnings[0] != "bundle_failed" {
		t.Fatalf("warnings=%v", res.Record.Warnings)
	}
}
