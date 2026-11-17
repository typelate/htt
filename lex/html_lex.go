package lex

import (
	"strings"
)

// The token types of HTML mode.
const (
	TokenTypeHTMLText         TokenType = iota + 100 // text outside HTML tags and template actions
	TokenTypeHTMLStartTag                            // start tag name, e.g. "div" from "<div"
	TokenTypeHTMLEndTag                              // end tag name, e.g. "div" from "</div>"
	TokenTypeHTMLTagOpen                             // '<' that opens a start tag
	TokenTypeHTMLTagEnd                              // '>' that closes a tag
	TokenTypeHTMLSelfClose                           // '/>' that self-closes a tag
	TokenTypeHTMLEndTagOpen                          // '</' that opens an end tag
	TokenTypeHTMLAttrName                            // attribute name
	TokenTypeHTMLAttrEquals                          // '=' between attribute name and value
	TokenTypeHTMLAttrValue                           // attribute value (includes quotes if present)
	TokenTypeHTMLComment                             // HTML comment content (between <!-- and -->)
	TokenTypeHTMLCommentOpen                         // '<!--'
	TokenTypeHTMLCommentClose                        // '-->'
	TokenTypeHTMLDoctype                             // DOCTYPE content
)

// rawTextElements is the set of HTML elements whose content is raw text.
var rawTextElements = map[string]bool{
	"script": true, "style": true, "textarea": true, "title": true,
}

// lexHTMLTag lexes the '<' at the current position and picks the state for
// what it opens: a start tag, an end tag, a comment or a doctype.
func lexHTMLTag(l *Scanner) stateFn {
	l.next() // consume '<'
	rest := l.input[l.pos:]
	if strings.HasPrefix(rest, "!--") {
		// Comment: <!--
		l.pos += 3
		l.htmlReturnState = lexHTMLCommentBody
		return l.emit(TokenTypeHTMLCommentOpen)
	}
	if len(rest) > 0 && (rest[0] == '!' || rest[0] == '?') {
		// DOCTYPE or processing instruction: <!DOCTYPE ...> or <?...?>
		return lexHTMLDoctype
	}
	if len(rest) > 0 && rest[0] == '/' {
		// End tag: </
		l.next() // consume '/'
		l.htmlReturnState = lexHTMLEndTagName
		return l.emit(TokenTypeHTMLEndTagOpen)
	}
	// Start tag: <tagname
	l.htmlReturnState = lexHTMLStartTagName
	return l.emit(TokenTypeHTMLTagOpen)
}

// lexHTMLDoctype scans a doctype or processing instruction through its
// '>', from the '!' or '?' after the '<'.
func lexHTMLDoctype(l *Scanner) stateFn {
	// Scan until '>'
	for {
		r := l.next()
		if r == eof {
			return l.errorf("unclosed DOCTYPE")
		}
		if r == '>' {
			l.htmlReturnState = lexHTMLText
			return l.emit(TokenTypeHTMLDoctype)
		}
	}
}

// lexHTMLStartTagName scans the tag name after '<'.
func lexHTMLStartTagName(l *Scanner) stateFn {
	if !scanHTMLTagName(l) {
		return l.errorf("expected tag name")
	}
	tagName := strings.ToLower(l.input[l.start:l.pos])
	if rawTextElements[tagName] {
		l.htmlRawTag = tagName
	}
	l.htmlReturnState = lexHTMLAfterTagName
	return l.emit(TokenTypeHTMLStartTag)
}

// lexHTMLEndTagName scans the tag name after "</".
func lexHTMLEndTagName(l *Scanner) stateFn {
	if !scanHTMLTagName(l) {
		return l.errorf("expected tag name after </")
	}
	l.htmlReturnState = lexHTMLEndTagClose
	return l.emit(TokenTypeHTMLEndTag)
}

// lexHTMLEndTagClose scans for '>' after an end tag name, skipping whitespace.
func lexHTMLEndTagClose(l *Scanner) stateFn {
	for isSpace(l.peek()) {
		l.next()
	}
	if l.pos > l.start {
		l.ignore()
	}
	r := l.next()
	if r == eof {
		return l.errorf("expected > to close end tag, got EOF")
	}
	if r != '>' {
		return l.errorf("expected > to close end tag, got %c", r)
	}
	l.htmlReturnState = lexHTMLText
	return l.emit(TokenTypeHTMLTagEnd)
}

