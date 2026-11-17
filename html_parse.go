package htt

import (
	"slices"
	"strconv"
	"strings"

	"github.com/typelate/htt/lex"
)

// voidElements is the set of HTML elements that have no closing tag.
var voidElements = map[string]bool{
	"area":   true,
	"base":   true,
	"br":     true,
	"col":    true,
	"embed":  true,
	"hr":     true,
	"img":    true,
	"input":  true,
	"link":   true,
	"meta":   true,
	"param":  true,
	"source": true,
	"track":  true,
	"wbr":    true,
}

// rawTextElements is the set of HTML elements whose content is raw text
// (no child elements, but template actions are still recognized).
var rawTextElements = map[string]bool{
	"script":   true,
	"style":    true,
	"textarea": true,
	"title":    true,
}

var formattingElements = map[string]bool{
	"a": true, "b": true, "big": true, "code": true, "em": true,
	"font": true, "i": true, "nobr": true, "s": true, "small": true,
	"strike": true, "strong": true, "tt": true, "u": true,
}

// IsFormattingElement reports whether the named element, in lower case,
// is a formatting element (WHATWG §13.2.4.2), which the adoption agency
// algorithm reopens when another element's end tag closes it.
func IsFormattingElement(tag string) bool {
	return formattingElements[tag]
}

var impliedEndTagElements = map[string]bool{
	"dd": true, "dt": true, "li": true, "optgroup": true, "option": true,
	"p": true, "rb": true, "rp": true, "rt": true, "rtc": true,
	"caption": true, "colgroup": true, "tbody": true, "td": true,
	"tfoot": true, "th": true, "thead": true, "tr": true,
}

// HasImpliedEndTag reports whether the named element, in lower case, may
// leave out its end tag: the "generate implied end tags" set of WHATWG
// §13.2.6.3 with its "thoroughly" additions.
func HasImpliedEndTag(tag string) bool {
	return impliedEndTagElements[tag]
}

// htmlParse parses HTML with template actions into an
// HTMLDocumentFragmentNode. It accepts every template html/template
// parses: markup the lexer does not lex as a tag (see lex's lexHTMLText)
// becomes text and action nodes, with no element and no diagnostic.
func (t *Tree) htmlParse() error {
	frag := t.newHTMLDocumentFragment(t.peek().Pos)
	for t.peek().Type != lex.TokenTypeEOF {
		n, err := t.htmlParseNode()
		if err != nil {
			return err
		}
		switch n.(type) {
		case *endNode, *elseNode:
			return t.errorf("unexpected %s", n)
		}
		if n != nil {
			frag.Children = append(frag.Children, n)
		}
	}
	frag.end = lex.Pos(len(t.text))
	t.Root = t.newList(0)
	var n Node = frag
	t.Root.Nodes = append(t.Root.Nodes, n)
	t.Root.end = lex.Pos(len(t.text))
	return nil
}

// htmlParseNode parses one node. Attribute tokens reach it from the body of
// a control structure written among attributes, as in
// {{if .Show}}class="x"{{end}}.
func (t *Tree) htmlParseNode() (Node, error) {
	switch token := t.next(); token.Type {
	case lex.TokenTypeHTMLText, lex.TokenTypeHTMLAttrValue:
		if token.DegradedTag != "" {
			t.htmlDegraded = append(t.htmlDegraded, token.DegradedTag)
		}
		return t.newText(token.Pos, token.End, token.Value), nil
	case lex.TokenTypeHTMLComment:
		// Comment content in the body of a control structure, as in
		// <!-- {{if .A}}a{{end}} -->.
		return t.newText(token.Pos, token.End, token.Value), nil
	case lex.TokenTypeHTMLTagOpen:
		return t.htmlParseStartTag(token)
	case lex.TokenTypeHTMLEndTagOpen:
		return t.htmlParseEndTag(token)
	case lex.TokenTypeHTMLCommentOpen:
		return t.htmlParseComment(token)
	case lex.TokenTypeHTMLDoctype:
		return t.newHTMLDoctype(token.Pos, token.End, token.Value), nil
	case lex.TokenTypeHTMLAttrName:
		attr, err := t.htmlParseAttribute(token)
		if err != nil {
			return nil, err
		}
		return attr, nil
	case lex.TokenTypeComment:
		c := t.newComment(token.Pos, token.End, token.Value)
		c.TrimLeft = token.TrimLeft
		c.TrimRight = token.TrimRight
		return c, nil
	case lex.TokenTypeLeftDelim:
		t.actionLine = token.Line
		t.actionPos = token.Pos
		t.actionTrimLeft = token.TrimLeft
		defer t.clearActionLine()
		return t.htmlParseTemplateAction()
	case lex.TokenTypeEOF:
		t.backup()
		return nil, nil
	default:
		return nil, t.unexpected(token, "HTML input")
	}
}

