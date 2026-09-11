package lawxml

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/okamyuji/egov-law-sync/internal/application"
	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

var _ application.TextRenderer = Renderer{}

// Renderer application.TextRendererの実装
type Renderer struct{}

// Render xmlDirのXMLを読み、textDirへmdとjsonlを書く。両方を一時ファイルに書いてからrenameし、片方だけが残る状態を作らない
func (Renderer) Render(xmlDir, textDir string, meta law.Law) (law.TextRecord, error) {
	f, err := os.Open(filepath.Join(xmlDir, string(meta.RevisionID)+".xml"))
	if err != nil {
		return law.TextRecord{}, err
	}
	defer f.Close()
	md, chunks, err := Convert(f, meta)
	if err != nil {
		return law.TextRecord{}, err
	}
	jsonl, err := encodeJSONL(chunks)
	if err != nil {
		return law.TextRecord{}, err
	}
	if err := os.MkdirAll(textDir, 0o755); err != nil {
		return law.TextRecord{}, err
	}
	base := filepath.Join(textDir, string(meta.RevisionID))
	if err := writePair(base+".md", md, base+".jsonl", jsonl); err != nil {
		return law.TextRecord{}, err
	}
	return law.TextRecord{
		RevisionID: meta.RevisionID,
		MDBytes:    int64(len(md)),
		JSONLBytes: int64(len(jsonl)),
		Chunks:     len(chunks),
	}, nil
}

// writePair 2つのファイルを一時名に書いてから順にrenameする。2つ目のrenameに失敗したら1つ目を消す
func writePair(p1 string, b1 []byte, p2 string, b2 []byte) error {
	t1, t2 := p1+".tmp", p2+".tmp"
	if err := os.WriteFile(t1, b1, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(t2, b2, 0o644); err != nil {
		_ = os.Remove(t1)
		return err
	}
	if err := os.Rename(t1, p1); err != nil {
		_ = os.Remove(t1)
		_ = os.Remove(t2)
		return err
	}
	if err := os.Rename(t2, p2); err != nil {
		_ = os.Remove(p1)
		_ = os.Remove(t2)
		return err
	}
	return nil
}

// encodeJSONL 1行1Chunk。HTMLエスケープは切り、本文の「<」をそのまま残す
func encodeJSONL(chunks []law.Chunk) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, c := range chunks {
		if err := enc.Encode(c); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}
