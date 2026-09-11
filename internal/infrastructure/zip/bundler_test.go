package zip

import (
	archivezip "archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	if strings.Contains(readEntry(t, r, "index.csv"), "MISSING") {
		t.Fatal("index.csv must not list a file that is absent from the zip")
	}
}

// readEntry zip内の1エントリを文字列で読む
func readEntry(t *testing.T, r *archivezip.ReadCloser, name string) string {
	t.Helper()
	for _, f := range r.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		body, err := io.ReadAll(rc)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	t.Fatalf("%s not found", name)
	return ""
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

func TestBundleRemovesZipOnError(t *testing.T) {
	dir := t.TempDir()
	// <rev>.xmlをディレクトリにするとos.ReadFileはEISDIRを返す。IsNotExistで飛ばされる経路と区別できる
	if err := os.Mkdir(filepath.Join(dir, "A_1.xml"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "laws-xml.zip")
	if err := (Bundler{}).Bundle(dir, out, []law.XMLRecord{{RevisionID: "A_1"}}); err == nil {
		t.Fatal("want an error when an xml file cannot be read")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("truncated zip must be removed, stat err=%v", err)
	}
}

func TestINV6BundleTextIndexMatchesEntries(t *testing.T) {
	dir := t.TempDir()
	must := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must("A_1.md", "# a\n")
	must("A_1.jsonl", "{}\n{}\n")
	must("B_1.md", "# b\n") // jsonlが無い: index.csvにもzipにも載せない
	out := filepath.Join(t.TempDir(), "laws-text.zip")
	index := []law.TextRecord{
		{RevisionID: "A_1", MDBytes: 4, JSONLBytes: 6, Chunks: 2},
		{RevisionID: "B_1", MDBytes: 4},
		{RevisionID: "MISSING", MDBytes: 1, JSONLBytes: 1, Chunks: 1},
	}
	if err := (Bundler{}).BundleText(dir, out, index); err != nil {
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
	if !names["A_1.md"] || !names["A_1.jsonl"] || names["B_1.md"] || names["MISSING.md"] {
		t.Fatalf("entries = %v", names)
	}
	want := "revision_id,md_bytes,jsonl_bytes,chunks\nA_1,4,6,2\n"
	if got := readEntry(t, r, "index.csv"); got != want {
		t.Fatalf("index.csv = %q", got)
	}
	if got := readEntry(t, r, "A_1.jsonl"); strings.Count(got, "\n") != 2 {
		t.Fatalf("jsonl lines = %d", strings.Count(got, "\n"))
	}
}

func TestBundleTextRemovesZipOnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "A_1.md"), 0o755); err != nil { // ディレクトリを読ませてEISDIRにする
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "A_1.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "laws-text.zip")
	if err := (Bundler{}).BundleText(dir, out, []law.TextRecord{{RevisionID: "A_1"}}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("zip must be removed on error")
	}
}