// htmlParseStartTag parses an element after its '<'.
func (t *Tree) htmlParseStartTag(openToken lex.Token) (Node, error) {
	nameToken := t.next()
	if nameToken.Type != lex.TokenTypeHTMLStartTag {
		return nil, t.unexpected(nameToken, "HTML start tag name")
	}
	tagName := nameToken.Value
	elem := t.newHTMLElement(openToken.Pos, tagName)
	elem.AttrStart = nameToken.End

	if err := t.htmlParseAttributes(elem); err != nil {
		return nil, err
	}

	if voidElements[strings.ToLower(tagName)] {
		elem.Void = true
		return elem, nil
	}
	if elem.Void {
		// Self-closing: htmlParseAttributes saw "/>".
		return elem, nil
	}

	if rawTextElements[strings.ToLower(tagName)] {
		if err := t.htmlParseRawTextContent(elem); err != nil {
			return nil, err
		}
		return elem, nil
	}

	t.htmlAncestors = append(t.htmlAncestors, strings.ToLower(tagName))
	err := t.htmlParseChildren(elem)
	t.htmlAncestors = t.htmlAncestors[:len(t.htmlAncestors)-1]
	if err != nil {
		return nil, err
	}
	return elem, nil
}

// htmlParseAttributes parses attributes until '>' or '/>' is found.
func (t *Tree) htmlParseAttributes(elem *HTMLElementNode) error {
	for {
		token := t.next()
		switch token.Type {
		case lex.TokenTypeHTMLTagEnd:
			elem.AttrEnd = token.End
			elem.end = token.End
			return nil
		case lex.TokenTypeHTMLSelfClose:
			elem.Void = true
			elem.AttrEnd = token.End
			elem.end = token.End
			return nil
		case lex.TokenTypeHTMLAttrName:
			attr, err := t.htmlParseAttribute(token)
			if err != nil {
				return err
			}
			elem.Attr = append(elem.Attr, attr)
		case lex.TokenTypeLeftDelim:
			// Template action in attribute position (e.g., {{if .Show}}class="x"{{end}}).
			t.actionLine = token.Line
			t.actionPos = token.Pos
			t.actionTrimLeft = token.TrimLeft
			action, err := t.htmlParseTemplateAction()
			t.clearActionLine()
			if err != nil {
				return err
			}
			if action != nil {
				elem.Attr = append(elem.Attr, action)
			}
		case lex.TokenTypeEOF:
			return t.errorf("unclosed HTML tag %q", elem.TagName)
		default:
			return t.unexpected(token, "HTML tag attributes")
		}
	}
}

// htmlParseAttribute parses one attribute after its name token.
func (t *Tree) htmlParseAttribute(nameToken lex.Token) (*HTMLAttributeNode, error) {
	attr := t.newHTMLAttribute(nameToken.Pos)
	attr.Key = append(attr.Key, t.newText(nameToken.Pos, nameToken.End, nameToken.Value))
	attr.end = nameToken.End

	next := t.next()
	if next.Type != lex.TokenTypeHTMLAttrEquals {
		// A boolean attribute.
		t.backup()
		return attr, nil
	}
	attr.HasValue = true

	// The value may be several tokens with actions between them.
	var quote byte // the quote a quoted value opened with
	closed := false
	for {
		token := t.next()
		switch token.Type {
		case lex.TokenTypeHTMLAttrValue:
			v := token.Value
			if len(attr.Value) == 0 && (strings.HasPrefix(v, `"`) || strings.HasPrefix(v, "'")) {
				quote = v[0]
				v = v[1:]
			}
			closed = quote != 0 && strings.HasSuffix(v, string(quote))
			attr.Value = append(attr.Value, t.newText(token.Pos, token.End, token.Value))
			attr.end = token.End
		case lex.TokenTypeLeftDelim:
			if closed {
				// An action after the closing quote is in attribute
				// position (class="x"{{if .Y}} hidden{{end}}).
				t.backup()
				return attr, nil
			}
			// An {{end}} or {{else}} ends the control structure the
			// attribute is in, not the value: leave it for itemList.
			next := t.nextNonSpace()
			if next.Type == lex.TokenTypeEnd || next.Type == lex.TokenTypeElse {
				t.backup2(token)
				return attr, nil
			}
			t.backup()
			t.actionLine = token.Line
			t.actionPos = token.Pos
			t.actionTrimLeft = token.TrimLeft
			action, err := t.htmlParseTemplateAction()
			t.clearActionLine()
			if err != nil {
				return nil, err
			}
			if action != nil {
				attr.Value = append(attr.Value, action)
				attr.end = action.End()
			}
		default:
			// End of value — this token belongs to the next attribute or tag close.
			t.backup()
			return attr, nil
		}
	}
}

