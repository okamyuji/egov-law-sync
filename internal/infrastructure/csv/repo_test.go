package csv

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/okamyuji/egov-law-sync/internal/application"
	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

func TestRoundTripAndOrder(t *testing.T) {
	r := New(t.TempDir())
	if err := r.SaveLaws([]law.Law{{ID: "B", RevisionID: "B_1"}, {ID: "A", RevisionID: "A_1", Title: "a,b"}}); err != nil {
		t.Fatal(err)
	}
	got, err := r.LoadLaws()
	if err != nil || len(got) != 2 || got[0].ID != "A" || got[1].Title != "" || got[0].Title != "a,b" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(r.dir, "laws.csv.tmp")); err == nil {
		t.Fatal("tmp file must be renamed away")
	}
}

func TestLoadMissingIsEmpty(t *testing.T) {
	r := New(t.TempDir())
	laws, err := r.LoadLaws()
	if err != nil || len(laws) != 0 {
		t.Fatalf("laws=%v err=%v", laws, err)
	}
	revs, err := r.LoadRevisions()
	if err != nil || len(revs) != 0 {
		t.Fatalf("revs=%v err=%v", revs, err)
	}
	idx, err := r.LoadXMLIndex()
	if err != nil || len(idx) != 0 {
		t.Fatalf("idx=%v err=%v", idx, err)
	}
	if _, ok, err := r.LastDaily(); ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if _, ok, err := r.LastBootstrap(); ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if _, ok, err := r.LastAppliedDaily(); ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestRevisionsRoundTrip(t *testing.T) {
	r := New(t.TempDir())
	in := map[law.RevisionID]law.Revision{
		"B_1": {ID: "B_1", LawID: "B", Title: "b"},
		"A_1": {ID: "A_1", LawID: "A", Title: "a,x"},
	}
	if err := r.SaveRevisions(in); err != nil {
		t.Fatal(err)
	}
	got, err := r.LoadRevisions()
	if err != nil || len(got) != 2 || got["A_1"].Title != "a,x" || got["B_1"].LawID != "B" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestXMLIndexRoundTrip(t *testing.T) {
	r := New(t.TempDir())
	in := map[law.RevisionID]law.XMLRecord{
		"A_1": {RevisionID: "A_1", SHA256: "s", Bytes: 42, ReleaseTag: "r1"},
	}
	if err := r.SaveXMLIndex(in); err != nil {
		t.Fatal(err)
	}
	got, err := r.LoadXMLIndex()
	if err != nil || len(got) != 1 || got["A_1"].Bytes != 42 || got["A_1"].ReleaseTag != "r1" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestLastAppliedDailyPicksMaxTo(t *testing.T) {
	r := New(t.TempDir())
	for _, rec := range []application.RunRecord{
		{Kind: "daily", StartedAt: "2026-09-12T22:00:00Z", To: "2026-09-11", Applied: true},
		{Kind: "daily", StartedAt: "2026-09-13T22:00:00Z", To: "2026-09-05", Applied: true}, // 手動の過去再処理
		{Kind: "daily", StartedAt: "2026-09-14T22:00:00Z", To: "2026-09-13", Applied: false},
	} {
		if err := r.SaveRun(rec); err != nil {
			t.Fatal(err)
		}
	}
	rec, ok, err := r.LastAppliedDaily()
	if err != nil || !ok || rec.To != "2026-09-11" {
		t.Fatalf("rec=%+v ok=%v err=%v", rec, ok, err)
	}
}

func TestLastDailyAndLastBootstrapPickNewestFile(t *testing.T) {
	r := New(t.TempDir())
	if err := r.SaveRun(application.RunRecord{Kind: "daily", StartedAt: "2026-09-11T22:00:00Z", To: "d1"}); err != nil {
		t.Fatal(err)
	}
	if err := r.SaveRun(application.RunRecord{Kind: "daily", StartedAt: "2026-09-12T22:00:00Z", To: "d2"}); err != nil {
		t.Fatal(err)
	}
	if err := r.SaveRun(application.RunRecord{Kind: "bootstrap", StartedAt: "2026-09-01T00:00:00Z", To: "b1"}); err != nil {
		t.Fatal(err)
	}
	daily, ok, err := r.LastDaily()
	if err != nil || !ok || daily.To != "d2" {
		t.Fatalf("daily=%+v ok=%v err=%v", daily, ok, err)
	}
	boot, ok, err := r.LastBootstrap()
	if err != nil || !ok || boot.To != "b1" {
		t.Fatalf("boot=%+v ok=%v err=%v", boot, ok, err)
	}
}

func TestSaveRunRejectsBadStartedAt(t *testing.T) {
	r := New(t.TempDir())
	if err := r.SaveRun(application.RunRecord{Kind: "daily", StartedAt: "not-a-time"}); err == nil {
		t.Fatal("want error for unparsable started_at")
	}
}

func TestLoadRejectsInvalidIDs(t *testing.T) {
	cases := []struct {
		name string
		file string
		body string
		load func(*Repo) error
	}{
		{
			name: "laws.csvのlaw_id",
			file: "laws.csv",
			body: "law_id,law_type,law_title,revision_id,updated,enforcement_date,repeal_status\n../x,Act,t,A_1,u,2020-01-01,None\n",
			load: func(r *Repo) error { _, err := r.LoadLaws(); return err },
		},
		{
			name: "laws.csvのrevision_id",
			file: "laws.csv",
			body: "law_id,law_type,law_title,revision_id,updated,enforcement_date,repeal_status\nA,Act,t,a/b,u,2020-01-01,None\n",
			load: func(r *Repo) error { _, err := r.LoadLaws(); return err },
		},
		{
			name: "revisions.csvのrevision_id",
			file: "revisions.csv",
			body: "revision_id,law_id,law_title,enforcement_date,promulgate_date,amendment_law_num,status,updated,first_seen\na/b,A,t,,,,CurrentEnforced,u,2026-09-11\n",
			load: func(r *Repo) error { _, err := r.LoadRevisions(); return err },
		},
		{
			name: "revisions.csvのlaw_id",
			file: "revisions.csv",
			body: "revision_id,law_id,law_title,enforcement_date,promulgate_date,amendment_law_num,status,updated,first_seen\nA_1,../x,t,,,,CurrentEnforced,u,2026-09-11\n",
			load: func(r *Repo) error { _, err := r.LoadRevisions(); return err },
		},
		{
			name: "xml_index.csvのrevision_id",
			file: "xml_index.csv",
			body: "revision_id,updated,sha256,xml_bytes,release_tag\n../x,u,s,1,t1\n",
			load: func(r *Repo) error { _, err := r.LoadXMLIndex(); return err },
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := New(t.TempDir())
			if err := os.WriteFile(filepath.Join(r.dir, c.file), []byte(c.body), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := c.load(r); err == nil {
				t.Fatal("want an error for an ID with a path separator")
			}
		})
	}
}
