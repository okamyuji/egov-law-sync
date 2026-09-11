package egov

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/okamyuji/egov-law-sync/internal/application"
	"github.com/okamyuji/egov-law-sync/internal/domain/law"
	"github.com/okamyuji/egov-law-sync/internal/infrastructure/zip"
)

// bulkTimeout sec1（全件）のタイムアウト。324MBの実測32.6sに余裕を持たせる
const bulkTimeout = 600 * time.Second

// RevisionDirs sec3（日次更新zip）を取得し、最上位ディレクトリ名の一覧を返す。500はfound=falseでerrなし
func (c *Client) RevisionDirs(ctx context.Context, d law.Date) ([]string, bool, error) {
	url := fmt.Sprintf("%s/bulkdownload?file_section=3&update_date=%s&only_xml_flag=true", c.bulkBase, compactDate(d))
	path, found, err := c.downloadToTemp(ctx, url, requestTimeout, http.StatusInternalServerError)
	if err != nil || !found {
		return nil, found, err
	}
	defer os.Remove(path)
	dirs, err := zip.ListDirs(path)
	if err != nil {
		return nil, false, err
	}
	return dirs, true, nil
}

// FetchAll sec1（全件zip）を一時ファイルへストリーム保存してから開く
func (c *Client) FetchAll(ctx context.Context) (application.Archive, error) {
	url := fmt.Sprintf("%s/bulkdownload?file_section=1&only_xml_flag=true", c.bulkBase)
	path, _, err := c.downloadToTemp(ctx, url, bulkTimeout, 0)
	if err != nil {
		return nil, err
	}
	defer os.Remove(path)
	return zip.OpenBulk(path)
}

// downloadToTemp レスポンスボディを一時ファイルへ保存し、そのパスを返す。notFoundStatusに一致した場合はfound=false
func (c *Client) downloadToTemp(ctx context.Context, url string, timeout time.Duration, notFoundStatus int) (string, bool, error) {
	resp, cancel, notFound, err := c.retryGet(ctx, url, timeout, notFoundStatus)
	if err != nil {
		return "", false, err
	}
	if notFound {
		return "", false, nil
	}
	defer cancel()
	defer resp.Body.Close()
	f, err := os.CreateTemp("", "egov-bulk-*.zip")
	if err != nil {
		return "", false, err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(f.Name())
		return "", false, err
	}
	return f.Name(), true, nil
}
