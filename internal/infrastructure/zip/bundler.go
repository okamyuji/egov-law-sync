package zip

import (
	"archive/zip"
	"encoding/csv"
	"os"
	"path/filepath"
	"strconv"

	"github.com/okamyuji/egov-law-sync/internal/application"
	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

var _ application.ReleaseBundler = Bundler{}

// Bundler application.ReleaseBundlerの実装
type Bundler struct{}

// Bundle xmlDir配下の<rev>.xmlとindex.csvをoutPathのzipにまとめる
func (Bundler) Bundle(xmlDir, outPath string, index []law.XMLRecord) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	w := zip.NewWriter(f)
	if err := writeXMLEntries(w, xmlDir, index); err == nil {
		err = writeIndexEntry(w, index)
	}
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// writeXMLEntries xmlDir内に存在する<rev>.xmlをzipに平坦に格納する
func writeXMLEntries(w *zip.Writer, xmlDir string, index []law.XMLRecord) error {
	for _, rec := range index {
		name := string(rec.RevisionID) + ".xml"
		body, err := os.ReadFile(filepath.Join(xmlDir, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		e, err := w.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err != nil {
			return err
		}
		if _, err := e.Write(body); err != nil {
			return err
		}
	}
	return nil
}

// writeIndexEntry revision_id,sha256,xml_bytesのindex.csvをzipに格納する
func writeIndexEntry(w *zip.Writer, index []law.XMLRecord) error {
	e, err := w.CreateHeader(&zip.FileHeader{Name: "index.csv", Method: zip.Deflate})
	if err != nil {
		return err
	}
	cw := csv.NewWriter(e)
	if err := cw.Write([]string{"revision_id", "sha256", "xml_bytes"}); err != nil {
		return err
	}
	for _, rec := range index {
		row := []string{string(rec.RevisionID), rec.SHA256, strconv.FormatInt(rec.Bytes, 10)}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