// htmlParseComment parses an HTML comment after '<!--' has been consumed.
func (t *Tree) htmlParseComment(openToken lex.Token) (Node, error) {
	comment := t.newHTMLComment(openToken.Pos)
	for {
		token := t.next()
		switch token.Type {
		case lex.TokenTypeHTMLComment:
			comment.Children = append(comment.Children, t.newText(token.Pos, token.End, token.Value))
		case lex.TokenTypeHTMLCommentClose:
			comment.end = token.End
			return comment, nil
		case lex.TokenTypeLeftDelim:
			// Template action inside comment.
			t.actionLine = token.Line
			t.actionPos = token.Pos
			t.actionTrimLeft = token.TrimLeft
			action, err := t.htmlParseTemplateAction()
			t.clearActionLine()
			if err != nil {
				return nil, err
			}
			if action != nil {
				comment.Children = append(comment.Children, action)
			}
		case lex.TokenTypeEOF:
			return nil, t.errorf("unclosed HTML comment")
		default:
			return nil, t.unexpected(token, "HTML comment")
		}
	}
}

// htmlParseEndTag parses, after its "</", an end tag that closes no open
// element, and keeps it as verbatim text. htmlParseChildren matches the
// end tags that do close one.
func (t *Tree) htmlParseEndTag(openToken lex.Token) (Node, error) {
	nameToken := t.next()
	if nameToken.Type != lex.TokenTypeHTMLEndTag {
		return nil, t.unexpected(nameToken, "HTML end tag name")
	}
	closeToken := t.next()
	if closeToken.Type != lex.TokenTypeHTMLTagEnd {
		return nil, t.unexpected(closeToken, "HTML end tag close")
	}
	text := t.newText(openToken.Pos, closeToken.End, "</"+nameToken.Value+">")
	text.Verbatim = true
	if t.htmlTakeDegraded(nameToken.Value) {
		// The end tag of an element whose start tag is text.
		return text, nil
	}
	// Browsers ignore a stray end tag; html/template writes it.
	t.diag(openToken.Pos, closeToken.End, SeverityWarning, DiagOrphanEndTag, msgOrphanEndTag, nameToken.Value)
	return text, nil
}

// htmlTakeDegraded reports whether name closes a start tag the lexer read
// as text inside the element being parsed, and forgets that start tag.
// Such an end tag is text too.
func (t *Tree) htmlTakeDegraded(name string) bool {
	for i := len(t.htmlDegraded) - 1; i >= t.htmlDegradedMin; i-- {
		if strings.EqualFold(t.htmlDegraded[i], name) {
			t.htmlDegraded = slices.Delete(t.htmlDegraded, i, i+1)
			return true
		}
	}
	return false
}

