package lawxml

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEachReadsFilesInNameOrder(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "B.jsonl"), `{"law_id":"B","revision_id":"B_1","text":"b"}`+"\n")
	writeFile(t, filepath.Join(dir, "A.jsonl"), `{"law_id":"A","revision_id":"A_1","text":"a1"}`+"\n"+`{"law_id":"A","revision_id":"A_1","text":"a2"}`+"\n")
	writeFile(t, filepath.Join(dir, "ignore.md"), "# not jsonl\n")
	var got []string
	err := JSONL{}.Each(dir, func(c law.Chunk) error { got = append(got, c.Text); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != "a1" || got[1] != "a2" || got[2] != "b" {
		t.Fatalf("got %v", got)
	}
}

func TestEachStopsOnBrokenLine(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "A.jsonl"), `{"law_id":"A","revision_id":"A_1","text":"a1"}`+"\n"+"{broken\n"+`{"law_id":"A","revision_id":"A_1","text":"a3"}`+"\n")
	n := 0
	err := JSONL{}.Each(dir, func(law.Chunk) error { n++; return nil })
	if err == nil || n != 1 {
		t.Fatalf("err=%v n=%d", err, n)
	}
	if !strings.Contains(err.Error(), "A.jsonl:2") {
		t.Fatalf("error must name the file and line: %v", err)
	}
}

func TestEachPropagatesCallbackError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "A.jsonl"), `{"law_id":"A","revision_id":"A_1","text":"a1"}`+"\n")
	want := errors.New("stop")
	if err := (JSONL{}).Each(dir, func(law.Chunk) error { return want }); !errors.Is(err, want) {
		t.Fatalf("err=%v", err)
	}
}

func TestEachOnEmptyDirIsNoOp(t *testing.T) {
	n := 0
	if err := (JSONL{}).Each(t.TempDir(), func(law.Chunk) error { n++; return nil }); err != nil || n != 0 {
		t.Fatalf("err=%v n=%d", err, n)
	}
}

func TestEachUnreadableFileIsError(t *testing.T) {
	dir := t.TempDir()
	// ディレクトリ名を*.jsonlにしてos.Openの後の読み取りを失敗させる
	if err := os.Mkdir(filepath.Join(dir, "A.jsonl"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := (JSONL{}).Each(dir, func(law.Chunk) error { return nil }); err == nil {
		t.Fatal("expected error")
	}
}

func TestEachRoundTripsRenderedChunks(t *testing.T) {
	xmlDir, textDir := t.TempDir(), t.TempDir()
	copySample(t, xmlDir)
	if _, err := (Renderer{}).Render(xmlDir, textDir, sampleMeta); err != nil {
		t.Fatal(err)
	}
	var got []law.Chunk
	if err := (JSONL{}).Each(textDir, func(c law.Chunk) error { got = append(got, c); return nil }); err != nil {
		t.Fatal(err)
	}
	_, want := mustConvert(t, "testdata/sample.xml", sampleMeta)
	if len(got) != len(want) {
		t.Fatalf("got %d chunks, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chunk %d differs:\ngot  %+v\nwant %+v", i, got[i], want[i])
		}
	}
}

func TestEachRejectsRegularFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "A.jsonl"), "{}\n")
	if err := (JSONL{}).Each(filepath.Join(dir, "A.jsonl"), func(law.Chunk) error { return nil }); err == nil {
		t.Fatal("a regular file must be an error")
	}
}

func TestEachRejectsMissingDir(t *testing.T) {
	err := JSONL{}.Each(filepath.Join(t.TempDir(), "missing"), func(law.Chunk) error { return nil })
	if err == nil {
		t.Fatal("missing dir must be an error")
	}
}
