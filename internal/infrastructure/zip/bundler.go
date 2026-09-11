package zip

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/okamyuji/egov-law-sync/internal/application"
	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

var _ application.ReleaseBundler = Bundler{}

// Bundler application.ReleaseBundlerの実装
type Bundler struct{}

// entry zipに入れる1ファイル。中身はpathから流し、bootstrapの数GBをメモリに載せない
type entry struct {
	name string
	path string
}

// Bundle xmlDir配下の<rev>.xmlとindex.csvをoutPathのzipにまとめる
func (Bundler) Bundle(xmlDir, outPath string, index []law.XMLRecord) error {
	var entries []entry
	rows := [][]string{}
	for _, rec := range index {
		p := filepath.Join(xmlDir, string(rec.RevisionID)+".xml")
		if _, ok, err := statIfExists(p); err != nil {
			return err
		} else if !ok {
			continue
		}
		entries = append(entries, entry{string(rec.RevisionID) + ".xml", p})
		rows = append(rows, []string{string(rec.RevisionID), rec.SHA256, strconv.FormatInt(rec.Bytes, 10)})
	}
	return writeRelease(outPath, entries, []string{"revision_id", "sha256", "xml_bytes"}, rows)
}

// BundleText textDir配下の<rev>.mdと<rev>.jsonlとindex.csvをoutPathのzipにまとめる。両方あるrevisionだけを載せる
func (Bundler) BundleText(textDir, outPath string, index []law.TextRecord) error {
	var entries []entry
	rows := [][]string{}
	for _, rec := range index {
		base := filepath.Join(textDir, string(rec.RevisionID))
		mdSize, okMD, err := statIfExists(base + ".md")
		if err != nil {
			return err
		}
		jlSize, okJL, err := statIfExists(base + ".jsonl")
		if err != nil {
			return err
		}
		if !okMD || !okJL {
			continue
		}
		chunks, err := countLines(base + ".jsonl")
		if err != nil {
			return err
		}
		entries = append(entries, entry{string(rec.RevisionID) + ".md", base + ".md"}, entry{string(rec.RevisionID) + ".jsonl", base + ".jsonl"})
		rows = append(rows, []string{string(rec.RevisionID), strconv.FormatInt(mdSize, 10), strconv.FormatInt(jlSize, 10), strconv.Itoa(chunks)})
	}
	return writeRelease(outPath, entries, []string{"revision_id", "md_bytes", "jsonl_bytes", "chunks"}, rows)
}

// statIfExists 無ければok=false。他のエラーはそのまま返す
func statIfExists(path string) (int64, bool, error) {
	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return fi.Size(), true, nil
}

// countLines index.csvのchunks列はjsonlの行数と一致させる
func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n := 0
	buf := make([]byte, 64<<10)
	for {
		c, err := f.Read(buf)
		n += bytes.Count(buf[:c], []byte("\n"))
		if errors.Is(err, io.EOF) {
			return n, nil
		}
		if err != nil {
			return 0, err
		}
	}
}

// writeRelease entriesとindex.csvをoutPathへ書く。途中で失敗したら部分的なzipを消す
func writeRelease(outPath string, entries []entry, header []string, rows [][]string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	err = writeEntries(f, entries, header, rows)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		// 途中まで書いたzipが残ると、ワークフローのrelease手順がそれをそのままReleaseに添付する
		_ = os.Remove(outPath)
	}
	return err
}

func writeEntries(f *os.File, entries []entry, header []string, rows [][]string) error {
	w := zip.NewWriter(f)
	err := addEntries(w, entries)
	if err == nil {
		err = addIndex(w, header, rows)
	}
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	return err
}

func addEntries(w *zip.Writer, entries []entry) error {
	for _, e := range entries {
		if err := addEntry(w, e); err != nil {
			return err
		}
	}
	return nil
}

func addEntry(w *zip.Writer, e entry) error {
	src, err := os.Open(e.path)
	if err != nil {
		return err
	}
	defer src.Close()
	zw, err := w.CreateHeader(&zip.FileHeader{Name: e.name, Method: zip.Deflate})
	if err != nil {
		return err
	}
	_, err = io.Copy(zw, src)
	return err
}

func addIndex(w *zip.Writer, header []string, rows [][]string) error {
	zw, err := w.CreateHeader(&zip.FileHeader{Name: "index.csv", Method: zip.Deflate})
	if err != nil {
		return err
	}
	cw := csv.NewWriter(zw)
	if err := cw.Write(header); err != nil {
		return err
	}
	if err := cw.WriteAll(rows); err != nil {
		return err
	}
	return cw.Error()
}
