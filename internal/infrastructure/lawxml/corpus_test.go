package lawxml

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

// corpusMeta EGOV_CORPUS_DIRのmeta.jsonの1件。実データでの検証用
type corpusMeta struct {
	LawID           law.LawID      `json:"law_id"`
	RevisionID      law.RevisionID `json:"revision_id"`
	Title           string         `json:"title"`
	EnforcementDate string         `json:"enforcement_date"`
}

// TestINV7CorpusFromDir EGOV_CORPUS_DIRにある実XML全件で、文の順序保存、Rt除外、md/jsonlの書き出しを確かめる。未設定なら飛ばす
func TestINV7CorpusFromDir(t *testing.T) {
	dir := os.Getenv("EGOV_CORPUS_DIR")
	if dir == "" {
		t.Skip("EGOV_CORPUS_DIR is not set")
	}
	body, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metas []corpusMeta
	if err := json.Unmarshal(body, &metas); err != nil {
		t.Fatal(err)
	}
	textDir := os.Getenv("EGOV_CORPUS_KEEP")
	if textDir == "" {
		textDir = t.TempDir()
	}
	for _, m := range metas {
		meta := law.Law{ID: m.LawID, Title: m.Title, RevisionID: m.RevisionID, EnforcementDate: m.EnforcementDate}
		rec, err := Renderer{}.Render(dir, textDir, meta)
		if err != nil {
			t.Fatalf("%s (%s): render: %v", m.Title, m.RevisionID, err)
		}
		md, err := os.ReadFile(filepath.Join(textDir, string(m.RevisionID)+".md"))
		if err != nil {
			t.Fatal(err)
		}
		assertAllSentencesInOrder(t, filepath.Join(dir, string(m.RevisionID)+".xml"), md)
		if rec.Chunks == 0 || !strings.Contains(string(md), "\n# "+m.Title+"\n") {
			t.Fatalf("%s: chunks=%d title heading missing", m.Title, rec.Chunks)
		}
		n := 0
		if err := (JSONL{}).Each(textDir, func(c law.Chunk) error {
			if c.RevisionID == m.RevisionID {
				n++
				if c.LawID == "" || c.Text == "" {
					t.Fatalf("%s: empty field in chunk %+v", m.Title, c)
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if n != rec.Chunks {
			t.Fatalf("%s: jsonl lines %d != chunks %d", m.Title, n, rec.Chunks)
		}
		t.Logf("%s: md=%dKB chunks=%d", m.Title, rec.MDBytes/1024, rec.Chunks)
	}
}