// scanHTMLTagName consumes a tag name: an ASCII letter, then letters,
// digits and hyphens. It reports whether it found one.
func scanHTMLTagName(l *Scanner) bool {
	r := l.next()
	if r == eof || !isASCIIAlpha(r) {
		l.backup()
		return false
	}
	for {
		r = l.next()
		if r == eof {
			l.backup()
			break
		}
		if !isHTMLTagNameChar(r) {
			l.backup()
			break
		}
	}
	return l.pos > l.start
}

// lexHTMLAfterTagName scans what follows a tag name or an attribute: an
// attribute, an action, '>' or "/>".
func lexHTMLAfterTagName(l *Scanner) stateFn {
	// Skip whitespace first.
	for isSpace(l.peek()) {
		l.next()
	}
	if l.pos > l.start {
		l.ignore()
	}

	r := l.peek()
	switch r {
	case eof:
		return l.errorf("unclosed HTML tag")
	case '>':
		l.next()
		if l.htmlRawTag != "" {
			l.htmlReturnState = lexHTMLRawText
		} else {
			l.htmlReturnState = lexHTMLText
		}
		return l.emit(TokenTypeHTMLTagEnd)
	case '/':
		l.next()
		if l.peek() == '>' {
			l.next()
			l.htmlRawTag = "" // self-closing raw text elements have no body
			l.htmlReturnState = lexHTMLText
			return l.emit(TokenTypeHTMLSelfClose)
		}
		return l.errorf("unexpected / in HTML tag")
	}
	// Check for template action start.
	if strings.HasPrefix(l.input[l.pos:], l.Options.LeftDelim) {
		l.htmlReturnState = lexHTMLAfterTagName
		return scanLeftDelim
	}
	// Must be an attribute name.
	return lexHTMLAttrName
}

// lexHTMLAttrName scans an HTML attribute name.
func lexHTMLAttrName(l *Scanner) stateFn {
	for {
		r := l.next()
		if r == eof {
			return l.errorf("unclosed HTML tag")
		}
		if r == '=' || r == '>' || isSpace(r) || r == '/' {
			l.backup()
			break
		}
		// Check for template action inside attribute name.
		l.backup()
		if strings.HasPrefix(l.input[l.pos:], l.Options.LeftDelim) {
			break
		}
		l.next() // re-consume the rune
	}
	if l.pos > l.start {
		l.htmlReturnState = lexHTMLAfterAttrName
		return l.emit(TokenTypeHTMLAttrName)
	}
	return l.errorf("expected attribute name")
}

// lexHTMLAfterAttrName scans the '=' after an attribute name, if there is
// one.
func lexHTMLAfterAttrName(l *Scanner) stateFn {
	// Skip whitespace.
	for isSpace(l.peek()) {
		l.next()
	}
	if l.pos > l.start {
		l.ignore()
	}
	r := l.peek()
	if r == '=' {
		l.next()
		l.htmlReturnState = lexHTMLAfterAttrEquals
		return l.emit(TokenTypeHTMLAttrEquals)
	}
	// No value — boolean attribute. Return to scanning for more attrs.
	return lexHTMLAfterTagName
}

// lexHTMLAfterAttrEquals picks the state for the value after '='.
func lexHTMLAfterAttrEquals(l *Scanner) stateFn {
	// Skip whitespace after '='.
	for isSpace(l.peek()) {
		l.next()
	}
	if l.pos > l.start {
		l.ignore()
	}
	r := l.peek()
	switch r {
	case '"':
		return lexHTMLAttrValueDoubleQuoted
	case '\'':
		return lexHTMLAttrValueSingleQuoted
	default:
		return lexHTMLAttrValueUnquoted
	}
}

// lexHTMLAttrValueDoubleQuoted scans a double-quoted attribute value from
// its opening quote.
func lexHTMLAttrValueDoubleQuoted(l *Scanner) stateFn {
	l.next() // consume opening '"'
	return lexHTMLAttrValueDoubleQuotedInner(l)
}

