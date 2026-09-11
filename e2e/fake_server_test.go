package e2e

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

// wireLawInfo v2の/lawsのlaw_info
type wireLawInfo struct {
	LawID string `json:"law_id"`
	Type  string `json:"law_type"`
}

// wireRevisionInfo v2の/lawsと/law_revisionsで共通のrevision表現
type wireRevisionInfo struct {
	LawRevisionID            string `json:"law_revision_id"`
	LawTitle                 string `json:"law_title"`
	Updated                  string `json:"updated"`
	AmendmentEnforcementDate string `json:"amendment_enforcement_date"`
	AmendmentPromulgateDate  string `json:"amendment_promulgate_date"`
	AmendmentLawNum          string `json:"amendment_law_num"`
	RepealStatus             string `json:"repeal_status"`
	CurrentRevisionStatus    string `json:"current_revision_status"`
}

// wireLawItem /lawsの1件
type wireLawItem struct {
	LawInfo      wireLawInfo       `json:"law_info"`
	RevisionInfo *wireRevisionInfo `json:"revision_info"`
}

// wireLawsPage /lawsの1ページ
type wireLawsPage struct {
	TotalCount int           `json:"total_count"`
	Count      int           `json:"count"`
	Laws       []wireLawItem `json:"laws"`
}

// wireRevisionsResponse /law_revisions/{id}
type wireRevisionsResponse struct {
	Revisions []wireRevisionInfo `json:"revisions"`
}

// fakeServer e-Gov v2、v1、bulkdownloadを1つのhttptestサーバで模す。状態はテストごとに組み立てる
type fakeServer struct {
	mu             sync.Mutex
	laws           map[law.LawID]law.Law
	futureLaws     map[law.LawID]law.Law
	extraRevisions map[law.LawID][]wireRevisionInfo
	xmlBody        map[law.RevisionID][]byte
	sec1Override   map[law.RevisionID][]byte
	v1Found        map[law.Date][]law.LawID
	sec3Found      map[law.Date][]string
}

func newFakeServer() *fakeServer {
	return &fakeServer{
		laws:           map[law.LawID]law.Law{},
		futureLaws:     map[law.LawID]law.Law{},
		extraRevisions: map[law.LawID][]wireRevisionInfo{},
		xmlBody:        map[law.RevisionID][]byte{},
		sec1Override:   map[law.RevisionID][]byte{},
		v1Found:        map[law.Date][]law.LawID{},
		sec3Found:      map[law.Date][]string{},
	}
}

func (s *fakeServer) setLaw(l law.Law) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.laws[l.ID] = l
}

// setFuture asof指定（futureAsof）でこの法令が返す値。未設定ならsetLawの値をそのまま返す
func (s *fakeServer) setFuture(l law.Law) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.futureLaws[l.ID] = l
}

// setRevisions /law_revisions/{id}の応答を明示指定する。未設定ならlawsの現在値から1件だけ自動生成する
func (s *fakeServer) setRevisions(id law.LawID, revs []wireRevisionInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.extraRevisions[id] = revs
}

func (s *fakeServer) setXML(id law.RevisionID, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.xmlBody[id] = body
}

// setSec1Override sec1 zip内の1エントリだけをv2の応答と別の内容にする（破損・陳腐化の模擬）
func (s *fakeServer) setSec1Override(id law.RevisionID, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sec1Override[id] = body
}

// setV1 その日のupdatelawlistsに登録済みの一覧を設定する。未設定の日は404を返す
func (s *fakeServer) setV1(d law.Date, ids []law.LawID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.v1Found[d] = ids
}

func (s *fakeServer) start() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/laws", s.handleLaws)
	mux.HandleFunc("/law_revisions/", s.handleRevisions)
	mux.HandleFunc("/law_file/xml/", s.handleXML)
	mux.HandleFunc("/updatelawlists/", s.handleV1)
	mux.HandleFunc("/bulkdownload", s.handleBulk)
	return httptest.NewServer(mux)
}

