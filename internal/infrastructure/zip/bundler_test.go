package zip

import (
	archivezip "archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

func TestBundleContainsXMLAndIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "A_1.xml"), []byte("<a/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "laws-xml.zip")
	err := Bundler{}.Bundle(dir, out, []law.XMLRecord{{RevisionID: "A_1", SHA256: "s", Bytes: 4}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := archivezip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	names := map[string]bool{}
	for _, f := range r.File {
		names[f.Name] = true
	}
	if !names["A_1.xml"] || !names["index.csv"] {
		t.Fatalf("names=%v", names)
	}
}

func TestBundleSkipsMissingXML(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(t.TempDir(), "nested", "laws-xml.zip")
	index := []law.XMLRecord{{RevisionID: "MISSING", SHA256: "s", Bytes: 0}}
	if err := (Bundler{}).Bundle(dir, out, index); err != nil {
		t.Fatal(err)
	}
	r, err := archivezip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	names := map[string]bool{}
	for _, f := range r.File {
		names[f.Name] = true
	}
	if names["MISSING.xml"] {
		t.Fatal("missing xml must not be bundled")
	}
	if !names["index.csv"] {
		t.Fatal("index.csv must always be bundled")
	}
}

func TestBundleWritesIndexCSVRows(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "A_1.xml"), []byte("<a/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "laws-xml.zip")
	index := []law.XMLRecord{{RevisionID: "A_1", SHA256: "deadbeef", Bytes: 4}}
	if err := (Bundler{}).Bundle(dir, out, index); err != nil {
		t.Fatal(err)
	}
	r, err := archivezip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var body []byte
	for _, f := range r.File {
		if f.Name == "index.csv" {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer rc.Close()
			buf, err := io.ReadAll(rc)
			if err != nil {
				t.Fatal(err)
			}
			body = buf
		}
	}
	want := "revision_id,sha256,xml_bytes\nA_1,deadbeef,4\n"
	if string(body) != want {
		t.Fatalf("body=%q want=%q", body, want)
	}
}
