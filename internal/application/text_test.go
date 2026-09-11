package application

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
	"github.com/okamyuji/egov-law-sync/internal/domain/sync"
)

// fixedNow text_test.go内のRunRecord生成に使う固定時刻
var fixedNow = time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)

// defaultThresholdsForTest MaxFetchFailuresは20
func defaultThresholdsForTest() sync.Thresholds {
	return sync.DefaultThresholds()
}

func TestINV9TextFailureDoesNotTouchIndexOrExitCode(t *testing.T) {
	targets := []law.Law{{ID: "A", RevisionID: "A_1"}, {ID: "B", RevisionID: "B_1"}}
	fetched := []law.XMLRecord{{RevisionID: "A_1"}, {RevisionID: "B_1"}}
	ft := &fakeText{fail: map[law.RevisionID]bool{"B_1": true}}
	d := Deps{Text: ft, Threshold: defaultThresholdsForTest()}
	rec := newRecord("daily", fixedNow)
	rendered := renderTexts(d, targets, fetched, "xml", "text", &rec)
	if len(rendered) != 1 || rendered[0].RevisionID != "A_1" {
		t.Fatalf("rendered = %+v", rendered)
	}
	if rec.Counts["text_ok"] != 1 || rec.Counts["text_failed"] != 1 {
		t.Fatalf("counts = %v", rec.Counts)
	}
	if len(rec.Warnings) != 0 {
		t.Fatalf("1 failure must not warn: %v", rec.Warnings)
	}
}

func TestRenderTextsSkipsWhenTextDirEmpty(t *testing.T) {
	ft := &fakeText{}
	rec := newRecord("daily", fixedNow)
	rendered := renderTexts(Deps{Text: ft}, []law.Law{{RevisionID: "A_1"}}, []law.XMLRecord{{RevisionID: "A_1"}}, "xml", "", &rec)
	if len(rendered) != 0 || len(ft.calls) != 0 {
		t.Fatal("must skip rendering when textDir is empty")
	}
}

func TestRenderTextsOnlyForFetchedRevisions(t *testing.T) {
	ft := &fakeText{}
	rec := newRecord("daily", fixedNow)
	targets := []law.Law{{RevisionID: "A_1"}, {RevisionID: "B_1"}}
	renderTexts(Deps{Text: ft, Threshold: defaultThresholdsForTest()}, targets, []law.XMLRecord{{RevisionID: "B_1"}}, "xml", "text", &rec)
	if len(ft.calls) != 1 || ft.calls[0] != "B_1" {
		t.Fatalf("calls = %v", ft.calls)
	}
}

func TestTextFailuresWarnAboveThreshold(t *testing.T) {
	fail := map[law.RevisionID]bool{}
	var targets []law.Law
	var fetched []law.XMLRecord
	for i := range 21 {
		id := law.RevisionID("R_" + string(rune('a'+i)))
		fail[id] = true
		targets = append(targets, law.Law{RevisionID: id})
		fetched = append(fetched, law.XMLRecord{RevisionID: id})
	}
	rec := newRecord("daily", fixedNow)
	renderTexts(Deps{Text: &fakeText{fail: fail}, Threshold: defaultThresholdsForTest()}, targets, fetched, "xml", "text", &rec)
	if !slices.Contains(rec.Warnings, "text_failures") {
		t.Fatalf("warnings = %v", rec.Warnings)
	}
}

func TestBundleTextWarnsOnFailure(t *testing.T) {
	b := &fakeBundler{textErr: errors.New("bundle failed")}
	rec := newRecord("daily", fixedNow)
	bundleText(Deps{Bundler: b}, "text", "out.zip", []law.TextRecord{{RevisionID: "A_1"}}, &rec)
	if len(b.textCalls) != 1 || !slices.Contains(rec.Warnings, "text_bundle_failed") {
		t.Fatalf("calls=%+v warnings=%v", b.textCalls, rec.Warnings)
	}
}

func TestBundleTextSkipsWhenNothingRendered(t *testing.T) {
	b := &fakeBundler{}
	rec := newRecord("daily", fixedNow)
	bundleText(Deps{Bundler: b}, "text", "out.zip", nil, &rec)
	if len(b.textCalls) != 0 {
		t.Fatalf("must skip bundling when rendered is empty: %+v", b.textCalls)
	}
}

func TestINV9RenderFailureKeepsIndexAndExitCodeInBootstrap(t *testing.T) {
	f := newFakes()
	f.catalog.current = []law.Law{{ID: "A", RevisionID: "A_1", Updated: "u1"}, {ID: "B", RevisionID: "B_1", Updated: "u1"}}
	f.catalog.future = f.catalog.current
	f.xml.sha["A_1"] = "sha-a"
	f.xml.sha["B_1"] = "sha-b"
	d := f.deps()
	d.Text = &fakeText{fail: map[law.RevisionID]bool{"A_1": true, "B_1": true}}
	res, err := NewBootstrapper(d).Run(context.Background(), BootstrapOptions{ReleaseTag: "v0.0.1", XMLDir: t.TempDir(), TextDir: t.TempDir(), TextZipPath: "t.zip"})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if len(f.repo.xml) != 2 || f.repo.xml["A_1"].SHA256 != "sha-a" || f.repo.xml["B_1"].SHA256 != "sha-b" {
		t.Fatalf("xml index must hold both fetched rows: %+v", f.repo.xml)
	}
	if res.Record.Counts["text_failed"] != 2 || res.Record.Counts["text_ok"] != 0 {
		t.Fatalf("counts = %v", res.Record.Counts)
	}
	if len(f.bundler.textCalls) != 0 {
		t.Fatal("nothing rendered, so laws-text.zip must not be built")
	}
}