// lexHTMLAttrValueDoubleQuotedInner scans a double-quoted value after its
// opening quote, or after an action in it.
func lexHTMLAttrValueDoubleQuotedInner(l *Scanner) stateFn {
	for {
		rest := l.input[l.pos:]
		idxDelim := strings.Index(rest, l.Options.LeftDelim)
		idxQuote := strings.IndexByte(rest, '"')
		switch {
		case idxQuote < 0:
			return l.errorf("unterminated attribute value")
		case idxDelim >= 0 && idxDelim < idxQuote:
			// Template action inside quoted value.
			l.pos += Pos(idxDelim)
			if l.pos > l.start {
				l.line += strings.Count(l.input[l.start:l.pos], "\n")
				l.htmlReturnState = lexHTMLAttrValueDoubleQuotedInner
				return l.emit(TokenTypeHTMLAttrValue)
			}
			l.htmlReturnState = lexHTMLAttrValueDoubleQuotedInner
			return scanLeftDelim
		default:
			// End of quoted value — include the closing quote.
			l.pos += Pos(idxQuote) + 1
			l.line += strings.Count(l.input[l.start:l.pos], "\n")
			l.htmlReturnState = lexHTMLAfterTagName
			return l.emit(TokenTypeHTMLAttrValue)
		}
	}
}

// lexHTMLAttrValueSingleQuoted scans a single-quoted attribute value.
func lexHTMLAttrValueSingleQuoted(l *Scanner) stateFn {
	l.next() // consume opening quote
	return lexHTMLAttrValueSingleQuotedInner(l)
}

// lexHTMLAttrValueSingleQuotedInner scans inside a single-quoted value.
func lexHTMLAttrValueSingleQuotedInner(l *Scanner) stateFn {
	for {
		rest := l.input[l.pos:]
		idxDelim := strings.Index(rest, l.Options.LeftDelim)
		idxQuote := strings.IndexByte(rest, '\'')
		switch {
		case idxQuote < 0:
			return l.errorf("unterminated attribute value")
		case idxDelim >= 0 && idxDelim < idxQuote:
			l.pos += Pos(idxDelim)
			if l.pos > l.start {
				l.line += strings.Count(l.input[l.start:l.pos], "\n")
				l.htmlReturnState = lexHTMLAttrValueSingleQuotedInner
				return l.emit(TokenTypeHTMLAttrValue)
			}
			l.htmlReturnState = lexHTMLAttrValueSingleQuotedInner
			return scanLeftDelim
		default:
			l.pos += Pos(idxQuote) + 1
			l.line += strings.Count(l.input[l.start:l.pos], "\n")
			l.htmlReturnState = lexHTMLAfterTagName
			return l.emit(TokenTypeHTMLAttrValue)
		}
	}
}

// lexHTMLAttrValueUnquoted scans an unquoted attribute value.
func lexHTMLAttrValueUnquoted(l *Scanner) stateFn {
	return lexHTMLAttrValueUnquotedImpl(l, false)
}

// lexHTMLAttrValueUnquotedContinue scans an unquoted value after an action
// in it.
func lexHTMLAttrValueUnquotedContinue(l *Scanner) stateFn {
	return lexHTMLAttrValueUnquotedImpl(l, true)
}

func lexHTMLAttrValueUnquotedImpl(l *Scanner, hasContent bool) stateFn {
	for {
		r := l.peek()
		if r == eof || r == '>' || isSpace(r) {
			break
		}
		// Check for template action.
		if strings.HasPrefix(l.input[l.pos:], l.Options.LeftDelim) {
			if l.pos > l.start {
				l.htmlReturnState = lexHTMLAttrValueUnquotedContinue
				return l.emit(TokenTypeHTMLAttrValue)
			}
			l.htmlReturnState = lexHTMLAttrValueUnquotedContinue
			return scanLeftDelim
		}
		l.next()
		hasContent = true
	}
	if l.pos > l.start {
		l.htmlReturnState = lexHTMLAfterTagName
		return l.emit(TokenTypeHTMLAttrValue)
	}
	if hasContent {
		// A template action already produced the value; nothing more to emit.
		return lexHTMLAfterTagName
	}
	return l.errorf("expected attribute value")
}

