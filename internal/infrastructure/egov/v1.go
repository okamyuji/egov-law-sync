package egov

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

// updateLawListsXML v1のupdatelawlists/{date}のレスポンス
type updateLawListsXML struct {
	ApplData struct {
		LawNameListInfo []struct {
			LawID string `xml:"LawId"`
		} `xml:"LawNameListInfo"`
	} `xml:"ApplData"`
}

// UpdatedLawIDs v1のupdatelawlistsから当日登録された法令IDを集める。404はfound=falseでerrなし
func (c *Client) UpdatedLawIDs(ctx context.Context, d law.Date) ([]law.LawID, bool, error) {
	url := fmt.Sprintf("%s/updatelawlists/%s", c.v1Base, compactDate(d))
	resp, cancel, notFound, err := c.retryGet(ctx, url, requestTimeout, http.StatusNotFound)
	if err != nil || notFound {
		return nil, false, err
	}
	defer cancel()
	defer resp.Body.Close()
	var data updateLawListsXML
	if err := xml.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, false, err
	}
	ids := make([]law.LawID, 0, len(data.ApplData.LawNameListInfo))
	for _, item := range data.ApplData.LawNameListInfo {
		ids = append(ids, law.LawID(item.LawID))
	}
	return ids, true, nil
}