// handleLaws /laws。asofが空なら現在値、futureAsofならfutureLawsで上書きした値を返す
func (s *fakeServer) handleLaws(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	asof := r.URL.Query().Get("asof")
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	ids := make([]string, 0, len(s.laws))
	for id := range s.laws {
		ids = append(ids, string(id))
	}
	slices.Sort(ids)
	page := wireLawsPage{TotalCount: len(ids)}
	for i, id := range ids {
		if i < offset {
			continue
		}
		l := s.laws[law.LawID(id)]
		if asof != "" {
			if fl, ok := s.futureLaws[law.LawID(id)]; ok {
				l = fl
			}
		}
		page.Laws = append(page.Laws, wireLawItem{
			LawInfo: wireLawInfo{LawID: id, Type: l.Type},
			RevisionInfo: &wireRevisionInfo{
				LawRevisionID:            string(l.RevisionID),
				LawTitle:                 l.Title,
				Updated:                  l.Updated,
				AmendmentEnforcementDate: l.EnforcementDate,
				RepealStatus:             l.RepealStatus,
			},
		})
	}
	page.Count = len(page.Laws)
	_ = json.NewEncoder(w).Encode(page)
}

// handleRevisions /law_revisions/{id}
func (s *fakeServer) handleRevisions(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := law.LawID(strings.TrimPrefix(r.URL.Path, "/law_revisions/"))
	if revs, ok := s.extraRevisions[id]; ok {
		_ = json.NewEncoder(w).Encode(wireRevisionsResponse{Revisions: revs})
		return
	}
	l, ok := s.laws[id]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	resp := wireRevisionsResponse{Revisions: []wireRevisionInfo{{
		LawRevisionID:            string(l.RevisionID),
		LawTitle:                 l.Title,
		Updated:                  l.Updated,
		AmendmentEnforcementDate: l.EnforcementDate,
		CurrentRevisionStatus:    "CurrentEnforced",
	}}}
	_ = json.NewEncoder(w).Encode(resp)
}

// handleXML /law_file/xml/{rev}
func (s *fakeServer) handleXML(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := law.RevisionID(strings.TrimPrefix(r.URL.Path, "/law_file/xml/"))
	body, ok := s.xmlBody[id]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	_, _ = w.Write(body)
}

// handleV1 /updatelawlists/{YYYYMMDD}。設定の無い日は404
func (s *fakeServer) handleV1(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := expandDate(strings.TrimPrefix(r.URL.Path, "/updatelawlists/"))
	ids, ok := s.v1Found[d]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0"?><DataRoot><Result><Code>0</Code></Result><ApplData>`)
	for _, id := range ids {
		sb.WriteString("<LawNameListInfo><LawId>" + string(id) + "</LawId></LawNameListInfo>")
	}
	sb.WriteString(`</ApplData></DataRoot>`)
	_, _ = w.Write([]byte(sb.String()))
}

// handleBulk /bulkdownload?file_section=1|3
func (s *fakeServer) handleBulk(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch r.URL.Query().Get("file_section") {
	case "3":
		d := expandDate(r.URL.Query().Get("update_date"))
		dirs, ok := s.sec3Found[d]
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		writeZip(w, sec3Entries(dirs))
	case "1":
		writeZip(w, s.sec1Entries())
	default:
		w.WriteHeader(http.StatusBadRequest)
	}
}

// sec1Entries 現在のlawsのrevision_idごとに<rev>/<rev>.xmlを積む。上書きが無ければv2と同じ内容にする
func (s *fakeServer) sec1Entries() map[string][]byte {
	entries := map[string][]byte{}
	for _, l := range s.laws {
		rev := l.RevisionID
		body, ok := s.sec1Override[rev]
		if !ok {
			body = s.xmlBody[rev]
		}
		entries[string(rev)+"/"+string(rev)+".xml"] = body
	}
	return entries
}

// sec3Entries dirsの各要素を最上位ディレクトリとして1ファイルだけ積む
func sec3Entries(dirs []string) map[string][]byte {
	entries := make(map[string][]byte, len(dirs))
	for _, d := range dirs {
		entries[d+"/"+d+".xml"] = []byte("<x/>")
	}
	return entries
}

// writeZip entriesをzipにまとめてwへ書く
func writeZip(w http.ResponseWriter, entries map[string][]byte) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	names := make([]string, 0, len(entries))
	for n := range entries {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		e, err := zw.Create(n)
		if err != nil {
			continue
		}
		_, _ = e.Write(entries[n])
	}
	_ = zw.Close()
	_, _ = w.Write(buf.Bytes())
}

// expandDate YYYYMMDDをYYYY-MM-DDにする。長さが違えば空文字（どの設定にも一致しないのでnot foundになる）
func expandDate(compact string) law.Date {
	if len(compact) != 8 {
		return ""
	}
	return law.Date(compact[0:4] + "-" + compact[4:6] + "-" + compact[6:8])
}
