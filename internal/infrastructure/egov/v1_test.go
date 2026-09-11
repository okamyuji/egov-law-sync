package egov

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUpdatedLawIDs404IsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/updatelawlists/20260910":
			w.Write([]byte(`<?xml version="1.0"?><DataRoot><Result><Code>0</Code></Result><ApplData><LawNameListInfo><LawId>A</LawId></LawNameListInfo><LawNameListInfo><LawId>B</LawId></LawNameListInfo></ApplData></DataRoot>`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := New("", srv.URL, "")
	ids, found, err := c.UpdatedLawIDs(context.Background(), "2026-09-10")
	if err != nil || !found || len(ids) != 2 {
		t.Fatalf("ids=%v found=%v err=%v", ids, found, err)
	}
	_, found, err = c.UpdatedLawIDs(context.Background(), "2026-09-05")
	if err != nil || found {
		t.Fatalf("404 must be not found: found=%v err=%v", found, err)
	}
}
