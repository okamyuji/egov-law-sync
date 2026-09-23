package egov

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestScopedCatalogAndTitle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("offset") != "0" {
			w.Write([]byte(`{"total_count":0,"count":0,"laws":[]}`))
			return
		}
		w.Write([]byte(`{"total_count":2,"count":2,"laws":[{"law_info":{"law_id":"A","law_type":"Act"},"revision_info":{"law_revision_id":"A_1","law_title":"基本法","abbrev":"税法"}},{"law_info":{"law_id":"AB","law_type":"Act"},"revision_info":{"law_revision_id":"AB_1","law_title":"税法施行令","abbrev":"税法施行令"}}]}`))
	}))
	defer server.Close()
	client := New(server.URL, "", "")
	laws, total, err := client.ListByID(context.Background(), "", "A")
	if err != nil || total != 1 || len(laws) != 1 || laws[0].ID != "A" {
		t.Fatalf("laws=%v total=%d err=%v", laws, total, err)
	}
	id, err := client.ResolveTitle(context.Background(), "税法")
	if err != nil || id != "A" {
		t.Fatalf("id=%s err=%v", id, err)
	}
	if _, err := client.ResolveTitle(context.Background(), "不明"); err == nil || !strings.Contains(err.Error(), "--law-id") {
		t.Fatalf("err=%v", err)
	}
}