// lexHTMLCommentBody scans a comment's content after "<!--", up to "-->".
// Actions in it are lexed. "<!-->" and "<!--->" are empty comments that
// close at once (WHATWG 13.2.5.43-44).
func lexHTMLCommentBody(l *Scanner) stateFn {
	rest := l.input[l.pos:]
	if len(rest) > 0 && rest[0] == '>' {
		// <!--> : empty comment, close is just '>'.
		l.next()
		l.htmlReturnState = lexHTMLText
		return l.emit(TokenTypeHTMLCommentClose)
	}
	if len(rest) >= 2 && rest[0] == '-' && rest[1] == '>' {
		// <!---> : empty comment, close is '->'.
		l.pos += 2
		l.htmlReturnState = lexHTMLText
		return l.emit(TokenTypeHTMLCommentClose)
	}

	for {
		rest := l.input[l.pos:]
		idxEnd := strings.Index(rest, "-->")
		idxDelim := strings.Index(rest, l.Options.LeftDelim)

		switch {
		case idxEnd < 0:
			return l.errorf("unclosed HTML comment")
		case idxDelim >= 0 && idxDelim < idxEnd:
			// Template action inside comment.
			l.pos += Pos(idxDelim)
			if l.pos > l.start {
				l.line += strings.Count(l.input[l.start:l.pos], "\n")
				l.htmlReturnState = lexHTMLCommentBody
				return l.emit(TokenTypeHTMLComment)
			}
			l.htmlReturnState = lexHTMLCommentBody
			return scanLeftDelim
		default:
			// Found -->
			l.pos += Pos(idxEnd)
			if l.pos > l.start {
				l.line += strings.Count(l.input[l.start:l.pos], "\n")
				l.htmlReturnState = lexHTMLCommentClose
				return l.emit(TokenTypeHTMLComment)
			}
			// No content before -->; emit close directly.
			l.pos += 3 // len("-->")
			l.htmlReturnState = lexHTMLText
			return l.emit(TokenTypeHTMLCommentClose)
		}
	}
}

// lexHTMLCommentClose emits the '-->' closing marker.
func lexHTMLCommentClose(l *Scanner) stateFn {
	l.pos += 3 // len("-->")
	l.htmlReturnState = lexHTMLText
	return l.emit(TokenTypeHTMLCommentClose)
}

// lexHTMLText scans text up to the next action or tag.
//
// Every template text/template accepts must lex, so markup that is not a
// well-formed tag is text, and the actions in it are still lexed. Among
// such markup:
//
//   - a '<' not followed by an ASCII letter, '/', '!' or '?', as in "1 < 2";
//   - a tag with an action or template comment in its name or in an
//     attribute name, as in <h{{.N}}> and data-{{.K}}="v";
//   - a tag or comment cut by an {{end}}, {{else}} or {{define}} that ends
//     the control structure or definition it began in;
//   - a tag or comment the input ends inside of.
//
// htmlTagAt has the whole rule. The text token that begins a start tag
// read as text carries the tag's name in DegradedTag.
func lexHTMLText(l *Scanner) stateFn {
	rest := l.input[l.pos:]
	// Find the earliest of {{ or <
	idxDelim := strings.Index(rest, l.Options.LeftDelim)
	idxTag, degradedTag := l.nextHTMLTag(rest, idxDelim)
	emitText := func(i Token) stateFn {
		i.DegradedTag = degradedTag
		return l.emitItem(i)
	}

	switch {
	case idxDelim < 0 && idxTag < 0:
		// No more delimiters or tags; consume remaining text.
		l.pos = Pos(len(l.input))
		if l.pos > l.start {
			l.line += strings.Count(l.input[l.start:l.pos], "\n")
			return emitText(l.thisItem(TokenTypeHTMLText))
		}
		return l.emit(TokenTypeEOF)

	case idxDelim >= 0 && (idxTag < 0 || idxDelim <= idxTag):
		// Template delimiter comes first (or at same position as <).
		if idxDelim > 0 {
			l.pos += Pos(idxDelim)
			l.line += strings.Count(l.input[l.start:l.pos], "\n")
			i := l.thisItem(TokenTypeHTMLText)
			l.ignore()
			if len(i.Value) > 0 {
				return emitText(i)
			}
		}
		// Save return state so we resume HTML text after the action.
		l.htmlReturnState = lexHTMLText
		return scanLeftDelim

	default:
		// HTML tag comes first.
		if idxTag > 0 {
			l.pos += Pos(idxTag)
			l.line += strings.Count(l.input[l.start:l.pos], "\n")
			return emitText(l.thisItem(TokenTypeHTMLText))
		}
		// idxTag == 0: we're at a '<', dispatch to tag lexing.
		return lexHTMLTag
	}
}

