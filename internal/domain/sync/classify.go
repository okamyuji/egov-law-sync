package sync

import (
	"sort"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

type ChangeKind string

const (
	Added        ChangeKind = "added"        // 新規
	Switched     ChangeKind = "switched"     // 切替
	Reregistered ChangeKind = "reregistered" // 再登録
	Removed      ChangeKind = "removed"      // 消失
)

type Change struct {
	LawID       law.LawID
	Kind        ChangeKind
	OldRevision law.RevisionID
	NewRevision law.RevisionID
}

// Classify 前回と今回の一覧を比較して変更を4種類に分ける。結果はLawID順
func Classify(prev, cur []law.Law) []Change {
	prevByID := make(map[law.LawID]law.Law, len(prev))
	for _, p := range prev {
		prevByID[p.ID] = p
	}
	curIDs := make(map[law.LawID]bool, len(cur))
	changes := make([]Change, 0)

	for _, c := range cur {
		curIDs[c.ID] = true
		p, existed := prevByID[c.ID]
		if !existed {
			changes = append(changes, Change{LawID: c.ID, Kind: Added, NewRevision: c.RevisionID})
		} else if p.RevisionID != c.RevisionID {
			changes = append(changes, Change{LawID: c.ID, Kind: Switched, OldRevision: p.RevisionID, NewRevision: c.RevisionID})
		} else if p.Updated != c.Updated {
			changes = append(changes, Change{LawID: c.ID, Kind: Reregistered, OldRevision: p.RevisionID, NewRevision: c.RevisionID})
		}
	}
	for _, p := range prev {
		if !curIDs[p.ID] {
			changes = append(changes, Change{LawID: p.ID, Kind: Removed, OldRevision: p.RevisionID})
		}
	}

	sort.SliceStable(changes, func(i, j int) bool { return changes[i].LawID < changes[j].LawID })
	return changes
}

// CountUnexpected 想定内以外の件数。切替のうちknownに新revisionがあるものだけを除く
func CountUnexpected(changes []Change, known map[law.RevisionID]bool) int {
	n := 0
	for _, c := range changes {
		if c.Kind == Switched && known[c.NewRevision] {
			continue
		}
		n++
	}
	return n
}
