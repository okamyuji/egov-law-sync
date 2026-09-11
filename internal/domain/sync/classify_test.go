package sync

import (
	"testing"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

func l(id, rev, updated string) law.Law {
	return law.Law{ID: law.LawID(id), RevisionID: law.RevisionID(rev), Updated: updated}
}

func TestClassifyFourKinds(t *testing.T) {
	prev := []law.Law{l("A", "A_1", "u1"), l("B", "B_1", "u1"), l("C", "C_1", "u1"), l("D", "D_1", "u1")}
	cur := []law.Law{l("A", "A_1", "u1"), l("B", "B_2", "u2"), l("C", "C_1", "u9"), l("E", "E_1", "u1")}
	got := Classify(prev, cur)
	want := []Change{
		{LawID: "B", Kind: Switched, OldRevision: "B_1", NewRevision: "B_2"},
		{LawID: "C", Kind: Reregistered, OldRevision: "C_1", NewRevision: "C_1"},
		{LawID: "D", Kind: Removed, OldRevision: "D_1"},
		{LawID: "E", Kind: Added, NewRevision: "E_1"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("index %d: got %+v want %+v", i, got[i], want[i])
		}
	}
}

func TestClassifyNoChange(t *testing.T) {
	prev := []law.Law{l("A", "A_1", "u1")}
	if got := Classify(prev, prev); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestClassifySortIsStableForEqualLawID(t *testing.T) {
	// 同一law_idの重複入力でも並び替えで元の順序が崩れないことを確認する
	cur := []law.Law{l("X", "X_1", "u1"), l("X", "X_2", "u1")}
	got := Classify(nil, cur)
	if len(got) != 2 || got[0].NewRevision != "X_1" || got[1].NewRevision != "X_2" {
		t.Fatalf("got %+v", got)
	}
}

func TestClassifyEmptyInputs(t *testing.T) {
	if got := Classify(nil, nil); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestCountUnexpectedExcludesKnownSwitch(t *testing.T) {
	changes := []Change{
		{LawID: "A", Kind: Switched, NewRevision: "A_2"},
		{LawID: "B", Kind: Switched, NewRevision: "B_2"},
		{LawID: "C", Kind: Reregistered},
		{LawID: "D", Kind: Added, NewRevision: "D_1"},
		{LawID: "E", Kind: Removed},
	}
	known := map[law.RevisionID]bool{"A_2": true, "D_1": true}
	// 想定内はAの切替だけ。Dは新規なので登録済みでも数える
	if got := CountUnexpected(changes, known); got != 4 {
		t.Fatalf("got %d", got)
	}
}

func TestCountUnexpectedEmpty(t *testing.T) {
	if got := CountUnexpected(nil, nil); got != 0 {
		t.Fatalf("got %d", got)
	}
}

func TestCountUnexpectedIgnoresNonSwitchedEvenIfKnown(t *testing.T) {
	// 対象がSwitched以外ならknownの中身は見ない
	changes := []Change{{LawID: "D", Kind: Added, NewRevision: "D_1"}}
	known := map[law.RevisionID]bool{"D_1": true}
	if got := CountUnexpected(changes, known); got != 1 {
		t.Fatalf("got %d", got)
	}
}
