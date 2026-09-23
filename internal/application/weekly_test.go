package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

func TestWeeklyRepairsMismatch(t *testing.T) {
	f := newFakes()
	f.repo.laws = []law.Law{{ID: "A", RevisionID: "A_1", Updated: "u5", RepealStatus: "None"}, {ID: "B", RevisionID: "B_1", Updated: "u1", RepealStatus: "Repeal"}}
	f.repo.xml = map[law.RevisionID]law.XMLRecord{"A_1": {RevisionID: "A_1", Updated: "u1", SHA256: "old", ReleaseTag: "b"}}
	f.bulk.sha["A_1"] = "zip-sha"
	f.xml.sha["A_1"] = "new"
	res, err := NewWeeklyChecker(f.deps()).Run(context.Background(), WeeklyOptions{ReleaseTag: "sync-w", XMLDir: t.TempDir()})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	got := f.repo.xml["A_1"]
	if got.SHA256 != "new" || got.ReleaseTag != "sync-w" || got.Updated != "u5" || res.Record.Counts["repaired"] != 1 {
		t.Fatalf("got %+v counts=%v", got, res.Record.Counts)
	}
	if f.bulk.lookups["B_1"] != 0 {
		t.Fatal("repealed law must be skipped")
	}
}

func TestWeeklyStaleZipIsCountedOnly(t *testing.T) {
	f := newFakes()
	f.repo.laws = []law.Law{{ID: "A", RevisionID: "A_1", RepealStatus: "None"}}
	f.repo.xml = map[law.RevisionID]law.XMLRecord{"A_1": {RevisionID: "A_1", SHA256: "cur"}}
	f.bulk.sha["A_1"] = "zip-old"
	f.xml.sha["A_1"] = "cur"
	res, _ := NewWeeklyChecker(f.deps()).Run(context.Background(), WeeklyOptions{ReleaseTag: "t", XMLDir: t.TempDir()})
	if res.Record.Counts["zip_stale"] != 1 || res.Record.Counts["repaired"] != 0 || f.repo.xml["A_1"].SHA256 != "cur" {
		t.Fatalf("counts=%v", res.Record.Counts)
	}
}

func TestWeeklyCountsMissingAndZipOnly(t *testing.T) {
	f := newFakes()
	f.repo.laws = []law.Law{{ID: "A", RevisionID: "A_1", RepealStatus: "None"}}
	f.repo.xml = map[law.RevisionID]law.XMLRecord{"A_1": {RevisionID: "A_1", SHA256: "cur"}}
	f.bulk.sha["X_9"] = "other"
	res, _ := NewWeeklyChecker(f.deps()).Run(context.Background(), WeeklyOptions{ReleaseTag: "t", XMLDir: t.TempDir()})
	if res.Record.Counts["missing_in_zip"] != 1 || res.Record.Counts["zip_only"] != 1 {
		t.Fatalf("counts=%v", res.Record.Counts)
	}
	if !f.bulk.closed {
		t.Fatal("archive must be closed")
	}
}

func TestWeeklyEmptyReleaseTagLeavesIndex(t *testing.T) {
	f := newFakes()
	f.repo.laws = []law.Law{{ID: "A", RevisionID: "A_1", RepealStatus: "None"}}
	f.repo.xml = map[law.RevisionID]law.XMLRecord{"A_1": {RevisionID: "A_1", SHA256: "cur"}}
	f.bulk.sha["A_1"] = "zip-old"
	res, _ := NewWeeklyChecker(f.deps()).Run(context.Background(), WeeklyOptions{XMLDir: t.TempDir()})
	if res.ExitCode != 0 || f.xml.calls != 0 || f.repo.savedXML {
		t.Fatalf("xml must be untouched: calls=%d savedXML=%v", f.xml.calls, f.repo.savedXML)
	}
}

