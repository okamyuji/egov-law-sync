package application

import (
	"context"
	"errors"
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
