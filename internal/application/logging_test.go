package application

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

// captureLog テストの間だけlogの出力先を差し替える
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	flags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(flags)
	})
	return &buf
}

func TestRevisionGoneDropsLawInsteadOfRequeueing(t *testing.T) {
	f := newFakes()
	f.clock.now = time.Date(2026, 9, 12, 0, 0, 0, 0, law.JST)
	f.repo.bootstrap = &RunRecord{Kind: "bootstrap", DateJST: "2026-09-11", TotalCount: 1, Applied: true}
	f.repo.laws = []law.Law{{ID: "A", RevisionID: "A_1", Updated: "u1"}}
	f.catalog.current = []law.Law{{ID: "A", RevisionID: "A_1", Updated: "u2"}}
	f.updates.notFound = true
	f.revisions.gone["A"] = true
	res, err := NewDailySyncer(f.deps()).Run(context.Background(), DailyOptions{})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if res.Record.Counts["revision_gone"] != 1 {
		t.Fatalf("counts=%v", res.Record.Counts)
	}
	if len(res.Record.PendingLawIDs) != 0 || len(res.Record.Warnings) != 0 {
		t.Fatalf("pending=%v warnings=%v", res.Record.PendingLawIDs, res.Record.Warnings)
	}
}

func TestRevisionFetchErrorStillRequeues(t *testing.T) {
	f := newFakes()
	f.clock.now = time.Date(2026, 9, 12, 0, 0, 0, 0, law.JST)
	f.repo.bootstrap = &RunRecord{Kind: "bootstrap", DateJST: "2026-09-11", TotalCount: 1, Applied: true}
	f.repo.laws = []law.Law{{ID: "A", RevisionID: "A_1", Updated: "u1"}}
	f.catalog.current = []law.Law{{ID: "A", RevisionID: "A_1", Updated: "u2"}}
	f.updates.notFound = true
	f.revisions.errs["A"] = errors.New("boom")
	buf := captureLog(t)
	res, err := NewDailySyncer(f.deps()).Run(context.Background(), DailyOptions{})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if len(res.Record.PendingLawIDs) != 1 || res.Record.Counts["revision_gone"] != 0 {
		t.Fatalf("pending=%v counts=%v", res.Record.PendingLawIDs, res.Record.Counts)
	}
	if !strings.Contains(buf.String(), "boom") || !strings.Contains(buf.String(), "A") {
		t.Fatalf("log=%q", buf.String())
	}
}

func TestFetchErrorsLogFirstTenThenSummary(t *testing.T) {
	f := newFakes()
	for i := range 12 {
		id := law.LawID(string(rune('A' + i)))
		f.catalog.current = append(f.catalog.current, law.Law{ID: id, RevisionID: law.RevisionID(string(id) + "_1")})
	}
	buf := captureLog(t)
	if _, err := NewBootstrapper(f.deps()).Run(context.Background(), BootstrapOptions{ReleaseTag: "t", XMLDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 11 {
		t.Fatalf("want 10 error lines and 1 summary, got %d: %q", len(lines), buf.String())
	}
	if !strings.Contains(lines[10], "2 more") {
		t.Fatalf("summary=%q", lines[10])
	}
}

func TestFetchErrorsUnderTenHaveNoSummary(t *testing.T) {
	f := newFakes()
	f.catalog.current = []law.Law{{ID: "A", RevisionID: "A_1"}}
	buf := captureLog(t)
	if _, err := NewBootstrapper(f.deps()).Run(context.Background(), BootstrapOptions{ReleaseTag: "t", XMLDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(buf.String(), "\n"); n != 1 || strings.Contains(buf.String(), "more") {
		t.Fatalf("log=%q", buf.String())
	}
}
