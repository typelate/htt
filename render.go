package htt

import (
	"io"
	"strings"

	"golang.org/x/net/html"
)

// markup records what a template's source writes that an html.Node
// cannot hold, so that render can write it as html/template would. A nil
// *markup records nothing.
type markup struct {
	open  map[*html.Node]bool          // elements whose source writes no end tag
	attrs map[*html.Node][]*attrSyntax // each attribute's syntax, by index in Attr
}

// sourceAttr is an attribute with how its source writes it.
type sourceAttr struct {
	html.Attribute
	syntax *attrSyntax // nil: a boolean attribute, or no source to follow
}

// attrSyntax is how an attribute's source writes its value.
type attrSyntax struct {
	quote byte   // '"', '\'', or 0 for an unquoted value
	raw   string // the value as written between the quotes
}

// setAttrs records the syntax of n's attributes, which are attrs.
func (m *markup) setAttrs(n *html.Node, attrs []sourceAttr) {
	if m == nil {
		return
	}
	var syntax []*attrSyntax
	for i, a := range attrs {
		if a.syntax == nil {
			continue
		}
		if syntax == nil {
			syntax = make([]*attrSyntax, len(attrs))
		}
		syntax[i] = a.syntax
	}
	if syntax == nil {
		return
	}
	if m.attrs == nil {
		m.attrs = make(map[*html.Node][]*attrSyntax)
	}
	m.attrs[n] = syntax
}

// syntax returns the syntax of n's i'th attribute, or nil.
func (m *markup) syntax(n *html.Node, i int) *attrSyntax {
	if m == nil || i >= len(m.attrs[n]) {
		return nil
	}
	return m.attrs[n][i]
}

// leaveOpen records that the source writes no end tag for n.
func (m *markup) leaveOpen(n *html.Node) {
	if m == nil {
		return
	}
	if m.open == nil {
		m.open = make(map[*html.Node]bool)
	}
	m.open[n] = true
}

// render writes n as html/template would write the template: void
// elements with no slash, empty attribute values bare, an attribute m has
// the syntax of as its source writes it, and no end tag for an element m
// says the source leaves open.
func render(w io.Writer, n *html.Node, m *markup) error {
	return renderer{m}.node(w, n)
}

type renderer struct{ m *markup }

func (r renderer) node(w io.Writer, n *html.Node) error {
	switch n.Type {
	case html.DocumentNode:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if err := r.node(w, c); err != nil {
				return err
			}
		}
		return nil

	case html.TextNode:
		_, err := io.WriteString(w, htmlEscapeString(n.Data))
		return err

	case html.ElementNode:
		return r.element(w, n)

	case html.CommentNode:
		if _, err := io.WriteString(w, "<!--"); err != nil {
			return err
		}
		if _, err := io.WriteString(w, n.Data); err != nil {
			return err
		}
		_, err := io.WriteString(w, "-->")
		return err

	case html.DoctypeNode:
		if _, err := io.WriteString(w, "<!DOCTYPE "); err != nil {
			return err
		}
		if _, err := io.WriteString(w, n.Data); err != nil {
			return err
		}
		_, err := io.WriteString(w, ">")
		return err

	case html.RawNode:
		_, err := io.WriteString(w, n.Data)
		return err

	default:
		return nil
	}
}

func (r renderer) element(w io.Writer, n *html.Node) error {
	if _, err := io.WriteString(w, "<"); err != nil {
		return err
	}
	if _, err := io.WriteString(w, n.Data); err != nil {
		return err
	}
	for i, a := range n.Attr {
		if syntax := r.m.syntax(n, i); syntax != nil {
			if err := renderSourceAttr(w, a, syntax); err != nil {
				return err
			}
			continue
		}
		if err := renderAttr(w, a); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, ">"); err != nil {
		return err
	}

	if isVoidElement(n.Data) {
		return nil
	}

	rawText := isRawTextElement(n.Data)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if rawText && c.Type == html.TextNode {
			if _, err := io.WriteString(w, c.Data); err != nil {
				return err
			}
			continue
		}
		if err := r.node(w, c); err != nil {
			return err
		}
	}

	if r.m != nil && r.m.open[n] {
		return nil
	}
	if _, err := io.WriteString(w, "</"); err != nil {
		return err
	}
	if _, err := io.WriteString(w, n.Data); err != nil {
		return err
	}
	_, err := io.WriteString(w, ">")
	return err
}

