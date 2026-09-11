package egov

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestListAllPagesUntilEmpty(t *testing.T) {
	var offsets []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offsets = append(offsets, r.URL.Query().Get("offset"))
		if r.URL.Query().Get("offset") == "0" {
			w.Write([]byte(`{"total_count":2,"count":2,"laws":[{"law_info":{"law_id":"A","law_type":"Act"},"revision_info":{"law_revision_id":"A_1","law_title":"a","updated":"u","amendment_enforcement_date":"2020-01-01","repeal_status":"None"}},{"law_info":{"law_id":"B","law_type":"Act"},"revision_info":null}]}`))
			return
		}
		w.Write([]byte(`{"total_count":0,"count":0,"laws":[]}`))
	}))
	defer srv.Close()
	laws, total, err := New(srv.URL, "", "").ListAll(context.Background(), "")
	if err != nil || total != 2 || len(laws) != 1 || laws[0].ID != "A" || len(offsets) != 2 {
		t.Fatalf("laws=%+v total=%d err=%v offsets=%v", laws, total, err, offsets)
	}
}

func TestRetriesThenFails(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(503) }))
	defer srv.Close()
	c := New(srv.URL, "", "")
	c.retryWait = func(int) time.Duration { return 0 }
	if _, _, err := c.ListAll(context.Background(), ""); err == nil || calls != 3 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestFetchXMLWritesFileAndHashes(t *testing.T) {
	body := []byte("<Law/>")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/law_file/xml/A_1" {
			w.WriteHeader(404)
			return
		}
		w.Write(body)
	}))
	defer srv.Close()
	dir := t.TempDir()
	sum, n, err := New(srv.URL, "", "").FetchXML(context.Background(), "A_1", dir)
	want := fmt.Sprintf("%x", sha256.Sum256(body))
	if err != nil || sum != want || n != int64(len(body)) {
		t.Fatalf("sum=%s n=%d err=%v", sum, n, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "A_1.xml")); err != nil {
		t.Fatal(err)
	}
}

func TestListAllStopsWhenServerNeverEmpties(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(`{"total_count":6000,"count":1,"laws":[{"law_info":{"law_id":"A","law_type":"Act"},"revision_info":{"law_revision_id":"A_1","law_title":"a","updated":"u","amendment_enforcement_date":"2020-01-01","repeal_status":"None"}}]}`))
	}))
	defer srv.Close()
	_, _, err := New(srv.URL, "", "").ListAll(context.Background(), "")
	if err == nil {
		t.Fatal("expected an error when the server never returns count=0")
	}
	if calls > 5 {
		t.Fatalf("expected the loop to terminate within a small bounded number of pages, got calls=%d", calls)
	}
}

func TestRevisionsSkipsNothingAndMapsFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/law_revisions/A" {
			w.WriteHeader(404)
			return
		}
		w.Write([]byte(`{"law_info":{"law_id":"A"},"revisions":[{"law_revision_id":"A_1","law_title":"t","updated":"u","amendment_enforcement_date":"2020-01-01","amendment_promulgate_date":"2019-12-01","amendment_law_num":"n","current_revision_status":"CurrentEnforced"}]}`))
	}))
	defer srv.Close()
	revs, found, err := New(srv.URL, "", "").Revisions(context.Background(), "A")
	if err != nil || !found || len(revs) != 1 || revs[0].ID != "A_1" || revs[0].LawID != "A" || revs[0].Status != "CurrentEnforced" {
		t.Fatalf("revs=%+v found=%v err=%v", revs, found, err)
	}
}

func TestListAllRejectsInvalidIDs(t *testing.T) {
	cases := map[string]string{
		"law_id":      `{"total_count":1,"count":1,"laws":[{"law_info":{"law_id":"../x","law_type":"Act"},"revision_info":{"law_revision_id":"A_1","law_title":"a","updated":"u","amendment_enforcement_date":"2020-01-01","repeal_status":"None"}}]}`,
		"revision_id": `{"total_count":1,"count":1,"laws":[{"law_info":{"law_id":"A","law_type":"Act"},"revision_info":{"law_revision_id":"a/b","law_title":"a","updated":"u","amendment_enforcement_date":"2020-01-01","repeal_status":"None"}}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte(body))
			}))
			defer srv.Close()
			if _, _, err := New(srv.URL, "", "").ListAll(context.Background(), ""); err == nil {
				t.Fatal("want an error for an ID with a path separator")
			}
		})
	}
}

func TestRevisionsRejectsInvalidRevisionID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"revisions":[{"law_revision_id":"../x","law_title":"t","updated":"u"}]}`))
	}))
	defer srv.Close()
	if _, _, err := New(srv.URL, "", "").Revisions(context.Background(), "A"); err == nil {
		t.Fatal("want an error for an ID with a path separator")
	}
}

func TestRevisionsEscapesIDInPath(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.EscapedPath()
		w.WriteHeader(404)
	}))
	defer srv.Close()
	if _, _, err := New(srv.URL, "", "").Revisions(context.Background(), "a/b"); err != nil {
		t.Fatal(err)
	}
	if got != "/law_revisions/a%2Fb" {
		t.Fatalf("path=%q", got)
	}
}

func TestRevisionsTreats404AsGone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()
	revs, found, err := New(srv.URL, "", "").Revisions(context.Background(), "A")
	if err != nil || found || revs != nil {
		t.Fatalf("revs=%+v found=%v err=%v", revs, found, err)
	}
}

func TestListAll404StaysAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()
	if _, _, err := New(srv.URL, "", "").ListAll(context.Background(), ""); err == nil {
		t.Fatal("404 on /laws must stay an error")
	}
}
