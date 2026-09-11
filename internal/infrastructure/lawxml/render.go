package lawxml

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

var errNoRoot = errors.New("lawxml: empty document")

// hierarchy 本則の階層要素とそのTitle要素
var hierarchy = map[string]string{
	"Part": "PartTitle", "Chapter": "ChapterTitle", "Section": "SectionTitle",
	"Subsection": "SubsectionTitle", "Division": "DivisionTitle",
}

// converter 1文書分の変換状態。pathは本則の階層のTitleを積む
type converter struct {
	meta   law.Law
	lawNum string
	md     bytes.Buffer
	chunks []law.Chunk
	path   []string
}

// Convert XMLを読み、MarkdownとChunkを返す。同じ入力からは同じ出力になる
func Convert(r io.Reader, meta law.Law) ([]byte, []law.Chunk, error) {
	root, err := parse(r)
	if err != nil {
		return nil, nil, err
	}
	body := root.child("LawBody")
	if body == nil {
		return nil, nil, errors.New("lawxml: LawBody not found")
	}
	c := &converter{meta: meta, lawNum: root.child("LawNum").text()}
	c.frontMatter()
	c.line("# " + body.child("LawTitle").text())
	if mp := body.child("MainProvision"); mp != nil {
		c.provision(mp, 2)
	}
	for _, sp := range body.children("SupplProvision") {
		c.supplProvision(sp)
	}
	for _, at := range body.children("AppdxTable") {
		c.appdxTable(at)
	}
	return c.md.Bytes(), c.chunks, nil
}

func (c *converter) frontMatter() {
	c.md.WriteString("---\n")
	c.md.WriteString("law_id: " + string(c.meta.ID) + "\n")
	c.md.WriteString("revision_id: " + string(c.meta.RevisionID) + "\n")
	c.md.WriteString("law_title: " + c.meta.Title + "\n")
	c.md.WriteString("law_num: " + c.lawNum + "\n")
	c.md.WriteString("enforcement_date: " + c.meta.EnforcementDate + "\n")
	c.md.WriteString("source_url: " + law.SourceURL(c.meta.ID) + "\n")
	c.md.WriteString("generated_by: egov-law-sync\n")
	c.md.WriteString("---\n")
}

// line 1行と空行を書く。Markdownの段落は空行で区切る
func (c *converter) line(s string) {
	c.md.WriteString("\n" + s + "\n")
}

// provision 本則の階層を再帰で書く。levelは見出しの深さ（編が2）
func (c *converter) provision(n *node, level int) {
	for _, e := range n.elements() {
		c.member(e, level)
	}
}

// member 本則と附則に共通の構成要素1つ。階層と条と項は形を保ち、それ以外はfallbackへ回す
func (c *converter) member(e *node, level int) {
	switch {
	case hierarchy[e.name] != "":
		title := e.child(hierarchy[e.name]).text()
		c.line(strings.Repeat("#", min(level, 6)) + " " + title)
		c.path = append(c.path, title)
		c.provision(e, level+1)
		c.path = c.path[:len(c.path)-1]
	case e.name == "Article":
		c.article(e)
	case e.name == "Paragraph":
		c.looseParagraph(e)
	default:
		// 条にも項にも属さない要素の本文はMarkdownにだけ残す。別表と同じ扱い
		c.fallback(e)
	}
}

// article 1条をMarkdownに書き、Chunkを1つ足す
func (c *converter) article(a *node) {
	title := a.child("ArticleTitle").text()
	caption := a.child("ArticleCaption").text()
	c.line("#### " + title + caption)
	c.addChunk(title, stripParens(caption), c.blocks(a, 0))
}

// looseParagraph 条に属さない項（短い法令や附則）。1項を1Chunkにする
func (c *converter) looseParagraph(p *node) {
	lines := c.paragraph(p)
	c.addChunk("", stripParens(p.child("ParagraphCaption").text()), lines)
}

// paragraph 1項をMarkdownに書き、Chunk用の行を返す
func (c *converter) paragraph(p *node) []string {
	if caption := p.child("ParagraphCaption").text(); caption != "" {
		c.line(caption)
	}
	body := sentences(p.child("ParagraphSentence"))
	if num := paragraphNum(p); num != "" {
		body = num + "　" + body
	}
	c.line(body)
	return append([]string{body}, c.blocks(p, 0)...)
}

// paragraphNum 項番号。ParagraphNumが空でもNum属性が2以上なら全角数字で補う。e-GovのOldNum項はParagraphNumを空にする
func paragraphNum(p *node) string {
	if num := p.child("ParagraphNum").text(); num != "" {
		return num
	}
	n, err := strconv.Atoi(p.attrs["Num"])
	if err != nil || n < 2 {
		return ""
	}
	return strings.Map(func(r rune) rune { return r - '0' + '０' }, strconv.Itoa(n))
}

