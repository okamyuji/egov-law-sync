package egov

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

// listLimit /lawsの1ページあたり件数。9567件を2ページで取り切れる上限値
const listLimit = 5000

// lawsPage /lawsの1ページ分
type lawsPage struct {
	TotalCount int `json:"total_count"`
	Count      int `json:"count"`
	Laws       []struct {
		LawInfo struct {
			LawID string `json:"law_id"`
			Type  string `json:"law_type"`
		} `json:"law_info"`
		RevisionInfo *revisionInfo `json:"revision_info"`
	} `json:"laws"`
}

// revisionInfo /lawsと/law_revisionsで共通のrevision表現
type revisionInfo struct {
	LawRevisionID            string `json:"law_revision_id"`
	LawTitle                 string `json:"law_title"`
	Updated                  string `json:"updated"`
	AmendmentEnforcementDate string `json:"amendment_enforcement_date"`
	AmendmentPromulgateDate  string `json:"amendment_promulgate_date"`
	AmendmentLawNum          string `json:"amendment_law_num"`
	RepealStatus             string `json:"repeal_status"`
	CurrentRevisionStatus    string `json:"current_revision_status"`
}

// revisionsResponse /law_revisions/{id}
type revisionsResponse struct {
	Revisions []revisionInfo `json:"revisions"`
}

// ListAll /lawsを全件取得する。1ページ目のtotal_countを返す。revision_infoが無い行は飛ばす。countが0にならないまま応答し続けるサーバーへの対策として、offsetがtotalを超えるか、(total/limit)+2ページを超えたら打ち切る
func (c *Client) ListAll(ctx context.Context, asof string) ([]law.Law, int, error) {
	var laws []law.Law
	total := 0
	offset := 0
	maxPages := 2
	for page := 0; ; page++ {
		url := fmt.Sprintf("%s/laws?limit=%d&offset=%d&response_format=json", c.v2Base, listLimit, offset)
		if asof != "" {
			url += "&asof=" + asof
		}
		var p lawsPage
		if err := c.getJSON(ctx, url, &p); err != nil {
			return nil, 0, err
		}
		if page == 0 {
			total = p.TotalCount
			maxPages = total/listLimit + 2
		}
		for _, item := range p.Laws {
			if item.RevisionInfo == nil {
				continue
			}
			laws = append(laws, law.Law{
				ID:              law.LawID(item.LawInfo.LawID),
				Type:            item.LawInfo.Type,
				Title:           item.RevisionInfo.LawTitle,
				RevisionID:      law.RevisionID(item.RevisionInfo.LawRevisionID),
				Updated:         item.RevisionInfo.Updated,
				EnforcementDate: item.RevisionInfo.AmendmentEnforcementDate,
				RepealStatus:    item.RevisionInfo.RepealStatus,
			})
		}
		if p.Count == 0 {
			break
		}
		offset += p.Count
		// offset==totalちょうどは最後の実データページとして正常なので、確認のための次ページ取得（count=0が返るはず）を許す。それを超えて続く場合とページ数上限に達した場合だけ打ち切る
		if offset > total || page+1 >= maxPages {
			if offset <= total {
				return nil, 0, fmt.Errorf("egov: pagination did not terminate after %d pages", page+1)
			}
			break
		}
	}
	return laws, total, nil
}

// Revisions /law_revisions/{id}を取得する
func (c *Client) Revisions(ctx context.Context, id law.LawID) ([]law.Revision, error) {
	url := fmt.Sprintf("%s/law_revisions/%s?response_format=json", c.v2Base, id)
	var r revisionsResponse
	if err := c.getJSON(ctx, url, &r); err != nil {
		return nil, err
	}
	out := make([]law.Revision, 0, len(r.Revisions))
	for _, item := range r.Revisions {
		out = append(out, law.Revision{
			ID:              law.RevisionID(item.LawRevisionID),
			LawID:           id,
			Title:           item.LawTitle,
			EnforcementDate: item.AmendmentEnforcementDate,
			PromulgateDate:  item.AmendmentPromulgateDate,
			AmendmentLawNum: item.AmendmentLawNum,
			Status:          item.CurrentRevisionStatus,
			Updated:         item.Updated,
		})
	}
	return out, nil
}

// FetchXML /law_file/xml/{id}をdirに<id>.xmlとして保存し、sha256とバイト数を返す
func (c *Client) FetchXML(ctx context.Context, id law.RevisionID, dir string) (string, int64, error) {
	url := fmt.Sprintf("%s/law_file/xml/%s", c.v2Base, id)
	resp, cancel, _, err := c.retryGet(ctx, url, requestTimeout, 0)
	if err != nil {
		return "", 0, err
	}
	defer cancel()
	defer resp.Body.Close()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, err
	}
	f, err := os.Create(filepath.Join(dir, string(id)+".xml"))
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), resp.Body)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// getJSON urlをGETしてvにデコードする
func (c *Client) getJSON(ctx context.Context, url string, v any) error {
	resp, cancel, _, err := c.retryGet(ctx, url, requestTimeout, 0)
	if err != nil {
		return err
	}
	defer cancel()
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}
