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
	err = writeEntries(f, xmlDir, index)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		// 途中まで書いたzipが残ると、ワークフローのrelease手順がそれをそのままReleaseに添付する
		_ = os.Remove(outPath)
	}
	return err
}

// writeEntries fへzipの中身を書き、zip.Writerを閉じる
func writeEntries(f *os.File, xmlDir string, index []law.XMLRecord) error {
	w := zip.NewWriter(f)
	written, err := writeXMLEntries(w, xmlDir, index)
	if err == nil {
		err = writeIndexEntry(w, written)
	}
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	return err
}

// writeXMLEntries xmlDir内に存在する<rev>.xmlをzipに平坦に格納し、格納できた記録を返す
func writeXMLEntries(w *zip.Writer, xmlDir string, index []law.XMLRecord) ([]law.XMLRecord, error) {
	written := make([]law.XMLRecord, 0, len(index))
	for _, rec := range index {
		name := string(rec.RevisionID) + ".xml"
		body, err := os.ReadFile(filepath.Join(xmlDir, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		e, err := w.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err != nil {
			return nil, err
		}
		if _, err := e.Write(body); err != nil {
			return nil, err
		}
		written = append(written, rec)
	}
	return written, nil
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
