package e2e

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/okamyuji/egov-law-sync/internal/application"
	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

// mkLaw テスト用のlaw.Lawを組み立てる
func mkLaw(id, rev, updated, enforcement string) law.Law {
	return law.Law{
		ID: law.LawID(id), Type: "Act", Title: "title-" + id,
		RevisionID: law.RevisionID(rev), Updated: updated,
		EnforcementDate: enforcement, RepealStatus: "None",
	}
}

// runCLI EGOV_BINをexec.Commandで実行し、終了コードとstdout、stderrを返す
func runCLI(t *testing.T, env map[string]string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	bin := os.Getenv("EGOV_BIN")
	if bin == "" {
		t.Fatal("EGOV_BIN is not set; run via make e2e")
	}
	cmd := exec.CommandContext(t.Context(), bin, args...)
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), outBuf.String(), errBuf.String()
		}
		t.Fatalf("run %v: %v", args, err)
	}
	return 0, outBuf.String(), errBuf.String()
}

// baseEnv 3つのBASEを偽サーバに向け、正本の場所をmanifestDirにする
func baseEnv(srvURL, manifestDir string) map[string]string {
	return map[string]string{
		"EGOV_V2_BASE": srvURL, "EGOV_V1_BASE": srvURL, "EGOV_BULK_BASE": srvURL,
		"EGOV_MANIFEST_DIR": manifestDir,
	}
}

// readRows CSVのヘッダを除いた行を返す
func readRows(t *testing.T, path string) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if len(rows) == 0 {
		return nil
	}
	return rows[1:]
}

// runFileNames runs/<kind>配下のファイル名をソート済みで返す。無ければ空
func runFileNames(t *testing.T, manifestDir, kind string) []string {
	t.Helper()
	dir := filepath.Join(manifestDir, "runs", kind)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	slices.Sort(names)
	return names
}