// htmlParseChildren parses an element's children up to its end tag.
func (t *Tree) htmlParseChildren(elem *HTMLElementNode) error {
	t.htmlDepth++
	defer func() { t.htmlDepth-- }()
	if t.htmlDepth > t.maxHTMLDepth {
		return t.errorf("maximum HTML element nesting depth exceeded (%d)", t.maxHTMLDepth)
	}
	// Start tags read as text inside this element are closed, if at all,
	// inside it.
	degradedMin := t.htmlDegradedMin
	t.htmlDegradedMin = len(t.htmlDegraded)
	defer func() {
		t.htmlDegraded = t.htmlDegraded[:t.htmlDegradedMin]
		t.htmlDegradedMin = degradedMin
	}()
	for {
		token := t.peek()
		switch token.Type {
		case lex.TokenTypeHTMLEndTagOpen:
			endTagOpen := t.next() // consume </
			nameToken := t.next()
			if nameToken.Type == lex.TokenTypeHTMLEndTag && t.htmlTakeDegraded(nameToken.Value) {
				// It closes a start tag that is text, so it is text too.
				closeToken := t.next() // consume >
				text := t.newText(endTagOpen.Pos, closeToken.End, "</"+nameToken.Value+">")
				text.Verbatim = true
				elem.Children = append(elem.Children, text)
				continue
			}
			if nameToken.Type == lex.TokenTypeHTMLEndTag && strings.EqualFold(nameToken.Value, elem.TagName) {
				closeToken := t.next()
				if closeToken.Type != lex.TokenTypeHTMLTagEnd {
					return t.unexpected(closeToken, "HTML end tag close")
				}
				elem.Closed = true
				elem.EndTagStart = endTagOpen.Pos
				elem.end = closeToken.End
				return nil
			}
			// An ancestor's end tag closes this element too, as in
			// x/net/html. Back up so the ancestor consumes it.
			if nameToken.Type == lex.TokenTypeHTMLEndTag && t.htmlHasAncestor(nameToken.Value) {
				tag := strings.ToLower(elem.TagName)
				if formattingElements[tag] {
					elem.Misnested = true
					t.diag(endTagOpen.Pos, nameToken.End, SeverityWarning, DiagMisnestedTag,
						msgMisnestedClose, nameToken.Value, elem.TagName, elem.TagName)
				}
				t.backup2(token)
				return nil
			}
			// An orphan end tag. Browsers ignore it; html/template
			// writes it.
			if nameToken.Type == lex.TokenTypeHTMLEndTag {
				closeToken := t.next() // consume >
				t.diag(endTagOpen.Pos, closeToken.End, SeverityWarning, DiagOrphanEndTag, msgOrphanEndTag, nameToken.Value)
				text := t.newText(endTagOpen.Pos, closeToken.End, "</"+nameToken.Value+">")
				text.Verbatim = true
				elem.Children = append(elem.Children, text)
			} else {
				return t.unexpected(nameToken, "HTML end tag name")
			}
		case lex.TokenTypeLeftDelim:
			// An {{end}} or {{else}} of an enclosing control structure
			// leaves the element unclosed.
			if t.htmlPeekIsControlTerminator() {
				return nil
			}
			n, err := t.htmlParseNode()
			if err != nil {
				return err
			}
			if n != nil {
				elem.Children = append(elem.Children, n)
			}
		case lex.TokenTypeEOF:
			// An unclosed element is not an error.
			return nil
		default:
			n, err := t.htmlParseNode()
			if err != nil {
				return err
			}
			if n != nil {
				elem.Children = append(elem.Children, n)
			}
		}
	}
}

// htmlParseRawTextContent parses the content of script, style, textarea
// and title, which the lexer emits as text and actions up to the end tag.
func (t *Tree) htmlParseRawTextContent(elem *HTMLElementNode) error {
	for {
		token := t.peek()
		switch token.Type {
		case lex.TokenTypeHTMLText:
			t.next()
			elem.Children = append(elem.Children, t.newText(token.Pos, token.End, token.Value))
		case lex.TokenTypeHTMLEndTagOpen:
			endTagOpen := t.next() // consume </
			nameToken := t.next()
			if nameToken.Type != lex.TokenTypeHTMLEndTag || !strings.EqualFold(nameToken.Value, elem.TagName) {
				return t.unexpected(nameToken, "raw text element end tag")
			}
			closeToken := t.next()
			if closeToken.Type != lex.TokenTypeHTMLTagEnd {
				return t.unexpected(closeToken, "raw text element end tag close")
			}
			elem.Closed = true
			elem.EndTagStart = endTagOpen.Pos
			elem.end = closeToken.End
			return nil
		case lex.TokenTypeLeftDelim:
			// An {{end}} or {{else}} that ends the control structure the
			// element began in leaves the element unclosed, as in
			// htmlParseChildren.
			if t.htmlPeekIsControlTerminator() {
				return nil
			}
			t.next()
			t.actionLine = token.Line
			t.actionPos = token.Pos
			t.actionTrimLeft = token.TrimLeft
			action, err := t.htmlParseTemplateAction()
			t.clearActionLine()
			if err != nil {
				return err
			}
			if action != nil {
				elem.Children = append(elem.Children, action)
			}
		case lex.TokenTypeComment:
			t.next()
			c := t.newComment(token.Pos, token.End, token.Value)
			c.TrimLeft = token.TrimLeft
			c.TrimRight = token.TrimRight
			elem.Children = append(elem.Children, c)
		case lex.TokenTypeEOF:
			return nil
		default:
			return t.unexpected(t.next(), "raw text element content")
		}
	}
}