// blocks 見出しと本文を書いた後に残る子要素を出現順に書き、Chunk用の行を返す。depthが1以上なら既に箇条書きの中にいる
func (c *converter) blocks(n *node, depth int) []string {
	var lines []string
	inList := depth > 0
	for _, e := range n.elements() {
		// 見出しや本文として既に書いた子は、親と同じ名前で始まる
		if strings.HasPrefix(e.name, n.name) {
			continue
		}
		switch {
		case e.name == "Item" || isSubitem(e.name):
			if !inList {
				// 箇条書きの前には空行が要る。前の段落と地続きに見えてしまうため
				c.md.WriteString("\n")
				inList = true
			}
			lines = append(lines, c.item(e, depth)...)
		case e.name == "Paragraph":
			inList = false
			lines = append(lines, c.paragraph(e)...)
		case e.name == "TableStruct":
			inList = false
			lines = append(lines, c.table(e)...)
		default:
			inList = false
			lines = append(lines, c.fallback(e)...)
		}
	}
	return lines
}

// item 号とその下位（Subitem1、Subitem2、Subitem3）を箇条書きにする
func (c *converter) item(it *node, depth int) []string {
	title := it.child(it.name + "Title").text()
	text := title + "　" + sentences(it.child(it.name+"Sentence"))
	c.md.WriteString(strings.Repeat("  ", depth) + "- " + text + "\n")
	return append([]string{text}, c.blocks(it, depth+1)...)
}

// fallback 明示的に扱わない要素。配下のSentenceを出現順に段落として書き出し、本文を落とさない
func (c *converter) fallback(n *node) []string {
	var lines []string
	for _, e := range n.elements() {
		if e.name != "Sentence" {
			lines = append(lines, c.fallback(e)...)
			continue
		}
		if t := e.text(); t != "" {
			c.line(t)
			lines = append(lines, t)
		}
	}
	return lines
}

// isSubitem Subitem1などの下位項目そのものか。Subitem1TitleとSubitem1Sentenceは中身なので除く
func isSubitem(name string) bool {
	return strings.HasPrefix(name, "Subitem") &&
		!strings.HasSuffix(name, "Title") && !strings.HasSuffix(name, "Sentence")
}

// table 表を行ごとに1行にする。列数が行ごとに変わるのでMarkdownの表にはしない
func (c *converter) table(ts *node) []string {
	var lines []string
	c.md.WriteString("\n")
	for _, tbl := range ts.children("Table") {
		for _, row := range tbl.children("TableRow") {
			var cols []string
			for _, col := range row.children("TableColumn") {
				cols = append(cols, col.text())
			}
			l := strings.Join(cols, "｜")
			c.md.WriteString(l + "\n")
			lines = append(lines, l)
		}
	}
	return lines
}

// supplProvision 附則。条があれば条ごと、無ければ項ごとにChunkにする
func (c *converter) supplProvision(sp *node) {
	title := "附則"
	if num := sp.attrs["AmendLawNum"]; num != "" {
		title += "（" + num + "）"
	}
	c.line("## " + title)
	c.path = []string{title}
	for _, e := range sp.elements() {
		if e.name == "SupplProvisionLabel" {
			continue
		}
		c.member(e, 3)
	}
	c.path = nil
}

// appdxTable 別表。Markdownにだけ書き、Chunkにはしない
func (c *converter) appdxTable(at *node) {
	c.line("## 別表（" + at.child("AppdxTableTitle").text() + "）")
	if rel := at.child("RelatedArticleNum").text(); rel != "" {
		c.line(rel)
	}
	c.blocks(at, 0)
}

func (c *converter) addChunk(article, articleTitle string, lines []string) {
	c.chunks = append(c.chunks, law.Chunk{
		LawID: c.meta.ID, RevisionID: c.meta.RevisionID, LawTitle: c.meta.Title, LawNum: c.lawNum,
		EnforcementDate: c.meta.EnforcementDate, Path: strings.Join(c.path, "/"),
		Article: article, ArticleTitle: articleTitle, Text: strings.Join(lines, "\n"),
		SourceURL: law.SourceURL(c.meta.ID),
	})
}

// sentences ParagraphSentenceやItemSentenceの中身。Columnは全角空白で結び、Sentenceはそのまま連結する
func sentences(n *node) string {
	if n == nil {
		return ""
	}
	if cols := n.children("Column"); len(cols) > 0 {
		parts := make([]string, 0, len(cols))
		for _, col := range cols {
			parts = append(parts, col.text())
		}
		return strings.Join(parts, "　")
	}
	return n.text()
}

// stripParens 見出しの前後の全角括弧を外す
func stripParens(s string) string {
	return strings.TrimSuffix(strings.TrimPrefix(s, "（"), "）")
}
