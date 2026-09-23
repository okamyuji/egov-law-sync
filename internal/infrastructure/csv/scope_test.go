package csv

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

func TestScopedManifestRejectsMixing(t *testing.T) {
	dir := t.TempDir()
	repo := New(dir)
	if err := repo.CheckScope("A"); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveLaws([]law.Law{{ID: "A", RevisionID: "A_1"}}); err != nil {
		t.Fatal(err)
	}
	if err := New(dir).CheckScope("A"); err != nil {
		t.Fatal(err)
	}
	if err := New(dir).CheckScope(""); err == nil {
		t.Fatal("scoped manifest accepted as full")
	}
	if err := New(dir).CheckScope("B"); err == nil {
		t.Fatal("changed scope accepted")
	}
	if err := os.Mkdir(filepath.Join(dir, "full"), 0o755); err != nil {
		t.Fatal(err)
	}
	full := New(filepath.Join(dir, "full"))
	if err := full.SaveLaws([]law.Law{{ID: "A", RevisionID: "A_1"}}); err != nil {
		t.Fatal(err)
	}
	if err := New(filepath.Join(dir, "full")).CheckScope("A"); err == nil {
		t.Fatal("full manifest accepted as scoped")
	}
}

func TestScopedManifestRejectsCorruptScope(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "scope.json"), []byte(`{"law_id":"../other"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := New(dir).CheckScope("A"); err == nil {
		t.Fatal("corrupt scope accepted")
	}
}
