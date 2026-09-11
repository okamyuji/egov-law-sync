// Package lawxml e-Gov法令XMLをLLM向けのMarkdownと条単位のJSONLに変換する
package lawxml

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

// xmlSpace 字下げに使われる文字。全角空白は表の空欄などの本文なので含めない
const xmlSpace = " \t\r\n"

// node 要素1つ。文字データと子要素の順序を保つため、partsに両方を出現順で持つ
type node struct {
	name  string
	attrs map[string]string
	parts []any // string または *node
}

// parse XMLトークンから木を作る。混在内容（SentenceのRuby）の順序を保つために自前で組む
func parse(r io.Reader) (*node, error) {
	dec := xml.NewDecoder(r)
	root := &node{name: "#root"}
	stack := []*node{root}
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		top := stack[len(stack)-1]
		switch v := tok.(type) {
		case xml.StartElement:
			n := &node{name: v.Name.Local, attrs: make(map[string]string, len(v.Attr))}
			for _, a := range v.Attr {
				n.attrs[a.Name.Local] = a.Value
			}
			top.parts = append(top.parts, n)
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if s := strings.Trim(string(v), xmlSpace); s != "" {
				top.parts = append(top.parts, s)
			}
		}
	}
	if len(root.parts) == 0 {
		return nil, errNoRoot
	}
	if n, ok := root.parts[0].(*node); ok {
		return n, nil
	}
	return nil, errNoRoot
}

// children 名前が一致する子要素を出現順で返す
func (n *node) children(name string) []*node {
	var out []*node
	for _, p := range n.parts {
		if c, ok := p.(*node); ok && c.name == name {
			out = append(out, c)
		}
	}
	return out
}

// child 名前が一致する最初の子要素。無ければnil
func (n *node) child(name string) *node {
	if cs := n.children(name); len(cs) > 0 {
		return cs[0]
	}
	return nil
}

// elements 子要素だけを出現順で返す
func (n *node) elements() []*node {
	var out []*node
	for _, p := range n.parts {
		if c, ok := p.(*node); ok {
			out = append(out, c)
		}
	}
	return out
}

// text 要素配下の文字データを出現順に連結する。Rt（ふりがな）は捨てる
func (n *node) text() string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	n.writeText(&b)
	return b.String()
}

func (n *node) writeText(b *strings.Builder) {
	for _, p := range n.parts {
		switch v := p.(type) {
		case string:
			b.WriteString(v)
		case *node:
			if v.name != "Rt" {
				v.writeText(b)
			}
		}
	}
}