// nextHTMLTag returns the offset in rest of the first '<' before limit
// (the next left delimiter, or -1 for none) that starts markup the lexer
// lexes as a tag, comment or doctype, or -1. A '<' it passes over is
// text; when the '<' at offset 0 is a start tag read as text, it also
// returns that tag's name.
func (l *Scanner) nextHTMLTag(rest string, limit int) (int, string) {
	end := len(rest)
	if limit >= 0 {
		end = limit
	}
	from := 0
	if l.htmlTextEnd > l.pos {
		from = int(l.htmlTextEnd - l.pos)
	}
	degradedTag := ""
	for from < end {
		k := strings.IndexByte(rest[from:end], '<')
		if k < 0 {
			break
		}
		at := from + k
		isTag, textEnd, name := l.htmlTagAt(int(l.pos) + at)
		if isTag {
			return at, degradedTag
		}
		if name != "" {
			if at > 0 {
				// Emit the text before the degraded start tag first, so
				// that the tag's own text token begins with its '<'.
				return at, degradedTag
			}
			degradedTag = name
		}
		if Pos(textEnd) > l.htmlTextEnd {
			l.htmlTextEnd = Pos(textEnd)
		}
		from = textEnd - int(l.pos)
	}
	return -1, degradedTag
}

// htmlTagAt classifies the '<' at l.input[i]. It reports whether the
// lexer lexes a tag, comment or doctype from there. When it does not,
// textEnd is the offset up to which the input is text, and for a start
// tag read as text name is its tag name.
func (l *Scanner) htmlTagAt(i int) (isTag bool, textEnd int, name string) {
	s := l.input
	j := i + 1
	if j >= len(s) {
		return false, j, ""
	}
	switch c := s[j]; {
	case strings.HasPrefix(s[j:], "!--"):
		return l.htmlCommentAt(j + 3)
	case c == '!' || c == '?':
		// A doctype, processing instruction or bogus comment is lexed
		// whole only when its '>' comes before any template action;
		// otherwise its "<!" or "<?" is text, as in html/template's text
		// state, and the actions after it stay actions.
		end := strings.IndexByte(s[j:], '>')
		if end < 0 {
			return false, j + 1, ""
		}
		if delim := strings.Index(s[j:], l.Options.LeftDelim); delim >= 0 && delim < end {
			return false, j + 1, ""
		}
		return true, 0, ""
	case c == '/':
		j++
		if j >= len(s) || !isASCIIAlpha(rune(s[j])) {
			return false, j, ""
		}
		for j < len(s) && isHTMLTagNameChar(rune(s[j])) {
			j++
		}
		k := j
		for k < len(s) && isSpace(rune(s[k])) {
			k++
		}
		if k < len(s) && s[k] == '>' {
			return true, 0, ""
		}
		return false, l.htmlDegradedTagEnd(j, 0), ""
	case isASCIIAlpha(rune(c)):
		return l.htmlStartTagAt(i)
	default:
		return false, j, ""
	}
}

// htmlCommentAt reports whether the HTML comment whose body begins at
// l.input[j] is lexed as a comment: it must close, and the control
// structures actions in it open must end in it, while an action in it
// must not end one the comment began in. Otherwise the comment's "<!--"
// is text.
func (l *Scanner) htmlCommentAt(j int) (isTag bool, textEnd int, name string) {
	s := l.input
	ld := l.Options.LeftDelim
	if strings.HasPrefix(s[j:], ">") || strings.HasPrefix(s[j:], "->") {
		return true, 0, "" // abruptly closed: <!--> and <!--->
	}
	depth := 0
	for {
		idxEnd := strings.Index(s[j:], "-->")
		idxDelim := strings.Index(s[j:], ld)
		switch {
		case idxEnd < 0:
			return false, j, ""
		case idxDelim < 0 || idxEnd < idxDelim:
			return depth == 0, j + idxEnd + len("-->"), ""
		}
		kind, end := l.htmlActionAt(j + idxDelim)
		if end < 0 {
			return true, 0, "" // the action lexer reports the action
		}
		switch kind {
		case htmlActionDefine:
			return false, j + idxDelim, ""
		case htmlActionOpen:
			depth++
		case htmlActionElse:
			if depth == 0 {
				return false, j + idxDelim, ""
			}
		case htmlActionEnd:
			if depth == 0 {
				return false, j + idxDelim, ""
			}
			depth--
		}
		j = end
	}
}