// lastRun runs/<kind>配下で最新のRunRecordを返す
func lastRun(t *testing.T, manifestDir, kind string) application.RunRecord {
	t.Helper()
	names := runFileNames(t, manifestDir, kind)
	if len(names) == 0 {
		t.Fatalf("no run files under runs/%s", kind)
	}
	body, err := os.ReadFile(filepath.Join(manifestDir, "runs", kind, names[len(names)-1]))
	if err != nil {
		t.Fatal(err)
	}
	var rec application.RunRecord
	if err := json.Unmarshal(body, &rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

// TestBootstrapFlow 導線1。3法令（1つは未施行改正あり）からlaws.csv、revisions.csv、xml_index.csvを作る
func TestBootstrapFlow(t *testing.T) {
	srv := newFakeServer()
	srv.setLaw(mkLaw("A", "A_1", "u1", "2020-01-01"))
	srv.setLaw(mkLaw("B", "B_1", "u1", "2020-01-01"))
	srv.setLaw(mkLaw("C", "C_1", "u1", "2020-01-01"))
	srv.setFuture(mkLaw("C", "C_2", "u2", "2030-01-01"))
	srv.setRevisions("C", []wireRevisionInfo{
		{LawRevisionID: "C_1", LawTitle: "title-C", Updated: "u1", AmendmentEnforcementDate: "2020-01-01", CurrentRevisionStatus: "CurrentEnforced"},
		{LawRevisionID: "C_2", LawTitle: "title-C", Updated: "u2", AmendmentEnforcementDate: "2030-01-01", CurrentRevisionStatus: "UnEnforced"},
	})
	srv.setXML("A_1", lawXML("title-A", "本文A"))
	srv.setXML("B_1", lawXML("title-B", "本文B"))
	srv.setXML("C_1", lawXML("title-C", "本文C"))
	ts := srv.start()
	defer ts.Close()

	manifestDir := t.TempDir()
	xmlDir := filepath.Join(t.TempDir(), "xml")
	zipPath := filepath.Join(t.TempDir(), "laws.zip")
	work := t.TempDir()
	textZip := filepath.Join(work, "laws-text.zip")

	code, _, stderr := runCLI(t, baseEnv(ts.URL, manifestDir),
		"bootstrap", "--release-tag", "t1", "--xml-dir", xmlDir, "--zip-path", zipPath,
		"--text-dir", filepath.Join(work, "text"), "--text-zip-path", textZip)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}

	if laws := readRows(t, filepath.Join(manifestDir, "laws.csv")); len(laws) != 3 {
		t.Fatalf("laws.csv rows=%v", laws)
	}
	revs := readRows(t, filepath.Join(manifestDir, "revisions.csv"))
	foundUnenforced := false
	for _, r := range revs {
		if r[0] == "C_2" && r[6] == "UnEnforced" {
			foundUnenforced = true
		}
	}
	if !foundUnenforced {
		t.Fatalf("revisions.csv missing C_2 UnEnforced: %v", revs)
	}
	if idx := readRows(t, filepath.Join(manifestDir, "xml_index.csv")); len(idx) != 3 {
		t.Fatalf("xml_index.csv rows=%v", idx)
	}
	if got := len(runFileNames(t, manifestDir, "bootstrap")); got != 1 {
		t.Fatalf("runs/bootstrap files=%d", got)
	}
	entries, err := os.ReadDir(xmlDir)
	if err != nil || len(entries) != 3 {
		t.Fatalf("xmlDir entries=%v err=%v", entries, err)
	}
	assertTextZip(t, textZip)
	rec := lastRun(t, manifestDir, "bootstrap")
	if rec.Counts["text_ok"] != 3 || rec.Counts["text_failed"] != 0 {
		t.Fatalf("text counts = %v", rec.Counts)
	}
}

// assertTextZip INV-5（jsonlの各行が正しいChunk）とINV-6（index.csvのchunks列がjsonl行数と一致し、
// 索引済みのrevisionがすべてmdとjsonlを持つ）をlaws-text.zipに対して検査する
func assertTextZip(t *testing.T, textZip string) {
	t.Helper()
	entries := zipEntries(t, textZip)
	idx := parseCSV(t, entries["index.csv"])
	if len(idx) != 4 { // header + 3
		t.Fatalf("index rows = %d", len(idx))
	}
	for _, row := range idx[1:] {
		assertTextZipEntry(t, entries, row)
	}
}

// assertTextZipEntry index.csvの1行分についてmd、jsonlの整合を検査する
func assertTextZipEntry(t *testing.T, entries map[string][]byte, row []string) {
	t.Helper()
	rev := row[0]
	md, okMD := entries[rev+".md"]
	jl, okJL := entries[rev+".jsonl"]
	if !okMD || !okJL {
		t.Fatalf("INV-6: %s must have md and jsonl", rev)
	}
	if strings.Count(string(jl), "\n") != atoi(t, row[3]) {
		t.Fatalf("INV-6: chunks column mismatch for %s", rev)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(jl)), "\n") {
		var c law.Chunk
		if err := json.Unmarshal([]byte(line), &c); err != nil || c.LawID == "" || c.Text == "" || string(c.RevisionID) != rev {
			t.Fatalf("INV-5: bad line %q err=%v", line, err)
		}
	}
	if !strings.HasPrefix(string(md), "---\nlaw_id: ") {
		t.Fatalf("md front matter missing for %s", rev)
	}
}

// zipEntries pathのzipを読み、エントリ名からバイト列への対応を返す
func zipEntries(t *testing.T, path string) map[string][]byte {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip %s: %v", path, err)
	}
	defer r.Close()
	entries := make(map[string][]byte, len(r.File))
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", f.Name, err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read zip entry %s: %v", f.Name, err)
		}
		entries[f.Name] = body
	}
	return entries
}

// parseCSV バイト列をCSVとして読み、行の集合を返す
func parseCSV(t *testing.T, body []byte) [][]string {
	t.Helper()
	rows, err := csv.NewReader(bytes.NewReader(body)).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	return rows
}

// atoi CSVの数値列をintにする。数値でなければテストを失敗させる
func atoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("atoi %q: %v", s, err)
	}
	return n
}

