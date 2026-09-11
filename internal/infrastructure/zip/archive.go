package zip

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"slices"
	"strings"

	"github.com/okamyuji/egov-law-sync/internal/application"
	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

// bulkArchive OpenBulkの戻り値。<rev>/<rev>.xmlのSHA256を開いた時点で読み切って保持する
type bulkArchive struct {
	hashes map[law.RevisionID]string
}

// OpenBulk sec1のzipを開き、<rev>/<rev>.xmlのSHA256を全件読み込む
func OpenBulk(path string) (application.Archive, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	hashes := map[law.RevisionID]string{}
	for _, f := range r.File {
		rev, ok := revisionXMLName(f.Name)
		if !ok {
			continue
		}
		sum, err := hashEntry(f)
		if err != nil {
			return nil, err
		}
		hashes[rev] = sum
	}
	return &bulkArchive{hashes: hashes}, nil
}

// revisionXMLName "<rev>/<rev>.xml"の形であればrevを返す
func revisionXMLName(name string) (law.RevisionID, bool) {
	i := strings.IndexByte(name, '/')
	if i <= 0 {
		return "", false
	}
	dir, rest := name[:i], name[i+1:]
	if rest != dir+".xml" {
		return "", false
	}
	return law.RevisionID(dir), true
}

// hashEntry zipエントリの内容全体をSHA256にする
func hashEntry(f *zip.File) (string, error) {
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()
	h := sha256.New()
	if _, err := io.Copy(h, rc); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// SHA256 revisionIDのハッシュを返す。無ければok=false
func (a *bulkArchive) SHA256(id law.RevisionID) (string, bool, error) {
	sum, ok := a.hashes[id]
	return sum, ok, nil
}

// RevisionIDs 保持しているrevision IDをソート済みで返す
func (a *bulkArchive) RevisionIDs() []law.RevisionID {
	ids := make([]law.RevisionID, 0, len(a.hashes))
	for id := range a.hashes {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// Close 開いた時点で読み切っているので何もしない
func (a *bulkArchive) Close() error { return nil }

// ListDirs zip内の最上位ディレクトリ名を重複無しソート済みで返す
func ListDirs(path string) ([]string, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	set := map[string]bool{}
	for _, f := range r.File {
		if i := strings.IndexByte(f.Name, '/'); i > 0 {
			set[f.Name[:i]] = true
		}
	}
	dirs := make([]string, 0, len(set))
	for d := range set {
		dirs = append(dirs, d)
	}
	slices.Sort(dirs)
	return dirs, nil
}