// htmlStartTagAt reports whether the start tag whose '<' is at l.input[i]
// is well formed. It must agree with what the tag lexers and the parser
// accept. A malformed action in the tag counts as well formed, so that the
// action's own error is reported.
func (l *Scanner) htmlStartTagAt(i int) (isTag bool, textEnd int, name string) {
	s := l.input
	ld := l.Options.LeftDelim
	j := i + 1
	for j < len(s) && isHTMLTagNameChar(rune(s[j])) {
		j++
	}
	name = s[i+1 : j]
	depth := 0 // control structures opened by actions in the tag
	degrade := func(at int) (bool, int, string) {
		return false, l.htmlDegradedTagEnd(at, depth), name
	}
	if strings.HasPrefix(s[j:], ld) {
		// An action against the name is in the name (<h{{.N}}>),
		// unless it opens a list of attributes.
		if !l.htmlActionOpensAttrs(j) {
			return degrade(j)
		}
	}
	skipSpace := func(k int) int {
		for k < len(s) && isSpace(rune(s[k])) {
			k++
		}
		return k
	}
	// action skips the action at k, reporting whether the tag is still
	// well formed after it and where the action ends.
	action := func(k int) (ok bool, end int) {
		kind, end := l.htmlActionAt(k)
		if end < 0 {
			return true, -1
		}
		switch kind {
		case htmlActionComment:
			// The parser accepts no comment in a tag; a comment the
			// lexer does not emit is no part of the tag.
			return !l.Options.EmitComment, end
		case htmlActionDefine:
			return false, end
		case htmlActionOpen:
			depth++
		case htmlActionElse:
			if depth == 0 {
				return false, end
			}
		case htmlActionEnd:
			if depth == 0 {
				return false, end
			}
			depth--
		}
		return true, end
	}
	for {
		j = skipSpace(j)
		switch {
		case j >= len(s):
			return false, len(s), name
		case s[j] == '>' || strings.HasPrefix(s[j:], "/>"):
			if depth != 0 {
				return degrade(j)
			}
			return true, 0, ""
		case s[j] == '/' || s[j] == '=':
			return degrade(j)
		case strings.HasPrefix(s[j:], ld):
			ok, end := action(j)
			if end < 0 {
				return true, 0, ""
			}
			if !ok {
				return degrade(j)
			}
			j = end
			if k := skipSpace(j); k < len(s) && s[k] == '=' {
				return degrade(k)
			}
		default:
			// Attribute name.
			for j < len(s) && !strings.HasPrefix(s[j:], ld) {
				if c := s[j]; c == '=' || c == '>' || c == '/' || isSpace(rune(c)) {
					break
				}
				j++
			}
			if strings.HasPrefix(s[j:], ld) {
				// An action written against the name is in the name,
				// unless it ends the control structure the attribute
				// is in ({{if .X}}hidden{{end}}).
				if kind, _ := l.htmlActionAt(j); kind != htmlActionEnd && kind != htmlActionElse && !l.htmlActionOpensAttrs(j) {
					return degrade(j)
				}
				continue
			}
			k := skipSpace(j)
			if k >= len(s) || s[k] != '=' {
				continue // a boolean attribute
			}
			j = skipSpace(k + 1)
			if j >= len(s) {
				return false, len(s), name
			}
			if q := s[j]; q == '"' || q == '\'' {
				j++
				for {
					idxQuote := strings.IndexByte(s[j:], q)
					idxDelim := strings.Index(s[j:], ld)
					if idxQuote < 0 {
						return false, len(s), name
					}
					if idxDelim < 0 || idxQuote < idxDelim {
						j += idxQuote + 1
						break
					}
					ok, end := action(j + idxDelim)
					if end < 0 {
						return true, 0, ""
					}
					if !ok {
						return degrade(j + idxDelim)
					}
					j = end
				}
				continue
			}
			valueStart := j
			for j < len(s) && s[j] != '>' && !isSpace(rune(s[j])) {
				if !strings.HasPrefix(s[j:], ld) {
					j++
					continue
				}
				ok, end := action(j)
				if end < 0 {
					return true, 0, ""
				}
				if !ok {
					return degrade(j)
				}
				j = end
			}
			if j == valueStart {
				return degrade(j)
			}
		}
	}
}