func TestWeeklyBulkErrorReturnsError(t *testing.T) {
	f := newFakes()
	f.bulk.err = errors.New("timeout")
	if _, err := NewWeeklyChecker(f.deps()).Run(context.Background(), WeeklyOptions{XMLDir: t.TempDir()}); err == nil {
		t.Fatal("want error")
	}
}

func TestWeeklyBundlesRepairedXML(t *testing.T) {
	f := newFakes()
	f.repo.laws = []law.Law{{ID: "A", RevisionID: "A_1", Updated: "u5", RepealStatus: "None"}}
	f.repo.xml = map[law.RevisionID]law.XMLRecord{"A_1": {RevisionID: "A_1", SHA256: "old"}}
	f.bulk.sha["A_1"] = "zip-sha"
	f.xml.sha["A_1"] = "new"
	res, _ := NewWeeklyChecker(f.deps()).Run(context.Background(), WeeklyOptions{ReleaseTag: "w", XMLDir: t.TempDir(), ZipPath: "bin/laws-xml.zip"})
	if res.ExitCode != 0 || len(f.bundler.calls) != 1 || f.bundler.calls[0].index[0].RevisionID != "A_1" {
		t.Fatalf("calls=%+v", f.bundler.calls)
	}
}

func TestWeeklyDoesNotBundleWhenNothingRepaired(t *testing.T) {
	f := newFakes()
	f.repo.laws = []law.Law{{ID: "A", RevisionID: "A_1", RepealStatus: "None"}}
	f.repo.xml = map[law.RevisionID]law.XMLRecord{"A_1": {RevisionID: "A_1", SHA256: "cur"}}
	f.bulk.sha["A_1"] = "zip-old"
	f.xml.sha["A_1"] = "cur"
	res, _ := NewWeeklyChecker(f.deps()).Run(context.Background(), WeeklyOptions{ReleaseTag: "w", XMLDir: t.TempDir(), ZipPath: "bin/laws-xml.zip"})
	if res.ExitCode != 0 || len(f.bundler.calls) != 0 {
		t.Fatalf("calls=%+v", f.bundler.calls)
	}
}

func TestWeeklyWarnsWhenXMLFailuresExceedThreshold(t *testing.T) {
	f := newFakes()
	for i := range 21 {
		id := law.RevisionID(fmt.Sprintf("R%03d", i))
		f.repo.laws = append(f.repo.laws, law.Law{ID: law.LawID(fmt.Sprintf("L%03d", i)), RevisionID: id, RepealStatus: "None"})
		f.repo.xml[id] = law.XMLRecord{RevisionID: id, SHA256: "cur"}
		f.bulk.sha[id] = "zip-old"
		f.xml.errs[id] = errors.New("503")
	}
	res, err := NewWeeklyChecker(f.deps()).Run(context.Background(), WeeklyOptions{ReleaseTag: "t", XMLDir: t.TempDir()})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if res.Record.Counts["xml_failed"] != 21 || !slices.Contains(res.Record.Warnings, "xml_failures") {
		t.Fatalf("counts=%v warnings=%v", res.Record.Counts, res.Record.Warnings)
	}
}

func TestScopedWeeklyDoesNotCountUnrelatedZipEntries(t *testing.T) {
	f := newFakes()
	f.repo.laws = []law.Law{{ID: "A", RevisionID: "A_1", RepealStatus: "None"}}
	f.repo.xml = map[law.RevisionID]law.XMLRecord{"A_1": {RevisionID: "A_1", SHA256: "cur"}}
	f.bulk.sha["A_1"] = "cur"
	f.bulk.sha["B_1"] = "other"
	deps := f.deps()
	deps.Scope = "A"
	res, err := NewWeeklyChecker(deps).Run(context.Background(), WeeklyOptions{})
	if err != nil || res.ExitCode != 0 || res.Record.Counts["zip_only"] != 0 || f.bulk.lookups["A_1"] != 1 {
		t.Fatalf("result=%+v err=%v", res, err)
	}
}
