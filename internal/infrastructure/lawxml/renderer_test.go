package lawxml

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func copySample(t *testing.T, xmlDir string) {
	t.Helper()
	body, err := os.ReadFile("testdata/sample.xml")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(xmlDir, string(sampleMeta.RevisionID)+".xml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestINV10RenderWritesOnlyMdAndJsonl(t *testing.T) {
	xmlDir, textDir := t.TempDir(), t.TempDir()
	copySample(t, xmlDir)
	rec, err := (Renderer{}).Render(xmlDir, textDir, sampleMeta)
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(textDir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := []string{string(sampleMeta.RevisionID) + ".jsonl", string(sampleMeta.RevisionID) + ".md"}
	if !slices.Equal(names, want) {
		t.Fatalf("files = %v", names)
	}
	md, _ := os.ReadFile(filepath.Join(textDir, want[1]))
	golden, _ := os.ReadFile("testdata/sample.md")
	if string(md) != string(golden) {
		t.Fatal("md file differs from golden")
	}
	jsonl, _ := os.ReadFile(filepath.Join(textDir, want[0]))
	goldenJSONL, _ := os.ReadFile("testdata/sample.jsonl")
	if string(jsonl) != string(goldenJSONL) {
		t.Fatal("jsonl file differs from golden")
	}
	if rec.RevisionID != sampleMeta.RevisionID || rec.Chunks != 4 || rec.MDBytes != int64(len(golden)) || rec.JSONLBytes != int64(len(goldenJSONL)) {
		t.Fatalf("record %+v", rec)
	}
}

func TestINV4NoMdWhenJsonlCannotBeWritten(t *testing.T) {
	xmlDir, textDir := t.TempDir(), t.TempDir()
	copySample(t, xmlDir)
	// 最終名をディレクトリで塞ぎ、jsonlのrenameを失敗させる
	if err := os.Mkdir(filepath.Join(textDir, string(sampleMeta.RevisionID)+".jsonl"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := (Renderer{}).Render(xmlDir, textDir, sampleMeta); err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(filepath.Join(textDir, string(sampleMeta.RevisionID)+".md")); !os.IsNotExist(err) {
		t.Fatal("md must not remain when jsonl failed")
	}
	entries, _ := os.ReadDir(textDir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Fatalf("tmp file left: %s", e.Name())
		}
	}
}

func TestINV4NoJsonlWhenMdCannotBeWritten(t *testing.T) {
	xmlDir, textDir := t.TempDir(), t.TempDir()
	copySample(t, xmlDir)
	if err := os.Mkdir(filepath.Join(textDir, string(sampleMeta.RevisionID)+".md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := (Renderer{}).Render(xmlDir, textDir, sampleMeta); err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(filepath.Join(textDir, string(sampleMeta.RevisionID)+".jsonl")); !os.IsNotExist(err) {
		t.Fatal("jsonl must not remain when md failed")
	}
	entries, _ := os.ReadDir(textDir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Fatalf("tmp file left: %s", e.Name())
		}
	}
}

func TestRenderMissingXMLIsError(t *testing.T) {
	if _, err := (Renderer{}).Render(t.TempDir(), t.TempDir(), sampleMeta); err == nil {
		t.Fatal("expected error")
	}
}

func TestRenderBrokenXMLIsError(t *testing.T) {
	xmlDir, textDir := t.TempDir(), t.TempDir()
	path := filepath.Join(xmlDir, string(sampleMeta.RevisionID)+".xml")
	if err := os.WriteFile(path, []byte("<Law><LawBody>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (Renderer{}).Render(xmlDir, textDir, sampleMeta); err == nil {
		t.Fatal("expected error")
	}
	if entries, _ := os.ReadDir(textDir); len(entries) != 0 {
		t.Fatalf("textDir not empty: %v", entries)
	}
}

func TestRenderCreatesTextDir(t *testing.T) {
	xmlDir := t.TempDir()
	textDir := filepath.Join(t.TempDir(), "nested", "text")
	copySample(t, xmlDir)
	if _, err := (Renderer{}).Render(xmlDir, textDir, sampleMeta); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(textDir, string(sampleMeta.RevisionID)+".md")); err != nil {
		t.Fatal(err)
	}
}