// TestDailyFlowWithChanges 導線2。切替1件（既知の改正が施行）と再登録1件を検出し、xml_indexを更新する
func TestDailyFlowWithChanges(t *testing.T) {
	srv := newFakeServer()
	srv.setLaw(mkLaw("A", "A_1", "u1", "2020-01-01"))
	srv.setLaw(mkLaw("B", "B_1", "u1", "2020-01-01"))
	srv.setLaw(mkLaw("C", "C_1", "u1", "2020-01-01"))
	srv.setFuture(mkLaw("C", "C_2", "u2", "2030-01-01"))
	srv.setRevisions("C", []wireRevisionInfo{
		{LawRevisionID: "C_1", LawTitle: "title-C", Updated: "u1", AmendmentEnforcementDate: "2020-01-01", CurrentRevisionStatus: "CurrentEnforced"},
		{LawRevisionID: "C_2", LawTitle: "title-C", Updated: "u2", AmendmentEnforcementDate: "2030-01-01", CurrentRevisionStatus: "UnEnforced"},
	})
	srv.setXML("A_1", lawXML("title-A", "本文A"))
	srv.setXML("B_1", lawXML("title-B", "本文B"))
	srv.setXML("C_1", lawXML("title-C", "本文C"))
	srv.setXML("C_2", lawXML("title-C", "本文C2"))
	ts := srv.start()
	defer ts.Close()

	manifestDir := t.TempDir()
	xmlDir := filepath.Join(t.TempDir(), "xml")
	zipPath := filepath.Join(t.TempDir(), "laws.zip")
	env := baseEnv(ts.URL, manifestDir)

	code, _, stderr := runCLI(t, env, "bootstrap", "--release-tag", "t1", "--xml-dir", xmlDir, "--zip-path", zipPath)
	if code != 0 {
		t.Fatalf("bootstrap exit=%d stderr=%s", code, stderr)
	}

	yesterday := law.DateOf(time.Now()).Add(-1)
	from := yesterday.Add(-3)
	srv.setV1(yesterday, []law.LawID{"B"})

	// 法令Cの未施行改正が施行された（切替、既知のrevisionなので想定内）。法令Bは同じrevisionのまま再登録された（想定外）
	srv.laws["C"] = mkLaw("C", "C_2", "u2", "2030-01-01")
	srv.laws["B"] = mkLaw("B", "B_1", "u2", "2020-01-01")
	srv.xmlBody["B_1"] = lawXML("title-B", "本文B2")

	code, _, stderr = runCLI(t, env, "daily", "--from", string(from), "--to", string(yesterday),
		"--release-tag", "t1", "--xml-dir", xmlDir, "--zip-path", zipPath)
	if code != 0 {
		t.Fatalf("daily exit=%d stderr=%s", code, stderr)
	}

	assertSwitchedAndReregistered(t, manifestDir)
	assertXMLIndexUpdated(t, manifestDir)

	rec := lastRun(t, manifestDir, "daily")
	if rec.Counts["switched"] != 1 || rec.Counts["reregistered"] != 1 || rec.Counts["unexpected"] != 1 {
		t.Fatalf("counts=%v", rec.Counts)
	}
}

// assertSwitchedAndReregistered laws.csvが3行のまま、Cは切替、Bは再登録された状態になっていることを確かめる
func assertSwitchedAndReregistered(t *testing.T, manifestDir string) {
	t.Helper()
	laws := readRows(t, filepath.Join(manifestDir, "laws.csv"))
	if len(laws) != 3 {
		t.Fatalf("laws.csv rows=%v", laws)
	}
	for _, l := range laws {
		if l[0] == "C" && l[3] != "C_2" {
			t.Fatalf("law C not switched: %v", l)
		}
		if l[0] == "B" && l[4] != "u2" {
			t.Fatalf("law B not reregistered: %v", l)
		}
	}
}

// assertXMLIndexUpdated xml_index.csvにB_1とC_2の新しいshaが記録されていることを確かめる
func assertXMLIndexUpdated(t *testing.T, manifestDir string) {
	t.Helper()
	idx := readRows(t, filepath.Join(manifestDir, "xml_index.csv"))
	newSHA := map[string]string{}
	for _, r := range idx {
		newSHA[r[0]] = r[2]
	}
	if newSHA["B_1"] == "" || newSHA["C_2"] == "" {
		t.Fatalf("xml_index.csv missing updated rows: %v", idx)
	}
}

