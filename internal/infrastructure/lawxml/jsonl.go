package lawxml

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/okamyuji/egov-law-sync/internal/application"
	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

var _ application.ChunkSource = JSONL{}

// JSONL application.ChunkSourceの実装。dir直下の*.jsonlをファイル名順に読む
type JSONL struct{}

// maxLineBytes 1行の上限。最大17MBの法令でも1条はこれより短い
const maxLineBytes = 16 << 20

// Each *.jsonlをファイル名順に開き、1行ずつfnへ渡す。壊れた行かfnのerrorでその場で止まる
func (JSONL) Each(dir string, fn func(law.Chunk) error) error {
	// 存在しないディレクトリは0件ではなく誤りとして返す。打ち間違いを無言で成功させないため
	if _, err := os.Stat(dir); err != nil {
		return err
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return err
	}
	for _, p := range paths {
		if err := eachLine(p, fn); err != nil {
			return err
		}
	}
	return nil
}

func eachLine(path string, fn func(law.Chunk) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), maxLineBytes)
	for n := 1; sc.Scan(); n++ {
		var c law.Chunk
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			return fmt.Errorf("%s:%d: %w", filepath.Base(path), n, err)
		}
		if err := fn(c); err != nil {
			return err
		}
	}
	return sc.Err()
}
