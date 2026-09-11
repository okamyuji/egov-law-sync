package application

import (
	"cmp"
	"log"
	"slices"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

// progressEvery 進捗を出す間隔。無音が続くと外からループと区別できない
const progressEvery = 500

// progress n件目ごとに1行出す
func progress(kind string, done, total int) {
	if done%progressEvery == 0 || done == total {
		log.Printf("%s %d/%d", kind, done, total)
	}
}

// renderTexts 取得できたrevisionのXMLをMarkdownとJSONLにする。失敗は数えるだけで、取得結果と終了コードには影響させない
func renderTexts(d Deps, targets []law.Law, fetched []law.XMLRecord, xmlDir, textDir string, rec *RunRecord) []law.TextRecord {
	if textDir == "" || len(fetched) == 0 {
		return nil
	}
	ok := make(map[law.RevisionID]bool, len(fetched))
	for _, f := range fetched {
		ok[f.RevisionID] = true
	}
	rendered := make([]law.TextRecord, 0, len(fetched))
	done := 0
	for _, t := range targets {
		if !ok[t.RevisionID] {
			continue
		}
		done++
		tr, err := d.Text.Render(xmlDir, textDir, t)
		if err != nil {
			rec.Counts["text_failed"]++
		} else {
			rendered = append(rendered, tr)
		}
		progress("text", done, len(fetched))
	}
	rec.Counts["text_ok"] += len(rendered)
	if rec.Counts["text_failed"] > d.Threshold.MaxFetchFailures {
		addWarning(rec, "text_failures")
	}
	return rendered
}

// bundleText 今回変換した分をlaws-text.zipにまとめる。zipは正本ではないので失敗は警告にとどめる
func bundleText(d Deps, textDir, zipPath string, rendered []law.TextRecord, rec *RunRecord) {
	if zipPath == "" || len(rendered) == 0 {
		return
	}
	index := slices.Clone(rendered)
	slices.SortFunc(index, func(a, b law.TextRecord) int { return cmp.Compare(a.RevisionID, b.RevisionID) })
	if err := d.Bundler.BundleText(textDir, zipPath, index); err != nil {
		addWarning(rec, "text_bundle_failed")
	}
}