// TestDailyFlowNoChanges 導線3。何も変えずに再実行すると、範囲が空になりCSVは変わらない
func TestDailyFlowNoChanges(t *testing.T) {
	srv := newFakeServer()
	srv.setLaw(mkLaw("A", "A_1", "u1", "2020-01-01"))
	srv.setLaw(mkLaw("B", "B_1", "u1", "2020-01-01"))
	srv.setLaw(mkLaw("C", "C_1", "u1", "2020-01-01"))
	srv.setFuture(mkLaw("C", "C_1", "u1", "2020-01-01"))
	srv.setXML("A_1", lawXML("title-A", "本文A"))
	srv.setXML("B_1", lawXML("title-B", "本文B"))
	srv.setXML("C_1", lawXML("title-C", "本文C"))
	ts := srv.start()
	defer ts.Close()

	manifestDir := t.TempDir()
	xmlDir := filepath.Join(t.TempDir(), "xml")
	zipPath := filepath.Join(t.TempDir(), "laws.zip")
	env := baseEnv(ts.URL, manifestDir)

	code, _, stderr := runCLI(t, env, "bootstrap", "--release-tag", "t1", "--xml-dir", xmlDir, "--zip-path", zipPath)
	if code != 0 {
		t.Fatalf("bootstrap exit=%d stderr=%s", code, stderr)
	}

	// 初回のbootstrapと同じ日に実行するdailyは、前回適用済みが無いのでbootstrapの実行日が起点になり、
	// 既定のtoは前日なので範囲は必ず空になる。空の実行はFrom=To=起点日で1回分のrunsを残す
	code, _, stderr = runCLI(t, env, "daily", "--release-tag", "t1", "--xml-dir", xmlDir, "--zip-path", zipPath)
	if code != 0 {
		t.Fatalf("first daily exit=%d stderr=%s", code, stderr)
	}
	lawsBefore := readRows(t, filepath.Join(manifestDir, "laws.csv"))
	idxBefore := readRows(t, filepath.Join(manifestDir, "xml_index.csv"))
	firstRec := lastRun(t, manifestDir, "daily")

	// runs/のファイル名は秒精度のUTC時刻。同じ秒に2回実行するとファイルを上書きしてしまうので1秒あける
	time.Sleep(1100 * time.Millisecond)
	code, _, stderr = runCLI(t, env, "daily", "--release-tag", "t1", "--xml-dir", xmlDir, "--zip-path", zipPath)
	if code != 0 {
		t.Fatalf("second daily exit=%d stderr=%s", code, stderr)
	}
	lawsAfter := readRows(t, filepath.Join(manifestDir, "laws.csv"))
	idxAfter := readRows(t, filepath.Join(manifestDir, "xml_index.csv"))
	if !rowsEqual(lawsBefore, lawsAfter) {
		t.Fatalf("laws.csv changed: before=%v after=%v", lawsBefore, lawsAfter)
	}
	if !rowsEqual(idxBefore, idxAfter) {
		t.Fatalf("xml_index.csv changed: before=%v after=%v", idxBefore, idxAfter)
	}

	rec := lastRun(t, manifestDir, "daily")
	if !rec.Applied || rec.From != firstRec.To || rec.To != firstRec.To {
		t.Fatalf("rec=%+v firstRec=%+v", rec, firstRec)
	}
	if got := len(runFileNames(t, manifestDir, "daily")); got != 2 {
		t.Fatalf("runs/daily files=%d", got)
	}
}

// TestWeeklyFlow 導線4。sec1 zipの1件だけがv2と違う内容でも、law_fileの再取得で同じsha256に戻ればzip_staleとして数える
func TestWeeklyFlow(t *testing.T) {
	srv := newFakeServer()
	srv.setLaw(mkLaw("A", "A_1", "u1", "2020-01-01"))
	srv.setLaw(mkLaw("B", "B_1", "u1", "2020-01-01"))
	srv.setLaw(mkLaw("C", "C_1", "u1", "2020-01-01"))
	srv.setFuture(mkLaw("C", "C_1", "u1", "2020-01-01"))
	srv.setXML("A_1", lawXML("title-A", "本文A"))
	srv.setXML("B_1", lawXML("title-B", "本文B"))
	srv.setXML("C_1", lawXML("title-C", "本文C"))
	ts := srv.start()
	defer ts.Close()

	manifestDir := t.TempDir()
	xmlDir := filepath.Join(t.TempDir(), "xml")
	zipPath := filepath.Join(t.TempDir(), "laws.zip")
	env := baseEnv(ts.URL, manifestDir)

	code, _, stderr := runCLI(t, env, "bootstrap", "--release-tag", "t1", "--xml-dir", xmlDir, "--zip-path", zipPath)
	if code != 0 {
		t.Fatalf("bootstrap exit=%d stderr=%s", code, stderr)
	}
	idxBefore := readRows(t, filepath.Join(manifestDir, "xml_index.csv"))

	srv.setSec1Override("A_1", lawXML("title-A", "陳腐化した本文A"))

	code, _, stderr = runCLI(t, env, "weekly", "--release-tag", "t1", "--xml-dir", xmlDir, "--zip-path", zipPath)
	if code != 0 {
		t.Fatalf("weekly exit=%d stderr=%s", code, stderr)
	}

	idxAfter := readRows(t, filepath.Join(manifestDir, "xml_index.csv"))
	if !rowsEqual(idxBefore, idxAfter) {
		t.Fatalf("xml_index.csv changed: before=%v after=%v", idxBefore, idxAfter)
	}
	rec := lastRun(t, manifestDir, "weekly")
	if rec.Counts["zip_stale"] != 1 {
		t.Fatalf("counts=%v", rec.Counts)
	}
}