func renderAttr(w io.Writer, a html.Attribute) error {
	if _, err := io.WriteString(w, " "); err != nil {
		return err
	}
	if a.Namespace != "" {
		if _, err := io.WriteString(w, a.Namespace); err != nil {
			return err
		}
		if _, err := io.WriteString(w, ":"); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, a.Key); err != nil {
		return err
	}
	if a.Val == "" {
		return nil
	}
	if _, err := io.WriteString(w, `="`); err != nil {
		return err
	}
	if _, err := io.WriteString(w, htmlEscapeAttrVal(a.Val)); err != nil {
		return err
	}
	_, err := io.WriteString(w, `"`)
	return err
}

// renderSourceAttr writes a as its source writes it: the value, as
// syntax has it, between the source's quotes.
func renderSourceAttr(w io.Writer, a html.Attribute, syntax *attrSyntax) error {
	var b strings.Builder
	b.WriteByte(' ')
	if a.Namespace != "" {
		b.WriteString(a.Namespace)
		b.WriteByte(':')
	}
	b.WriteString(a.Key)
	b.WriteByte('=')
	if syntax.quote != 0 {
		b.WriteByte(syntax.quote)
	}
	b.WriteString(syntax.raw)
	if syntax.quote != 0 {
		b.WriteByte(syntax.quote)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// quotedAttrEscaper and unquotedAttrEscaper escape an action's output in
// an attribute value as html/template's htmlEscaper and
// htmlNospaceEscaper do.
var (
	quotedAttrEscaper = strings.NewReplacer(
		"\x00", "\uFFFD",
		`"`, "&#34;",
		`&`, "&amp;",
		`'`, "&#39;",
		`+`, "&#43;",
		`<`, "&lt;",
		`>`, "&gt;",
	)
	unquotedAttrEscaper = strings.NewReplacer(
		"\x00", "&#xfffd;",
		"\t", "&#9;",
		"\n", "&#10;",
		"\v", "&#11;",
		"\f", "&#12;",
		"\r", "&#13;",
		" ", "&#32;",
		`"`, "&#34;",
		`&`, "&amp;",
		`'`, "&#39;",
		`+`, "&#43;",
		`<`, "&lt;",
		`=`, "&#61;",
		`>`, "&gt;",
		"`", "&#96;",
	)
)

// escapeAttrText escapes an action's output for an attribute value
// between quote, 0 for none. An empty output in an unquoted value would
// end the value, so it is ZgotmplZ, as in html/template.
func escapeAttrText(s string, quote byte) string {
	if quote != 0 {
		return quotedAttrEscaper.Replace(s)
	}
	if s == "" {
		return "ZgotmplZ"
	}
	return unquotedAttrEscaper.Replace(s)
}

// renderTextEscaper escapes text as x/net/html's Render does.
var renderTextEscaper = strings.NewReplacer(
	`&`, "&amp;",
	`'`, "&#39;",
	`<`, "&lt;",
	`>`, "&gt;",
	`"`, "&#34;",
	"\r", "&#13;",
)

func htmlEscapeString(s string) string {
	return renderTextEscaper.Replace(s)
}

// attrEscaper escapes a value written between double quotes, so a single
// quote stays as it is.
var attrEscaper = strings.NewReplacer(
	`&`, "&amp;",
	`<`, "&lt;",
	`>`, "&gt;",
	`"`, "&#34;",
	"\n", "&#10;",
	"\r", "&#13;",
)

func htmlEscapeAttrVal(s string) string {
	return attrEscaper.Replace(s)
}

func isVoidElement(tag string) bool {
	switch tag {
	case "area", "base", "br", "col", "embed", "hr", "img", "input",
		"link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}

// IsVoidElement reports whether the named element is void: it has no
// content and no end tag.
func IsVoidElement(tag string) bool {
	return isVoidElement(tag)
}

// IsRawTextElement reports whether the named element is script or style,
// whose text is written unescaped.
func IsRawTextElement(tag string) bool {
	return isRawTextElement(tag)
}

// changesTextContext reports whether a browser reads what follows the
// named element's start tag as something other than HTML text. Such an
// element always gets its end tag, even where the source leaves it out,
// so that output escaped as HTML text does not land in script.
func changesTextContext(tag string) bool {
	switch strings.ToLower(tag) {
	case "script", "style", "textarea", "title",
		"xmp", "iframe", "noembed", "noframes", "noscript", "plaintext":
		return true
	}
	return false
}

// isRawTextElement reports whether tag is script or style. A browser
// decodes no entities there, so their text is written as it is.
func isRawTextElement(tag string) bool {
	switch tag {
	case "script", "style":
		return true
	}
	return false
}
