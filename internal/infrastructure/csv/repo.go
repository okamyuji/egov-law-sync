package csv

import (
	"cmp"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/okamyuji/egov-law-sync/internal/application"
	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

var _ application.ManifestRepository = (*Repo)(nil)

const (
	lawsFile      = "laws.csv"
	revisionsFile = "revisions.csv"
	xmlIndexFile  = "xml_index.csv"
	runsDir       = "runs"
	nameLayout    = "20060102T150405Z"
)

var (
	lawsHeader      = []string{"law_id", "law_type", "law_title", "revision_id", "updated", "enforcement_date", "repeal_status"}
	revisionsHeader = []string{"revision_id", "law_id", "law_title", "enforcement_date", "promulgate_date", "amendment_law_num", "status", "updated", "first_seen"}
	xmlIndexHeader  = []string{"revision_id", "updated", "sha256", "xml_bytes", "release_tag"}
)

// Repo application.ManifestRepositoryの実装。dir配下にlaws.csv、revisions.csv、xml_index.csv、runs/を持つ
type Repo struct{ dir string }

// New dirを正本の置き場所とするRepoを作る
func New(dir string) *Repo {
	return &Repo{dir: dir}
}

// writeCSV headerと行をpathへ書く。<path>.tmpへ書いてからrenameで置き換える
func writeCSV(path string, header []string, rows [][]string) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	if err := w.Write(header); err != nil {
		f.Close()
		return err
	}
	if err := w.WriteAll(rows); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// readCSV pathを読みヘッダを除いた行を返す。無ければ空を返す
func readCSV(path string) ([][]string, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) <= 1 {
		return nil, nil
	}
	return rows[1:], nil
}

// LoadLaws laws.csvをlaw_idでソート済みで読む
func (r *Repo) LoadLaws() ([]law.Law, error) {
	rows, err := readCSV(filepath.Join(r.dir, lawsFile))
	if err != nil {
		return nil, err
	}
	laws := make([]law.Law, 0, len(rows))
	for _, row := range rows {
		if len(row) != len(lawsHeader) {
			return nil, errors.New("csv: laws.csv row has wrong column count")
		}
		if !law.ValidID(row[0]) || !law.ValidID(row[3]) {
			return nil, fmt.Errorf("csv: laws.csv has an unusable id: law_id=%q revision_id=%q", row[0], row[3])
		}
		laws = append(laws, law.Law{
			ID:              law.LawID(row[0]),
			Type:            row[1],
			Title:           row[2],
			RevisionID:      law.RevisionID(row[3]),
			Updated:         row[4],
			EnforcementDate: row[5],
			RepealStatus:    row[6],
		})
	}
	slices.SortFunc(laws, func(a, b law.Law) int { return cmp.Compare(a.ID, b.ID) })
	return laws, nil
}

// SaveLaws laws.csvへlaw_idでソートして書く
func (r *Repo) SaveLaws(laws []law.Law) error {
	sorted := slices.Clone(laws)
	slices.SortFunc(sorted, func(a, b law.Law) int { return cmp.Compare(a.ID, b.ID) })
	rows := make([][]string, 0, len(sorted))
	for _, l := range sorted {
		rows = append(rows, []string{
			string(l.ID), l.Type, l.Title, string(l.RevisionID), l.Updated, l.EnforcementDate, l.RepealStatus,
		})
	}
	return writeCSV(filepath.Join(r.dir, lawsFile), lawsHeader, rows)
}

// LoadRevisions revisions.csvをrevision_idをキーにしたmapで読む
func (r *Repo) LoadRevisions() (map[law.RevisionID]law.Revision, error) {
	rows, err := readCSV(filepath.Join(r.dir, revisionsFile))
	if err != nil {
		return nil, err
	}
	out := make(map[law.RevisionID]law.Revision, len(rows))
	for _, row := range rows {
		if len(row) != len(revisionsHeader) {
			return nil, errors.New("csv: revisions.csv row has wrong column count")
		}
		if !law.ValidID(row[0]) || !law.ValidID(row[1]) {
			return nil, fmt.Errorf("csv: revisions.csv has an unusable id: revision_id=%q law_id=%q", row[0], row[1])
		}
		rev := law.Revision{
			ID:              law.RevisionID(row[0]),
			LawID:           law.LawID(row[1]),
			Title:           row[2],
			EnforcementDate: row[3],
			PromulgateDate:  row[4],
			AmendmentLawNum: row[5],
			Status:          row[6],
			Updated:         row[7],
			FirstSeen:       row[8],
		}
		out[rev.ID] = rev
	}
	return out, nil
}

// SaveRevisions revisions.csvへrevision_idでソートして書く
func (r *Repo) SaveRevisions(revisions map[law.RevisionID]law.Revision) error {
	ids := slices.Sorted(maps.Keys(revisions))
	rows := make([][]string, 0, len(ids))
	for _, id := range ids {
		rev := revisions[id]
		rows = append(rows, []string{
			string(rev.ID), string(rev.LawID), rev.Title, rev.EnforcementDate, rev.PromulgateDate,
			rev.AmendmentLawNum, rev.Status, rev.Updated, rev.FirstSeen,
		})
	}
	return writeCSV(filepath.Join(r.dir, revisionsFile), revisionsHeader, rows)
}

