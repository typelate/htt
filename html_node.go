package htt

import (
	"strings"
	"sync/atomic"

	"github.com/typelate/htt/lex"
)

// HTMLDocumentFragmentNode holds the top-level nodes of a tree parsed in
// HTML mode. It imposes no well-formedness.
type HTMLDocumentFragmentNode struct {
	nodeParent
	tr         *Tree
	start, end lex.Pos
	Children   []Node
}

func (t *Tree) newHTMLDocumentFragment(start lex.Pos) *HTMLDocumentFragmentNode {
	return &HTMLDocumentFragmentNode{tr: t, start: start}
}

func (n *HTMLDocumentFragmentNode) Start() lex.Pos { return n.start }
func (n *HTMLDocumentFragmentNode) End() lex.Pos   { return n.end }
func (n *HTMLDocumentFragmentNode) String() string {
	return n.tr.src(n.Start(), n.End())
}

func (n *HTMLDocumentFragmentNode) writeTo(sb *strings.Builder) {
	for _, child := range n.Children {
		child.writeTo(sb)
	}
}

func (n *HTMLDocumentFragmentNode) tree() *Tree { return n.tr }

// HTMLElementNode is an HTML element. A tag with an action in its name is
// not an element: the parser reads it as text.
type HTMLElementNode struct {
	nodeParent
	tr          *Tree
	start, end  lex.Pos
	TagName     string       // The tag name as written.
	Attr        []Node       // HTMLAttributeNode values, and the actions and control structures among them.
	Children    []Node       // Child nodes in lexical order.
	Void        bool         // True for void elements (br, hr, img, etc.).
	Closed      bool         // True if the parser found a matching end tag.
	Misnested   bool         // True if an ancestor's end tag closed it first; a misnested-tag diagnostic reports it.
	AttrStart   lex.Pos      // Position after tag name (start of attribute region).
	AttrEnd     lex.Pos      // Position after '>' closing the opening tag.
	EndTagStart lex.Pos      // Position of '<' in '</tag>'.
	openCount   atomic.Int64 // <tag — before attrs
	attrCount   atomic.Int64 // ...> — after attrs
	closeCount  atomic.Int64 // </tag> — after children
}

func (t *Tree) newHTMLElement(start lex.Pos, tagName string) *HTMLElementNode {
	return &HTMLElementNode{tr: t, start: start, TagName: tagName}
}

func (n *HTMLElementNode) Start() lex.Pos { return n.start }
func (n *HTMLElementNode) End() lex.Pos   { return n.end }
func (n *HTMLElementNode) String() string { return n.tr.src(n.Start(), n.End()) }

func (n *HTMLElementNode) writeTo(sb *strings.Builder) {
	sb.WriteByte('<')
	sb.WriteString(n.TagName)
	for _, attr := range n.Attr {
		sb.WriteByte(' ')
		attr.writeTo(sb)
	}
	if n.Void {
		sb.WriteString(" />")
		return
	}
	sb.WriteByte('>')
	for _, child := range n.Children {
		child.writeTo(sb)
	}
	sb.WriteString("</")
	sb.WriteString(n.TagName)
	sb.WriteByte('>')
}

func (n *HTMLElementNode) tree() *Tree { return n.tr }

// HTMLAttributeNode is an HTML attribute. Key and Value interleave text
// with actions.
type HTMLAttributeNode struct {
	nodeParent
	tr         *Tree
	start, end lex.Pos
	Namespace  string // Optional namespace (e.g., "xlink" in "xlink:href").
	Key        []Node // Attribute name parts (supports dynamic names).
	Value      []Node // Attribute value parts (supports dynamic values).
	HasValue   bool   // Distinguishes `disabled` from `disabled=""`.
	count      atomic.Int64
}

func (t *Tree) newHTMLAttribute(start lex.Pos) *HTMLAttributeNode {
	return &HTMLAttributeNode{tr: t, start: start}
}

func (n *HTMLAttributeNode) Start() lex.Pos { return n.start }
func (n *HTMLAttributeNode) End() lex.Pos   { return n.end }
func (n *HTMLAttributeNode) String() string {
	return n.tr.src(n.Start(), n.End())
}

func (n *HTMLAttributeNode) writeTo(sb *strings.Builder) {
	if n.Namespace != "" {
		sb.WriteString(n.Namespace)
		sb.WriteByte(':')
	}
	for _, k := range n.Key {
		k.writeTo(sb)
	}
	if n.HasValue {
		sb.WriteByte('=')
		for _, v := range n.Value {
			v.writeTo(sb)
		}
	}
}

func (n *HTMLAttributeNode) tree() *Tree { return n.tr }

// HTMLCommentNode is an HTML comment. Children interleaves its text with
// the actions written in it.
type HTMLCommentNode struct {
	nodeParent
	tr         *Tree
	start, end lex.Pos
	Children   []Node // Comment content: TextNode and ActionNode interleaved.
	count      atomic.Int64
}

func (t *Tree) newHTMLComment(pos lex.Pos) *HTMLCommentNode {
	return &HTMLCommentNode{tr: t, start: pos}
}

func (n *HTMLCommentNode) Start() lex.Pos { return n.start }
func (n *HTMLCommentNode) End() lex.Pos   { return n.end }
func (n *HTMLCommentNode) String() string {
	return n.tr.src(n.Start(), n.End())
}
func (n *HTMLCommentNode) tree() *Tree { return n.tr }

func (n *HTMLCommentNode) writeTo(sb *strings.Builder) {
	sb.WriteString("<!--")
	for _, child := range n.Children {
		child.writeTo(sb)
	}
	sb.WriteString("-->")
}

// HTMLDoctypeNode is a DOCTYPE declaration.
type HTMLDoctypeNode struct {
	nodeParent
	tr         *Tree
	start, end lex.Pos
	Text       string // The raw DOCTYPE text, e.g. "<!DOCTYPE html>".
	count      atomic.Int64
}

func (t *Tree) newHTMLDoctype(start, end lex.Pos, text string) *HTMLDoctypeNode {
	return &HTMLDoctypeNode{tr: t, start: start, end: end, Text: text}
}

func (n *HTMLDoctypeNode) Start() lex.Pos { return n.start }
func (n *HTMLDoctypeNode) End() lex.Pos   { return n.end }
func (n *HTMLDoctypeNode) String() string {
	return n.tr.src(n.Start(), n.End())
}
func (n *HTMLDoctypeNode) writeTo(sb *strings.Builder) { sb.WriteString(n.Text) }
func (n *HTMLDoctypeNode) tree() *Tree                 { return n.tr }
