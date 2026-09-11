package law

import (
	"encoding/json"
	"testing"
)

func TestINV5ChunkJSONKeys(t *testing.T) {
	c := Chunk{LawID: "322AC0000000049", RevisionID: "322AC0000000049_20260717_508AC0000000060", LawTitle: "労働基準法",
		LawNum: "昭和二十二年法律第四十九号", EnforcementDate: "2026-07-17", Path: "第一章", Article: "第一条",
		ArticleTitle: "労働条件の原則", Text: "本文", SourceURL: SourceURL("322AC0000000049")}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"law_id":"322AC0000000049","revision_id":"322AC0000000049_20260717_508AC0000000060","law_title":"労働基準法","law_num":"昭和二十二年法律第四十九号","enforcement_date":"2026-07-17","path":"第一章","article":"第一条","article_title":"労働条件の原則","text":"本文","source_url":"https://laws.e-gov.go.jp/law/322AC0000000049"}`
	if string(b) != want {
		t.Fatalf("got %s", b)
	}
	var back Chunk
	if err := json.Unmarshal(b, &back); err != nil || back != c {
		t.Fatalf("round trip: %+v err=%v", back, err)
	}
}

func TestSourceURL(t *testing.T) {
	if got := SourceURL("A1"); got != "https://laws.e-gov.go.jp/law/A1" {
		t.Fatal(got)
	}
}
