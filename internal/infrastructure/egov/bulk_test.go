package egov

import (
	archivezip "archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRevisionDirs500IsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer srv.Close()
	_, found, err := New("", "", srv.URL).RevisionDirs(context.Background(), "2026-09-05")
	if err != nil || found {
		t.Fatalf("found=%v err=%v", found, err)
	}
}

func bulkZipBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := archivezip.NewWriter(&buf)
	for name, body := range map[string][]byte{
		"all_law_list.csv": []byte("x"),
		"A_1/A_1.xml":      []byte("<a/>"),
		"B_1/B_1.xml":      []byte("<b/>"),
	} {
		e, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestRevisionDirsListsTopLevelDirs(t *testing.T) {
	body := bulkZipBytes(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("file_section") != "3" {
			w.WriteHeader(400)
			return
		}
		w.Write(body)
	}))
	defer srv.Close()
	dirs, found, err := New("", "", srv.URL).RevisionDirs(context.Background(), "2026-09-10")
	if err != nil || !found || len(dirs) != 2 || dirs[0] != "A_1" || dirs[1] != "B_1" {
		t.Fatalf("dirs=%v found=%v err=%v", dirs, found, err)
	}
}

func TestFetchAllOpensArchive(t *testing.T) {
	body := bulkZipBytes(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("file_section") != "1" {
			w.WriteHeader(400)
			return
		}
		w.Write(body)
	}))
	defer srv.Close()
	a, err := New("", "", srv.URL).FetchAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if ids := a.RevisionIDs(); len(ids) != 2 {
		t.Fatalf("ids=%v", ids)
	}
	if sum, ok, err := a.SHA256("A_1"); err != nil || !ok || sum == "" {
		t.Fatalf("sum=%s ok=%v err=%v", sum, ok, err)
	}
}
