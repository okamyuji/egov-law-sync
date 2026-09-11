package zip

import (
	archivezip "archive/zip"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func writeZip(t *testing.T, entries map[string][]byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "x.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := archivezip.NewWriter(f)
	for name, body := range entries {
		e, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOpenBulkHashesByRevision(t *testing.T) {
	p := writeZip(t, map[string][]byte{"all_law_list.csv": []byte("x"), "A_1/A_1.xml": []byte("<a/>"), "B_1/B_1.xml": []byte("<b/>")})
	a, err := OpenBulk(p)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	sum, ok, err := a.SHA256("A_1")
	if err != nil || !ok || sum != fmt.Sprintf("%x", sha256.Sum256([]byte("<a/>"))) {
		t.Fatalf("sum=%s ok=%v err=%v", sum, ok, err)
	}
	if _, ok, _ := a.SHA256("Z_9"); ok {
		t.Fatal("missing revision must be ok=false")
	}
	if ids := a.RevisionIDs(); len(ids) != 2 {
		t.Fatalf("ids=%v", ids)
	}
}

func TestListDirsSortedNoDuplicates(t *testing.T) {
	p := writeZip(t, map[string][]byte{
		"all_law_list.csv": []byte("x"),
		"B_1/B_1.xml":      []byte("<b/>"),
		"A_1/A_1.xml":      []byte("<a/>"),
		"A_1/extra.txt":    []byte("y"),
	})
	dirs, err := ListDirs(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 2 || dirs[0] != "A_1" || dirs[1] != "B_1" {
		t.Fatalf("dirs=%v", dirs)
	}
}