// LoadXMLIndex xml_index.csvをrevision_idをキーにしたmapで読む
func (r *Repo) LoadXMLIndex() (map[law.RevisionID]law.XMLRecord, error) {
	rows, err := readCSV(filepath.Join(r.dir, xmlIndexFile))
	if err != nil {
		return nil, err
	}
	out := make(map[law.RevisionID]law.XMLRecord, len(rows))
	for _, row := range rows {
		if len(row) != len(xmlIndexHeader) {
			return nil, errors.New("csv: xml_index.csv row has wrong column count")
		}
		if !law.ValidID(row[0]) {
			return nil, fmt.Errorf("csv: xml_index.csv has an unusable revision_id: %q", row[0])
		}
		n, err := strconv.ParseInt(row[3], 10, 64)
		if err != nil {
			return nil, err
		}
		rec := law.XMLRecord{
			RevisionID: law.RevisionID(row[0]),
			Updated:    row[1],
			SHA256:     row[2],
			Bytes:      n,
			ReleaseTag: row[4],
		}
		out[rec.RevisionID] = rec
	}
	return out, nil
}

// SaveXMLIndex xml_index.csvへrevision_idでソートして書く
func (r *Repo) SaveXMLIndex(index map[law.RevisionID]law.XMLRecord) error {
	ids := slices.Sorted(maps.Keys(index))
	rows := make([][]string, 0, len(ids))
	for _, id := range ids {
		rec := index[id]
		rows = append(rows, []string{
			string(rec.RevisionID), rec.Updated, rec.SHA256, strconv.FormatInt(rec.Bytes, 10), rec.ReleaseTag,
		})
	}
	return writeCSV(filepath.Join(r.dir, xmlIndexFile), xmlIndexHeader, rows)
}

// runFileName StartedAtからruns/<kind>/<name>.jsonのファイル名部分を作る
func runFileName(r application.RunRecord) (string, error) {
	t, err := time.Parse(time.RFC3339, r.StartedAt)
	if err != nil {
		return "", err
	}
	return t.UTC().Format(nameLayout) + ".json", nil
}

// SaveRun RunRecordをruns/<kind>/<name>.jsonへ書く
func (r *Repo) SaveRun(rec application.RunRecord) error {
	name, err := runFileName(rec)
	if err != nil {
		return err
	}
	dir := filepath.Join(r.dir, runsDir, rec.Kind)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// listRunFiles runs/<kind>配下のファイル名をソート済みで返す
func (r *Repo) listRunFiles(kind string) ([]string, error) {
	dir := filepath.Join(r.dir, runsDir, kind)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	return names, nil
}

// loadRun runs/<kind>/<name>を読みRunRecordにする
func (r *Repo) loadRun(kind, name string) (application.RunRecord, error) {
	body, err := os.ReadFile(filepath.Join(r.dir, runsDir, kind, name))
	if err != nil {
		return application.RunRecord{}, err
	}
	var rec application.RunRecord
	if err := json.Unmarshal(body, &rec); err != nil {
		return application.RunRecord{}, err
	}
	return rec, nil
}

// newestRun kind配下の最新ファイル(ファイル名の最大値)を返す。無ければok=false
func (r *Repo) newestRun(kind string) (application.RunRecord, bool, error) {
	names, err := r.listRunFiles(kind)
	if err != nil {
		return application.RunRecord{}, false, err
	}
	if len(names) == 0 {
		return application.RunRecord{}, false, nil
	}
	rec, err := r.loadRun(kind, names[len(names)-1])
	if err != nil {
		return application.RunRecord{}, false, err
	}
	return rec, true, nil
}

// LastDaily runs/daily配下でファイル名が最大のものを返す
func (r *Repo) LastDaily() (application.RunRecord, bool, error) {
	return r.newestRun("daily")
}

// LastBootstrap runs/bootstrap配下でファイル名が最大のものを返す
func (r *Repo) LastBootstrap() (application.RunRecord, bool, error) {
	return r.newestRun("bootstrap")
}

// LastAppliedDaily applied==trueの中でtoが最大のものを返す。同点はファイル名が新しい方
func (r *Repo) LastAppliedDaily() (application.RunRecord, bool, error) {
	names, err := r.listRunFiles("daily")
	if err != nil {
		return application.RunRecord{}, false, err
	}
	var best application.RunRecord
	found := false
	for _, name := range names {
		rec, err := r.loadRun("daily", name)
		if err != nil {
			return application.RunRecord{}, false, err
		}
		if !rec.Applied {
			continue
		}
		if !found || rec.To >= best.To {
			best = rec
			found = true
		}
	}
	return best, found, nil
}
