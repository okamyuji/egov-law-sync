package egov

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"testing"
	"time"
)

// TestRetryGetDrainsBodyForConnectionReuse 5xxの応答本文を読み切ってから閉じないと、Goのトランスポートはkeep-alive接続をプールに返さない。読み切っていれば、同じretryGet呼び出し内の再試行は接続を再利用する
func TestRetryGetDrainsBodyForConnectionReuse(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(503)
			_, _ = w.Write([]byte("temporarily unavailable, please retry later"))
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := New(srv.URL, "", "")
	c.retryWait = func(int) time.Duration { return 0 }

	var reused []bool
	trace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) { reused = append(reused, info.Reused) },
	}
	ctx := httptrace.WithClientTrace(context.Background(), trace)

	resp, cancel, notFound, err := c.retryGet(ctx, srv.URL, requestTimeout, 0)
	if err != nil || notFound {
		t.Fatalf("err=%v notFound=%v", err, notFound)
	}
	defer cancel()
	defer resp.Body.Close()

	if len(reused) != 2 || !reused[1] {
		t.Fatalf("expected the second attempt to reuse the connection after draining the 5xx body, got reused=%v", reused)
	}
}