// htmlDegradedTagEnd returns the offset just past the '>' that closes a
// tag read as text, scanning from j, where the tag's actions have opened
// depth control structures. The tag also ends before an action that ends
// the control structure or definition the tag began in, and at the end of
// the input.
func (l *Scanner) htmlDegradedTagEnd(j, depth int) int {
	s := l.input
	ld := l.Options.LeftDelim
	var quote byte
	afterEquals := false
	for j < len(s) {
		if strings.HasPrefix(s[j:], ld) {
			kind, end := l.htmlActionAt(j)
			if end < 0 {
				return j
			}
			switch kind {
			case htmlActionDefine:
				return j
			case htmlActionOpen:
				depth++
			case htmlActionElse:
				if depth == 0 {
					return j
				}
			case htmlActionEnd:
				if depth == 0 {
					return j
				}
				depth--
			}
			j = end
			continue
		}
		c := s[j]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '>':
			return j + 1
		case (c == '"' || c == '\'') && afterEquals:
			quote = c
		}
		if quote == 0 && !isSpace(rune(c)) {
			afterEquals = c == '='
		}
		j++
	}
	return len(s)
}

// Kinds of template action, as htmlActionAt reports them.
const (
	htmlActionOther   = iota
	htmlActionComment // {{/* ... */}}
	htmlActionOpen    // {{if}}, {{with}}, {{range}}, {{block}}: needs an {{end}}
	htmlActionElse    // {{else}}
	htmlActionEnd     // {{end}}
	htmlActionDefine  // {{define}}
)

// htmlActionOpensAttrs reports whether the action at l.input[i], written
// against a tag or attribute name, opens a control structure whose body
// starts with a space: html/template reads that body as more attributes
// (<li{{if .X}} hidden{{end}}>, formnovalidate{{if .X}} disabled{{end}}),
// not as more of the name.
func (l *Scanner) htmlActionOpensAttrs(i int) bool {
	kind, end := l.htmlActionAt(i)
	if kind != htmlActionOpen || end < 0 || end >= len(l.input) || !isSpace(rune(l.input[end])) {
		return false
	}
	// A right trim marker removes the space.
	body := l.input[i : end-len(l.Options.RightDelim)]
	return len(body) < 2 || !hasRightTrimMarker(body[len(body)-2:])
}

// htmlActionAt classifies the template action whose left delimiter is at
// l.input[i] and returns the offset just past its right delimiter, or -1
// when the action does not end (the action lexer reports that).
func (l *Scanner) htmlActionAt(i int) (kind, end int) {
	s := l.input
	rd := l.Options.RightDelim
	k := i + len(l.Options.LeftDelim)
	if hasLeftTrimMarker(s[k:]) {
		k += int(trimMarkerLen)
	}
	if strings.HasPrefix(s[k:], leftComment) {
		x := strings.Index(s[k+len(leftComment):], rightComment)
		if x < 0 {
			return htmlActionComment, -1
		}
		k += len(leftComment) + x + len(rightComment)
		if hasRightTrimMarker(s[k:]) {
			k += int(trimMarkerLen)
		}
		if !strings.HasPrefix(s[k:], rd) {
			return htmlActionComment, -1
		}
		return htmlActionComment, k + len(rd)
	}
	for k < len(s) && isSpace(rune(s[k])) {
		k++
	}
	w := k
	for w < len(s) && 'a' <= s[w] && s[w] <= 'z' {
		w++
	}
	switch s[k:w] {
	case "if", "with", "range", "block":
		kind = htmlActionOpen
	case "else":
		kind = htmlActionElse
	case "end":
		kind = htmlActionEnd
	case "define":
		kind = htmlActionDefine
	}
	for k < len(s) {
		if strings.HasPrefix(s[k:], rd) {
			return kind, k + len(rd)
		}
		switch q := s[k]; q {
		case '"', '\'', '`':
			k++
			for k < len(s) && s[k] != q {
				if s[k] == '\\' && q != '`' {
					k++
				}
				k++
			}
		}
		k++
	}
	return kind, -1
}

