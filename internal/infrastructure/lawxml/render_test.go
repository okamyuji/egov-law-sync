package lawxml

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

var sampleMeta = law.Law{ID: "508AC0000000001", Title: "テスト法", RevisionID: "508AC0000000001_20260401_508AC0000000001", EnforcementDate: "2026-04-01"}

var realMeta = law.Law{ID: "322AC0000000049", Title: "労働基準法", RevisionID: "322AC0000000049_20260717_508AC0000000060", EnforcementDate: "2026-07-17"}

func mustConvert(t *testing.T, path string, meta law.Law) ([]byte, []law.Chunk) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	md, chunks, err := Convert(f, meta)
	if err != nil {
		t.Fatal(err)
	}
	return md, chunks
}

func TestINV8MarkdownMatchesGolden(t *testing.T) {
	md, _ := mustConvert(t, "testdata/sample.xml", sampleMeta)
	want, err := os.ReadFile("testdata/sample.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(md, want) {
		t.Fatalf("markdown differs:\n--- got ---\n%s\n--- want ---\n%s", md, want)
	}
	md2, _ := mustConvert(t, "testdata/sample.xml", sampleMeta)
	if !bytes.Equal(md, md2) {
		t.Fatal("not deterministic")
	}
}

func TestINV8ChunksMatchGolden(t *testing.T) {
	_, chunks := mustConvert(t, "testdata/sample.xml", sampleMeta)
	want, err := os.ReadFile("testdata/sample.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, c := range chunks {
		if err := enc.Encode(c); err != nil {
			t.Fatal(err)
		}
	}
	if buf.String() != string(want) {
		t.Fatalf("jsonl differs:\n--- got ---\n%s\n--- want ---\n%s", buf.String(), want)
	}
}

// sentenceScan Sentenceの本文を集める状態。Rtの中だけを読み飛ばす
type sentenceScan struct {
	out   []string
	cur   *strings.Builder
	depth int
	skip  int
}

func (s *sentenceScan) start(name string) {
	switch {
	case name == "Sentence" && s.cur == nil:
		s.cur = &strings.Builder{}
		s.depth = 0
	case s.cur != nil:
		s.depth++
		if name == "Rt" {
			s.skip++
		}
	}
}

func (s *sentenceScan) end(name string) {
	if s.cur == nil {
		return
	}
	if s.depth == 0 {
		if text := s.cur.String(); text != "" {
			s.out = append(s.out, text)
		}
		s.cur = nil
		return
	}
	if name == "Rt" {
		s.skip--
	}
	s.depth--
}

func (s *sentenceScan) chars(b []byte) {
	if s.cur == nil || s.skip > 0 {
		return
	}
	// 変換器と同じ正規化。要素間の改行や字下げは本文ではないが、全角スペースは本文として残す
	if t := strings.Trim(string(b), " \t\r\n"); t != "" {
		s.cur.WriteString(t)
	}
}

// sentenceTexts XML中のSentenceのテキスト（Rtを除く）を文書順で返す
func sentenceTexts(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	dec := xml.NewDecoder(f)
	var s sentenceScan
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return s.out
		}
		if err != nil {
			t.Fatal(err)
		}
		switch v := tok.(type) {
		case xml.StartElement:
			s.start(v.Name.Local)
		case xml.EndElement:
			s.end(v.Name.Local)
		case xml.CharData:
			s.chars(v)
		}
	}
}

func assertAllSentencesInOrder(t *testing.T, xmlPath string, md []byte) {
	t.Helper()
	text := string(md)
	pos := 0
	for i, s := range sentenceTexts(t, xmlPath) {
		idx := strings.Index(text[pos:], s)
		if idx < 0 {
			t.Fatalf("sentence %d not found in order: %q", i, s)
		}
		pos += idx + len(s)
	}
}

func TestINV7AllSentencesAppearInOrderSample(t *testing.T) {
	md, _ := mustConvert(t, "testdata/sample.xml", sampleMeta)
	assertAllSentencesInOrder(t, "testdata/sample.xml", md)
	if strings.Contains(string(md), "ろう") {
		t.Fatal("Rt text must not appear")
	}
}

func TestINV7AllSentencesAppearInOrderRealLaw(t *testing.T) {
	md, chunks := mustConvert(t, "testdata/322AC0000000049.xml", realMeta)
	assertAllSentencesInOrder(t, "testdata/322AC0000000049.xml", md)
	articles := 0
	for _, c := range chunks {
		if c.Article != "" && !strings.HasPrefix(c.Path, "附則") {
			articles++
		}
	}
	// 本則の条数。TOCと附則は含まない
	if articles != 124 {
		t.Fatalf("main provision articles = %d", articles)
	}
	if chunks[0].Article != "第一条" || chunks[0].ArticleTitle != "労働条件の原則" {
		t.Fatalf("first chunk %+v", chunks[0])
	}
	if !strings.Contains(string(md), "哺育等") || strings.Contains(string(md), "哺ほ") {
		t.Fatal("Ruby base text must survive without its Rt")
	}
}