// TestDailyFlowThresholdExceeded 導線5。201法令のupdatedが変わると異常判定で終了コード3、CSVは書かない
func TestDailyFlowThresholdExceeded(t *testing.T) {
	const n = 201
	srv := newFakeServer()
	for i := range n {
		id := lawID(i)
		srv.setLaw(mkLaw(id, id+"_1", "u1", "2020-01-01"))
	}
	ts := srv.start()
	defer ts.Close()

	manifestDir := t.TempDir()
	env := baseEnv(ts.URL, manifestDir)

	code, _, stderr := runCLI(t, env, "bootstrap")
	if code != 0 {
		t.Fatalf("bootstrap exit=%d stderr=%s", code, stderr)
	}
	lawsBefore := readRows(t, filepath.Join(manifestDir, "laws.csv"))

	yesterday := law.DateOf(time.Now()).Add(-1)
	from := yesterday.Add(-3)
	for i := range n {
		id := lawID(i)
		srv.laws[law.LawID(id)] = mkLaw(id, id+"_1", "u2", "2020-01-01")
	}

	code, _, stderr = runCLI(t, env, "daily", "--from", string(from), "--to", string(yesterday))
	if code != 3 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}

	lawsAfter := readRows(t, filepath.Join(manifestDir, "laws.csv"))
	if !rowsEqual(lawsBefore, lawsAfter) {
		t.Fatalf("laws.csv changed: before=%v after=%v", lawsBefore, lawsAfter)
	}
	rec := lastRun(t, manifestDir, "daily")
	if rec.Applied {
		t.Fatal("expected Applied=false")
	}
	if len(rec.Changes) != n {
		t.Fatalf("changes=%d", len(rec.Changes))
	}
}

// TestIngestFlow 導線6。bootstrapが作ったtext-dirをingestサブコマンドで読み、noop sinkの件数出力を確かめる
func TestIngestFlow(t *testing.T) {
	srv := newFakeServer()
	srv.setLaw(mkLaw("A", "A_1", "2026-09-01T00:00:00+09:00", "2026-09-01"))
	srv.setXML("A_1", lawXML("法A", "本文A"))
	ts := srv.start()
	defer ts.Close()
	manifestDir, work := t.TempDir(), t.TempDir()
	env := baseEnv(ts.URL, manifestDir)
	code, _, _ := runCLI(t, env, "bootstrap", "--release-tag", "v0.0.1",
		"--xml-dir", filepath.Join(work, "xml"), "--zip-path", filepath.Join(work, "x.zip"),
		"--text-dir", filepath.Join(work, "text"), "--text-zip-path", filepath.Join(work, "t.zip"))
	if code != 0 {
		t.Fatalf("bootstrap code=%d", code)
	}
	code, _, stderr := runCLI(t, env, "ingest", filepath.Join(work, "text"))
	if code != 0 || !strings.Contains(stderr, "noop sink: 1 chunks (total 1)") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	code, _, _ = runCLI(t, env, "ingest")
	if code != 2 {
		t.Fatalf("missing arg must be usage error, code=%d", code)
	}
}

// lawID iから3桁ゼロ埋めの法令IDを作る
func lawID(i int) string {
	return fmt.Sprintf("L%03d", i)
}

// rowsEqual 行の集合として比較する。ソートで書き込み順の違いを吸収する
func rowsEqual(a, b [][]string) bool {
	if len(a) != len(b) {
		return false
	}
	as, bs := stringifyRows(a), stringifyRows(b)
	slices.Sort(as)
	slices.Sort(bs)
	for i := range as {
		if as[i] != bs[i] {
			return false
		}
	}
	return true
}

func stringifyRows(rows [][]string) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = filepath.Join(r...)
	}
	return out
}