// lexHTMLRawText scans the content of script, style, textarea or title.
// Only an action and the element's own end tag end the text; any other
// '<' is part of it.
func lexHTMLRawText(l *Scanner) stateFn {
	for {
		rest := l.input[l.pos:]
		if len(rest) == 0 {
			// EOF — emit accumulated text.
			if l.pos > l.start {
				l.line += strings.Count(l.input[l.start:l.pos], "\n")
				return l.emit(TokenTypeHTMLText)
			}
			return l.emit(TokenTypeEOF)
		}

		// Find earliest of leftDelim and case-insensitive </tagname.
		idxDelim := strings.Index(rest, l.Options.LeftDelim)

		// Search for the end tag: </tagname followed by a terminator.
		// Per WHATWG 13.1.2.6, the terminator is one of: tab, LF, FF, CR,
		// space, '>', or '/'.
		idxEndTag := -1
		endTagPrefix := "</" + l.htmlRawTag
		// asciiLower keeps byte offsets; strings.ToLower widens bytes
		// that are not UTF-8, and the end tag would be missed.
		restLower := asciiLower(rest)
		searchFrom := 0
		for {
			idx := strings.Index(restLower[searchFrom:], endTagPrefix)
			if idx < 0 {
				break
			}
			idx += searchFrom
			// The end tag is lexed only when space and '>' follow the
			// name (the end tag lexer accepts nothing else); otherwise,
			// as for </scripting>, it is raw text and the search goes on.
			afterTag := idx + len(endTagPrefix)
			for afterTag < len(rest) && isSpace(rune(rest[afterTag])) {
				afterTag++
			}
			if afterTag < len(rest) && rest[afterTag] == '>' {
				idxEndTag = idx
				break
			}
			searchFrom = idx + 1
		}

		// Determine which comes first.
		switch {
		case idxDelim < 0 && idxEndTag < 0:
			// Neither found — consume everything as text.
			l.pos = Pos(len(l.input))
			if l.pos > l.start {
				l.line += strings.Count(l.input[l.start:l.pos], "\n")
				return l.emit(TokenTypeHTMLText)
			}
			return l.emit(TokenTypeEOF)

		case idxDelim >= 0 && (idxEndTag < 0 || idxDelim < idxEndTag):
			// Template delimiter comes first.
			if idxDelim > 0 {
				l.pos += Pos(idxDelim)
				l.line += strings.Count(l.input[l.start:l.pos], "\n")
				i := l.thisItem(TokenTypeHTMLText)
				l.ignore()
				if len(i.Value) > 0 {
					l.htmlReturnState = lexHTMLRawText
					return l.emitItem(i)
				}
			}
			l.htmlReturnState = lexHTMLRawText
			return scanLeftDelim

		default:
			// End tag comes first (or at same position as delimiter).
			if idxEndTag > 0 {
				l.pos += Pos(idxEndTag)
				l.line += strings.Count(l.input[l.start:l.pos], "\n")
				i := l.thisItem(TokenTypeHTMLText)
				if len(i.Value) > 0 {
					// Back up so the end tag tokens are emitted next.
					l.htmlReturnState = lexHTMLTag
					l.htmlRawTag = ""
					return l.emitItem(i)
				}
			}
			// We're at the </tagname — let lexHTMLTag handle the end tag tokens.
			l.htmlRawTag = ""
			return lexHTMLTag
		}
	}
}

// asciiLower returns s with its ASCII letters lowered and every other byte
// as written.
func asciiLower(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { return 'A' <= r && r <= 'Z' }) {
		return s
	}
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

func isASCIIAlpha(r rune) bool {
	return ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z')
}

// isHTMLTagNameChar reports whether r is an ASCII letter, digit or hyphen.
func isHTMLTagNameChar(r rune) bool {
	return isASCIIAlpha(r) || (r >= '0' && r <= '9') || r == '-'
}
