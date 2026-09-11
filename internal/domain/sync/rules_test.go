package sync

import (
	"testing"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

func TestCheckAnomaliesBoundaries(t *testing.T) {
	th := DefaultThresholds()
	cases := []struct {
		name       string
		unexpected int
		prev, cur  int
		force      bool
		want       int
	}{
		{"200 is ok", 200, 9567, 9567, false, 0},
		{"201 is anomaly", 201, 9567, 9567, false, 1},
		{"1% drop exactly is ok", 0, 10000, 9900, false, 0},
		{"more than 1% drop", 0, 10000, 9899, false, 1},
		{"no previous total", 0, 0, 5, false, 0},
		{"force skips both", 500, 10000, 1, true, 0},
		{"increase is ok", 0, 9567, 9600, false, 0},
		{"prevTotal zero with negative curTotal is not anomaly", 0, 0, -1, false, 0},
	}
	for _, c := range cases {
		got := CheckAnomalies(c.unexpected, c.prev, c.cur, th, c.force)
		if len(got) != c.want {
			t.Fatalf("%s: got %v", c.name, got)
		}
	}
}

func TestCheckAnomaliesBothKinds(t *testing.T) {
	th := DefaultThresholds()
	got := CheckAnomalies(201, 10000, 1, th, false)
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	found := map[Anomaly]bool{}
	for _, a := range got {
		found[a] = true
	}
	if !found[AnomalyTooManyChanges] || !found[AnomalyTotalDropped] {
		t.Fatalf("got %v", got)
	}
}

func TestXMLTargets(t *testing.T) {
	cur := []law.Law{
		{ID: "A", RevisionID: "A_1", Updated: "u1"},
		{ID: "B", RevisionID: "B_1", Updated: "u2"},
		{ID: "C", RevisionID: "C_1", Updated: "u1"},
	}
	index := map[law.RevisionID]law.XMLRecord{
		"A_1": {RevisionID: "A_1", Updated: "u1"},
		"B_1": {RevisionID: "B_1", Updated: "u1"},
	}
	got := XMLTargets(cur, index)
	if len(got) != 2 || got[0].ID != "B" || got[1].ID != "C" {
		t.Fatalf("got %+v", got)
	}
}

func TestXMLTargetsEmpty(t *testing.T) {
	if got := XMLTargets(nil, nil); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestDateRange(t *testing.T) {
	if got := DateRange("2026-09-09", "2026-09-11"); len(got) != 3 || got[2] != "2026-09-11" {
		t.Fatalf("got %v", got)
	}
	if got := DateRange("2026-09-12", "2026-09-11"); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	if got := DateRange("2026-09-11", "2026-09-11"); len(got) != 1 || got[0] != "2026-09-11" {
		t.Fatalf("got %v", got)
	}
	if got := V1Range("2026-09-11", "2026-09-11"); len(got) != 3 || got[0] != "2026-09-09" {
		t.Fatalf("got %v", got)
	}
}

func TestDefaultThresholdsValues(t *testing.T) {
	th := DefaultThresholds()
	if th.MaxUnexpected != 200 || th.MaxTotalDropRatio != 0.01 || th.MaxFetchFailures != 20 {
		t.Fatalf("got %+v", th)
	}
}
