package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

func TestBootstrapBuildsThreeCSVs(t *testing.T) {
	f := newFakes()
	f.catalog.current = []law.Law{{ID: "A", RevisionID: "A_1", Updated: "u1"}, {ID: "B", RevisionID: "B_1", Updated: "u1"}}
	f.catalog.future = []law.Law{{ID: "A", RevisionID: "A_2"}, {ID: "B", RevisionID: "B_1"}}
	f.revisions.byLaw["A"] = []law.Revision{{ID: "A_1", LawID: "A"}, {ID: "A_2", LawID: "A", Status: "UnEnforced"}}
	f.xml.sha["A_1"] = "sha-a"
	f.xml.sha["B_1"] = "sha-b"
	res, err := NewBootstrapper(f.deps()).Run(context.Background(), BootstrapOptions{ReleaseTag: "bootstrap-x", XMLDir: t.TempDir()})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if len(f.repo.laws) != 2 || len(f.repo.revisions) != 2 || len(f.repo.xml) != 2 {
		t.Fatalf("laws=%d revs=%d xml=%d", len(f.repo.laws), len(f.repo.revisions), len(f.repo.xml))
	}
	if f.repo.xml["A_1"].ReleaseTag != "bootstrap-x" || f.repo.revisions["A_2"].FirstSeen == "" {
		t.Fatal("release tag or first_seen missing")
	}
	if f.revisions.calls["B"] != 0 {
		t.Fatal("B has no pending revision; must not be fetched")
	}
}

