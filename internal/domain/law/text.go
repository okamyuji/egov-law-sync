package law

// Chunk 条1つ分のテキストとメタデータ。JSONLの1行に対応する
type Chunk struct {
	LawID           LawID      `json:"law_id"`
	RevisionID      RevisionID `json:"revision_id"`
	LawTitle        string     `json:"law_title"`
	LawNum          string     `json:"law_num"`
	EnforcementDate string     `json:"enforcement_date"`
	Path            string     `json:"path"`
	Article         string     `json:"article"`
	ArticleTitle    string     `json:"article_title"`
	Text            string     `json:"text"`
	SourceURL       string     `json:"source_url"`
}

// TextRecord laws-text.zipのindex.csvの1行
type TextRecord struct {
	RevisionID RevisionID
	MDBytes    int64
	JSONLBytes int64
	Chunks     int
}

// SourceURL 法令IDからe-Gov法令検索の閲覧URLを作る
func SourceURL(id LawID) string {
	return "https://laws.e-gov.go.jp/law/" + string(id)
}
