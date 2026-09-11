package lawxml

import (
	"strings"
	"testing"
)

func mustParse(t *testing.T, doc string) *node {
	t.Helper()
	n, err := parse(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestTextKeepsOrderAndDropsRuby(t *testing.T) {
	n := mustParse(t, `<S>あ<Ruby>労<Rt>ろう</Rt></Ruby>い<Ruby>働<Rt>どう</Rt></Ruby>う</S>`)
	if got := n.text(); got != "あ労い働う" {
		t.Fatalf("text = %q", got)
	}
}

func TestTextKeepsFullWidthSpaceButDropsIndentation(t *testing.T) {
	n := mustParse(t, "<T>\n  <C>　</C>\n  <C>あ</C>\n</T>")
	cs := n.children("C")
	if len(cs) != 2 || cs[0].text() != "　" || cs[1].text() != "あ" {
		t.Fatalf("children = %q", []string{cs[0].text(), cs[1].text()})
	}
	if got := n.text(); got != "　あ" {
		t.Fatalf("text = %q", got)
	}
}

func TestNilChildTextIsEmpty(t *testing.T) {
	n := mustParse(t, `<A><B>x</B></A>`)
	if n.child("missing") != nil {
		t.Fatal("child must be nil when absent")
	}
	if got := n.child("missing").text(); got != "" {
		t.Fatalf("text = %q", got)
	}
	if len(n.elements()) != 1 || n.elements()[0].name != "B" {
		t.Fatalf("elements = %v", n.elements())
	}
	if n.child("B").attrs == nil {
		t.Fatal("attrs must be allocated")
	}
}

func TestParseRejectsDocumentsWithoutAnElement(t *testing.T) {
	for _, doc := range []string{"", "   \n ", "bare text"} {
		if _, err := parse(strings.NewReader(doc)); err == nil {
			t.Fatalf("%q: expected error", doc)
		}
	}
}

func TestConvertHandlesParagraphWithoutSentence(t *testing.T) {
	doc := `<Law><LawNum>x</LawNum><LawBody><LawTitle>T</LawTitle><MainProvision>` +
		`<Article Num="1"><ArticleTitle>第一条</ArticleTitle>` +
		`<Paragraph Num="1"><ParagraphNum>２</ParagraphNum></Paragraph>` +
		`</Article></MainProvision></LawBody></Law>`
	md, chunks, err := Convert(strings.NewReader(doc), sampleMeta)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 1 || chunks[0].Text != "２　" {
		t.Fatalf("chunks = %+v", chunks)
	}
	if !strings.Contains(string(md), "#### 第一条") {
		t.Fatalf("md = %s", md)
	}
}