// htmlPeekIsControlTerminator reports whether the action at the current
// position is {{end}} or {{else}}. It consumes nothing.
func (t *Tree) htmlPeekIsControlTerminator() bool {
	leftDelim := t.next() // consume {{
	keyword := t.nextNonSpace()
	isTerminator := keyword.Type == lex.TokenTypeEnd || keyword.Type == lex.TokenTypeElse
	t.backup2(leftDelim)
	return isTerminator
}

// htmlHasAncestor reports whether tag names an open ancestor element.
func (t *Tree) htmlHasAncestor(tag string) bool {
	return slices.Contains(t.htmlAncestors, strings.ToLower(tag))
}

// htmlParseTemplateAction parses an action after its left delimiter.
func (t *Tree) htmlParseTemplateAction() (Node, error) {
	switch token := t.nextNonSpace(); token.Type {
	case lex.TokenTypeBlock:
		return t.blockControl()
	case lex.TokenTypeBreak:
		return t.breakControl(t.actionPos, token.Pos, token.Line)
	case lex.TokenTypeContinue:
		return t.continueControl(t.actionPos, token.Pos, token.Line)
	case lex.TokenTypeDefine:
		return t.htmlDefineControl(token)
	case lex.TokenTypeElse:
		return t.elseControl()
	case lex.TokenTypeEnd:
		return t.endControl()
	case lex.TokenTypeIf:
		return t.ifControl()
	case lex.TokenTypeRange:
		return t.rangeControl()
	case lex.TokenTypeTemplate:
		return t.templateControl()
	case lex.TokenTypeWith:
		return t.withControl()
	default:
		t.backup()
		token := t.peek()
		pipe, err := t.pipeline("command", lex.TokenTypeRightDelim)
		if err != nil {
			return nil, err
		}
		a := t.newAction(t.actionPos, pipe.end, token.Line, pipe)
		a.TrimLeft = t.actionTrimLeft
		a.TrimRight = t.actionTrimRight
		return a, nil
	}
}

// htmlDefineControl parses {{define "name"}}...{{end}} after the define
// keyword and returns a DefineNode.
func (t *Tree) htmlDefineControl(openToken lex.Token) (Node, error) {
	const context = "define clause"
	// Saved now: parsing the body clears t.actionPos.
	actionPos := t.actionPos
	startTrimLeft := t.actionTrimLeft
	nameToken, err := t.expectOneOf(lex.TokenTypeString, lex.TokenTypeRawString, context)
	if err != nil {
		return nil, err
	}
	name, err := strconv.Unquote(nameToken.Value)
	if err != nil {
		return nil, t.error(err)
	}
	rightDelim, err := t.expect(lex.TokenTypeRightDelim, context)
	if err != nil {
		return nil, err
	}
	startTrimRight := rightDelim.TrimRight
	var end Node
	var body *ListNode
	body, end, err = t.itemList()
	if err != nil {
		return nil, err
	}
	en, ok := end.(*endNode)
	if !ok {
		return nil, t.errorf("unexpected %s in %s", end, context)
	}
	// Register the define as text/template does, so that a second
	// definition that is not empty is an error. DefinedTrees swaps in
	// the tree Extract builds for this body.
	if err := t.addDefinition(&Tree{Name: name, Root: body, text: t.text, leftDelim: t.leftDelim, rightDelim: t.rightDelim, ParseName: t.ParseName, Filepath: t.Filepath, HTMLMode: t.HTMLMode, RootTrimFirst: startTrimRight, RootTrimLast: en.TrimLeft, define: true}); err != nil {
		return nil, err
	}
	d := t.newDefine(actionPos, end.End(), openToken.Line, name, body)
	d.StartTrimLeft = startTrimLeft
	d.StartTrimRight = startTrimRight
	d.EndTrimLeft = en.TrimLeft
	d.EndTrimRight = en.TrimRight
	return d, nil
}

// htmlTextOrAction parses an HTML token in the body of a control structure.
func (t *Tree) htmlTextOrAction() (Node, error) {
	return t.htmlParseNode()
}