func TestINV5EveryChunkHasRequiredFields(t *testing.T) {
	_, chunks := mustConvert(t, "testdata/322AC0000000049.xml", realMeta)
	for i, c := range chunks {
		if c.LawID == "" || c.RevisionID != realMeta.RevisionID || c.Text == "" {
			t.Fatalf("chunk %d missing fields: %+v", i, c)
		}
	}
}

func TestINV7FallbackKeepsEverySentence(t *testing.T) {
	md, chunks := mustConvert(t, "testdata/fallback.xml", sampleMeta)
	assertAllSentencesInOrder(t, "testdata/fallback.xml", md)
	if !strings.Contains(string(md), "\n### 第一章　経過\n") {
		t.Fatalf("suppl provision chapter title must be a heading:\n%s", md)
	}
	for _, want := range []string{"一覧の一文である。", "備考の一文である。", "左の欄｜右の欄"} {
		if !strings.Contains(chunks[0].Text, want) {
			t.Fatalf("chunk text must carry %q: %q", want, chunks[0].Text)
		}
	}
	for _, want := range []string{"\n## 制定文\n", "内閣は、この政令を制定する。", "\n## （附図）\n", "記章ノ制式｜径三糎", "記章ハ左胸ニ佩用ス", "- 一　径ハ三糎トス"} {
		if !strings.Contains(string(md), want) {
			t.Fatalf("appendix content %q must reach the markdown:\n%s", want, md)
		}
	}
	if !strings.Contains(string(md), "改正の一文である。") {
		t.Fatal("provision level fallback must reach the markdown")
	}
	for _, c := range chunks {
		if strings.Contains(c.Text, "改正の一文である。") {
			t.Fatalf("provision level fallback must stay out of chunks: %+v", c)
		}
	}
}

func TestParagraphNumFallsBackToNumAttr(t *testing.T) {
	doc := `<Law><LawNum>x</LawNum><LawBody><LawTitle>T</LawTitle><MainProvision>` +
		`<Article Num="1"><ArticleTitle>第一条</ArticleTitle>` +
		`<Paragraph Num="1"><ParagraphNum/><ParagraphSentence><Sentence>一つ目である。</Sentence></ParagraphSentence></Paragraph>` +
		`<Paragraph Num="2"><ParagraphNum/><ParagraphSentence><Sentence>二つ目である。</Sentence></ParagraphSentence></Paragraph>` +
		`<Paragraph Num="12"><ParagraphNum/><ParagraphSentence><Sentence>十二個目である。</Sentence></ParagraphSentence></Paragraph>` +
		`</Article></MainProvision></LawBody></Law>`
	md, chunks, err := Convert(strings.NewReader(doc), sampleMeta)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), "\n一つ目である。\n") {
		t.Fatalf("first paragraph must have no number: %s", md)
	}
	for _, want := range []string{"\n２　二つ目である。\n", "\n１２　十二個目である。\n"} {
		if !strings.Contains(string(md), want) {
			t.Fatalf("missing %q in:\n%s", want, md)
		}
	}
	if chunks[0].Text != "一つ目である。\n２　二つ目である。\n１２　十二個目である。" {
		t.Fatalf("chunk text = %q", chunks[0].Text)
	}
}

func TestRealLawParagraphsGetNumbersFromNumAttr(t *testing.T) {
	md, _ := mustConvert(t, "testdata/322AC0000000049.xml", realMeta)
	numbered := 0
	for l := range strings.SplitSeq(string(md), "\n") {
		r := []rune(l)
		if len(r) > 1 && r[0] >= '０' && r[0] <= '９' {
			numbered++
		}
	}
	// 番号が空でNum属性が2以上の項は151件、もとから番号を持つ項は47件。合わせて198件
	if numbered < 190 {
		t.Fatalf("numbered paragraphs = %d", numbered)
	}
	if n := strings.Count(string(md), "\n２　"); n < 50 {
		t.Fatalf("second paragraphs numbered = %d", n)
	}
}

func TestConvertRejectsBrokenInput(t *testing.T) {
	for name, body := range map[string]string{
		"empty":      "",
		"noLawBody":  `<Law><LawNum>x</LawNum></Law>`,
		"malformed":  `<Law><LawBody>`,
		"textAtRoot": "hello",
	} {
		if _, _, err := Convert(strings.NewReader(body), sampleMeta); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}