func TestBootstrapKeepsFirstSeenOnRerun(t *testing.T) {
	f := newFakes()
	f.repo.revisions = map[law.RevisionID]law.Revision{"A_2": {ID: "A_2", LawID: "A", FirstSeen: "2026-01-01"}}
	f.catalog.current = []law.Law{{ID: "A", RevisionID: "A_1"}}
	f.catalog.future = []law.Law{{ID: "A", RevisionID: "A_2"}}
	f.revisions.byLaw["A"] = []law.Revision{{ID: "A_2", LawID: "A", Status: "UnEnforced"}}
	f.xml.sha["A_1"] = "x"
	if _, err := NewBootstrapper(f.deps()).Run(context.Background(), BootstrapOptions{ReleaseTag: "t", XMLDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if f.repo.revisions["A_2"].FirstSeen != "2026-01-01" {
		t.Fatalf("first_seen overwritten: %+v", f.repo.revisions["A_2"])
	}
}

func TestBootstrapCountMismatchIsAnomaly(t *testing.T) {
	f := newFakes()
	f.catalog.current = []law.Law{{ID: "A", RevisionID: "A_1"}}
	f.catalog.total = 2
	res, _ := NewBootstrapper(f.deps()).Run(context.Background(), BootstrapOptions{ReleaseTag: "t", XMLDir: t.TempDir()})
	if res.ExitCode != 3 || f.repo.savedLaws {
		t.Fatalf("res=%+v savedLaws=%v", res, f.repo.savedLaws)
	}
	if len(f.repo.runs) != 1 {
		t.Fatal("runs/ must be written even on anomaly")
	}
}

func TestBootstrapEmptyReleaseTagSkipsXML(t *testing.T) {
	f := newFakes()
	f.catalog.current = []law.Law{{ID: "A", RevisionID: "A_1"}}
	res, err := NewBootstrapper(f.deps()).Run(context.Background(), BootstrapOptions{XMLDir: t.TempDir()})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if f.xml.calls != 0 || f.repo.savedXML {
		t.Fatalf("xml must be untouched: calls=%d savedXML=%v", f.xml.calls, f.repo.savedXML)
	}
}

func TestBootstrapRevisionFetchFailurePends(t *testing.T) {
	f := newFakes()
	f.catalog.current = []law.Law{{ID: "A", RevisionID: "A_1"}}
	f.catalog.future = []law.Law{{ID: "A", RevisionID: "A_2"}}
	f.revisions.errs["A"] = errors.New("503")
	res, err := NewBootstrapper(f.deps()).Run(context.Background(), BootstrapOptions{XMLDir: t.TempDir()})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if len(res.Record.PendingLawIDs) != 1 || res.Record.Warnings[0] != "revision_fetch_failed" {
		t.Fatalf("record=%+v", res.Record)
	}
}

func TestBootstrapCatalogErrorReturnsError(t *testing.T) {
	f := newFakes()
	f.catalog.err = errors.New("500")
	if _, err := NewBootstrapper(f.deps()).Run(context.Background(), BootstrapOptions{XMLDir: t.TempDir()}); err == nil {
		t.Fatal("want error")
	}
	if len(f.repo.runs) != 0 {
		t.Fatal("nothing must be written on transport failure")
	}
}

func TestBootstrapXMLFailureIsCounted(t *testing.T) {
	f := newFakes()
	f.catalog.current = []law.Law{{ID: "A", RevisionID: "A_1"}, {ID: "B", RevisionID: "B_1"}}
	f.xml.sha["A_1"] = "sha-a"
	res, _ := NewBootstrapper(f.deps()).Run(context.Background(), BootstrapOptions{ReleaseTag: "t", XMLDir: t.TempDir()})
	if res.Record.Counts["xml_ok"] != 1 || res.Record.Counts["xml_failed"] != 1 || len(f.repo.xml) != 1 {
		t.Fatalf("counts=%v xml=%v", res.Record.Counts, f.repo.xml)
	}
}

func TestBootstrapFutureCountMismatchIsAnomaly(t *testing.T) {
	f := newFakes()
	f.catalog.current = []law.Law{{ID: "A", RevisionID: "A_1"}}
	f.catalog.future = []law.Law{{ID: "A", RevisionID: "A_2"}}
	f.catalog.futureTotal = 9
	res, _ := NewBootstrapper(f.deps()).Run(context.Background(), BootstrapOptions{ReleaseTag: "t", XMLDir: t.TempDir()})
	if res.ExitCode != 3 || f.repo.savedLaws || res.Record.Anomalies[0] != "count_mismatch" {
		t.Fatalf("res=%+v savedLaws=%v", res.Record, f.repo.savedLaws)
	}
	if len(f.repo.runs) != 1 {
		t.Fatal("runs/ must be written even on anomaly")
	}
}

func TestBootstrapBundlesFetchedXML(t *testing.T) {
	f := newFakes()
	f.catalog.current = []law.Law{{ID: "B", RevisionID: "B_1"}, {ID: "A", RevisionID: "A_1"}}
	f.xml.sha["A_1"] = "sha-a"
	f.xml.sha["B_1"] = "sha-b"
	res, _ := NewBootstrapper(f.deps()).Run(context.Background(), BootstrapOptions{ReleaseTag: "t", XMLDir: t.TempDir(), ZipPath: "bin/laws-xml.zip"})
	if res.ExitCode != 0 || len(f.bundler.calls) != 1 {
		t.Fatalf("calls=%+v", f.bundler.calls)
	}
	index := f.bundler.calls[0].index
	if len(index) != 2 || index[0].RevisionID != "A_1" || index[1].RevisionID != "B_1" {
		t.Fatalf("index must be sorted by revision_id: %+v", index)
	}
}

func TestBootstrapWarnsWhenXMLFailuresExceedThreshold(t *testing.T) {
	f := newFakes()
	for i := range 21 {
		id := law.LawID(fmt.Sprintf("L%03d", i))
		f.catalog.current = append(f.catalog.current, law.Law{ID: id, RevisionID: law.RevisionID(string(id) + "_1")})
	}
	res, err := NewBootstrapper(f.deps()).Run(context.Background(), BootstrapOptions{ReleaseTag: "t", XMLDir: t.TempDir()})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if res.Record.Counts["xml_failed"] != 21 || !slices.Contains(res.Record.Warnings, "xml_failures") {
		t.Fatalf("counts=%v warnings=%v", res.Record.Counts, res.Record.Warnings)
	}
}

func TestBootstrapDoesNotWarnAtThreshold(t *testing.T) {
	f := newFakes()
	for i := range 20 {
		id := law.LawID(fmt.Sprintf("L%03d", i))
		f.catalog.current = append(f.catalog.current, law.Law{ID: id, RevisionID: law.RevisionID(string(id) + "_1")})
	}
	res, _ := NewBootstrapper(f.deps()).Run(context.Background(), BootstrapOptions{ReleaseTag: "t", XMLDir: t.TempDir()})
	if len(res.Record.Warnings) != 0 {
		t.Fatalf("warnings=%v", res.Record.Warnings)
	}
}

// seqLaws L000からn件の法令。revision_idは<id>_1
func seqLaws(n int) []law.Law {
	laws := make([]law.Law, 0, n)
	for i := range n {
		id := law.LawID(fmt.Sprintf("L%03d", i))
		laws = append(laws, law.Law{ID: id, RevisionID: law.RevisionID(string(id) + "_1"), Updated: "u1"})
	}
	return laws
}

// INV-B1、INV-B4、INV-B5。既存laws.csvの200件に対し一覧が197件（1.5%減）なら、取得の前に止めて消失一覧を残す
func TestBootstrapRerunTotalDroppedIsAnomaly(t *testing.T) {
	f := newFakes()
	f.repo.laws = seqLaws(200)
	f.catalog.current = seqLaws(197)
	f.catalog.future = []law.Law{{ID: "L000", RevisionID: "L000_2"}}
	f.revisions.byLaw["L000"] = []law.Revision{{ID: "L000_2", LawID: "L000", Status: "UnEnforced"}}
	res, err := NewBootstrapper(f.deps()).Run(context.Background(), BootstrapOptions{ReleaseTag: "t", XMLDir: t.TempDir()})
	if err != nil || res.ExitCode != 3 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if f.repo.savedLaws || f.repo.savedXML || len(f.repo.runs) != 1 {
		t.Fatalf("csv must not be written: savedLaws=%v savedXML=%v runs=%d", f.repo.savedLaws, f.repo.savedXML, len(f.repo.runs))
	}
	if !slices.Contains(res.Record.Anomalies, "total_count_dropped") {
		t.Fatalf("anomalies=%v", res.Record.Anomalies)
	}
	if f.xml.calls != 0 || f.revisions.calls["L000"] != 0 {
		t.Fatalf("fetch must not start before the check: xml=%d revisions=%d", f.xml.calls, f.revisions.calls["L000"])
	}
	if res.Record.Counts["removed"] != 3 || len(res.Record.Changes) != 3 || res.Record.Changes[0].LawID != "L197" || res.Record.Changes[0].Kind != "removed" {
		t.Fatalf("counts=%v changes=%+v", res.Record.Counts, res.Record.Changes)
	}
}

// INV-B1の境界。200件から198件（ちょうど1%）は異常にせず、消失2件を記録して適用する
func TestBootstrapRerunDropAtThresholdApplies(t *testing.T) {
	f := newFakes()
	f.repo.laws = seqLaws(200)
	f.catalog.current = seqLaws(198)
	res, err := NewBootstrapper(f.deps()).Run(context.Background(), BootstrapOptions{XMLDir: t.TempDir()})
	if err != nil || res.ExitCode != 0 || !f.repo.savedLaws {
		t.Fatalf("res=%+v err=%v savedLaws=%v", res, err, f.repo.savedLaws)
	}
	if len(f.repo.laws) != 198 || res.Record.Counts["removed"] != 2 || len(res.Record.Changes) != 2 {
		t.Fatalf("laws=%d counts=%v changes=%d", len(f.repo.laws), res.Record.Counts, len(res.Record.Changes))
	}
}

// INV-B2。--forceなら1%超の減少でも適用する
func TestBootstrapRerunForceApplies(t *testing.T) {
	f := newFakes()
	f.repo.laws = seqLaws(200)
	f.catalog.current = seqLaws(197)
	res, err := NewBootstrapper(f.deps()).Run(context.Background(), BootstrapOptions{XMLDir: t.TempDir(), Force: true})
	if err != nil || res.ExitCode != 0 || !f.repo.savedLaws || len(f.repo.laws) != 197 {
		t.Fatalf("res=%+v err=%v savedLaws=%v", res, err, f.repo.savedLaws)
	}
	if len(res.Record.Anomalies) != 0 || res.Record.Counts["removed"] != 3 {
		t.Fatalf("record=%+v", res.Record)
	}
}

// INV-B3。laws.csvが無ければ減少判定はしない（一覧0件でも適用する）
func TestBootstrapFirstRunSkipsDropCheck(t *testing.T) {
	f := newFakes()
	res, err := NewBootstrapper(f.deps()).Run(context.Background(), BootstrapOptions{XMLDir: t.TempDir()})
	if err != nil || res.ExitCode != 0 || !f.repo.savedLaws {
		t.Fatalf("res=%+v err=%v savedLaws=%v", res, err, f.repo.savedLaws)
	}
	if _, ok := res.Record.Counts["removed"]; ok {
		t.Fatalf("removed must not be counted on first run: %v", res.Record.Counts)
	}
}
