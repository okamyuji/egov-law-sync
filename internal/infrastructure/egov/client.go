// Package egov e-Gov法令API v2、v1、bulkdownloadへのHTTPクライアント
package egov

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/okamyuji/egov-law-sync/internal/application"
	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

// requestTimeout sec1以外の既定タイムアウト
const requestTimeout = 60 * time.Second

// Client e-Gov API v2、v1、bulkdownloadへのHTTPクライアント
type Client struct {
	http      *http.Client
	v2Base    string
	v1Base    string
	bulkBase  string
	retryWait func(attempt int) time.Duration
}

var (
	_ application.LawCatalog     = (*Client)(nil)
	_ application.RevisionSource = (*Client)(nil)
	_ application.XMLSource      = (*Client)(nil)
	_ application.UpdateList     = (*Client)(nil)
	_ application.DailyArchive   = (*Client)(nil)
	_ application.BulkArchive    = (*Client)(nil)
)

// New base URL3つを設定したClientを作る。retryはtransportエラーと5xxを対象に2秒、4秒の間隔で最大3回試行する
func New(v2Base, v1Base, bulkBase string) *Client {
	return &Client{
		http:     &http.Client{},
		v2Base:   v2Base,
		v1Base:   v1Base,
		bulkBase: bulkBase,
		retryWait: func(attempt int) time.Duration {
			return time.Duration(attempt) * 2 * time.Second
		},
	}
}

// retryGet urlをGETする。notFoundに一致するステータスは即座にisNotFound=trueで返し、リトライしない。それ以外の5xxとトランスポートエラーは最大3回試行する
func (c *Client) retryGet(ctx context.Context, url string, timeout time.Duration, notFound int) (resp *http.Response, cancel context.CancelFunc, isNotFound bool, err error) {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		res, cancelFn, retryErr := c.attempt(ctx, url, timeout)
		switch {
		case retryErr != nil:
			lastErr = retryErr
		case notFound != 0 && res.StatusCode == notFound:
			discardAndClose(res)
			cancelFn()
			return nil, nil, true, nil
		case res.StatusCode >= 500:
			discardAndClose(res)
			cancelFn()
			lastErr = fmt.Errorf("egov: %s returned %d", url, res.StatusCode)
		case res.StatusCode >= 400:
			discardAndClose(res)
			cancelFn()
			return nil, nil, false, fmt.Errorf("egov: %s returned %d", url, res.StatusCode)
		default:
			return res, cancelFn, false, nil
		}
		if attempt < 3 {
			time.Sleep(c.retryWait(attempt))
		}
	}
	return nil, nil, false, lastErr
}

// discardAndClose 破棄する応答本文を読み切ってから閉じる。読み切らずに閉じるとkeep-alive接続が再利用されない
func discardAndClose(res *http.Response) {
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
}

// attempt 1回分のGETを実行する
func (c *Client) attempt(ctx context.Context, url string, timeout time.Duration) (*http.Response, context.CancelFunc, error) {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("egov: request failed: %w", err)
	}
	return res, cancel, nil
}

// compactDate YYYY-MM-DDをYYYYMMDDにする
func compactDate(d law.Date) string {
	s := string(d)
	return s[0:4] + s[5:7] + s[8:10]
}
