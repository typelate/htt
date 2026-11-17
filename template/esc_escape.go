// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Copied from html/template/escape.go of go1.27.3, with the changes marked below.

package template

import (
	"bytes"
	"fmt"
	"html"
	"maps"
	"regexp"

	"github.com/typelate/htt" // Changed: htt's nodes replace text/template/parse's.
)

// Changed: the escaper records its edits in a plan and never rewrites a tree.
// Its input is the shadow tree of each template: the same source parsed as
// text, where only control structures, actions and calls are not TextNodes.
// Edits are keyed by the instance (the template name mangled with its start
// context) and the node, since one tree runs in many contexts.
//
// Changed: only part of what the escaper computes is used yet. A template
// that needs more takes the old engine; see escaper.unsupported.

// escapeTemplate escapes the named template and every template it calls,
// and returns the plan for running it as the shadow trees in trees.
// Changed: it returns a plan instead of rewriting trees, and errUnsupported
// where the plan cannot yet be used.
func escapeTemplate(trees map[string]*htt.Tree, name string) (*escapePlan, error) {
	e := makeEscaper(trees)
	c, _ := e.escapeTree(context{}, nil, name, 0)
	var err error
	if c.err != nil {
		err, c.err.Name = c.err, name
	} else if c.state != stateText {
		err = &Error{ErrEndContext, nil, name, 0, fmt.Sprintf("ends in a non-text context: %v", c)}
	}
	if err != nil {
		// Changed: html/template reports this at Execute; it is not reported
		// yet, so the old engine runs.
		return nil, fmt.Errorf("%w: %w", errUnsupported, err)
	}
	if e.unsupported != "" {
		return nil, fmt.Errorf("%w: %s", errUnsupported, e.unsupported)
	}
	return e.commit(), nil
}

// Changed: evalArgs and funcMap are in esc_plan.go.

// escaper collects type inferences about templates and changes needed to make
// templates injection safe.
type escaper struct {
	// trees is the shadow tree of each template, by name.
	// Changed: replaces ns, the name space.
	trees map[string]*htt.Tree
	// instance is the mangled name of the template being escaped. Changed: new.
	instance string
	// output[templateName] is the output context for a templateName that
	// has been mangled to include its input context.
	output map[string]context
	// called[templateName] is a set of called mangled template names.
	called map[string]bool
	// actionNodeEdits are the accumulated edits to apply during commit.
	// Such edits are not applied immediately in case a template set
	// executes a given template in different escaping contexts.
	// Changed: keyed by instance and node; derived and the template node
	// edits are not needed yet.
	actionNodeEdits map[planKey][]string
	// textNodeEdits are the text that replaces a text node, by instance and
	// node, for the escaper's text the escaper rewrote. Changed: new.
	textNodeEdits map[planKey][]byte
	// rangeContext holds context about the current range loop.
	rangeContext *rangeContext
	// unsupported says why the plan cannot be used yet, or is empty. Changed: new.
	unsupported string
}

// rangeContext holds information about the current range loop.
type rangeContext struct {
	outer     *rangeContext // outer loop
	breaks    []context     // context at each break action
	continues []context     // context at each continue action
}

// makeEscaper creates a blank escaper for the given set.
func makeEscaper(trees map[string]*htt.Tree) escaper {
	return escaper{
		trees:           trees,
		output:          map[string]context{},
		called:          map[string]bool{},
		actionNodeEdits: map[planKey][]string{},
		textNodeEdits:   map[planKey][]byte{},
	}
}

// unsupportedBecause records that the plan cannot be used yet. The first
// reason is kept. Changed: new.
func (e *escaper) unsupportedBecause(reason string) {
	if e.unsupported == "" {
		e.unsupported = reason
	}
}

// filterFailsafe is an innocuous word that is emitted in place of unsafe values
// by sanitizer functions. It is not a keyword in any programming language,
// contains no special characters, is not empty, and when it appears in output
// it is distinct enough that a developer can find the source of the problem
// via a search engine.
const filterFailsafe = "ZgotmplZ"

// escape escapes a template node.
// Changed: a TextNode is escaped by escapeList, which knows its trim markers;
// a DefineNode writes nothing and a BlockNode is a call.
func (e *escaper) escape(c context, n htt.Node) context {
	switch n := n.(type) {
	case *htt.ActionNode:
		return e.escapeAction(c, n)
	case *htt.BlockNode:
		return e.escapeTemplate(c, n.Call())
	case *htt.BreakNode:
		c.n = n
		e.rangeContext.breaks = append(e.rangeContext.breaks, c)
		return context{state: stateDead}
	case *htt.CommentNode:
		return c
	case *htt.ContinueNode:
		c.n = n
		e.rangeContext.continues = append(e.rangeContext.continues, c)
		return context{state: stateDead}
	case *htt.DefineNode:
		return c
	case *htt.IfNode:
		return e.escapeBranch(c, n, &n.BranchNode, "if")
	case *htt.ListNode:
		return e.escapeList(c, n, false, false)
	case *htt.RangeNode:
		return e.escapeBranch(c, n, &n.BranchNode, "range")
	case *htt.TemplateNode:
		return e.escapeTemplate(c, n)
	case *htt.WithNode:
		return e.escapeBranch(c, n, &n.BranchNode, "with")
	}
	panic("escaping " + n.String() + " is unimplemented")
}

// escapeAction escapes an action template node.
func (e *escaper) escapeAction(c context, n *htt.ActionNode) context {
	if len(n.Pipe.Decl) != 0 {
		// A local variable assignment, not an interpolation.
		return c
	}
	c = nudge(c)
	// Check for disallowed use of predefined escapers in the pipeline.
	for pos, idNode := range n.Pipe.Commands {
		node, ok := idNode.Args[0].(*htt.IdentifierNode)
		if !ok {
			// A predefined escaper "esc" will never be found as an identifier in a
			// Chain or Field node, since:
			// - "esc.x ..." is invalid, since predefined escapers return strings, and
			//   strings do not have methods, keys or fields.
			// - "... .esc" is invalid, since predefined escapers are global functions,
			//   not methods or fields of any types.
			// Therefore, it is safe to ignore these two node types.
			continue
		}
		ident := node.Ident
		if _, ok := predefinedEscapers[ident]; ok {
			if pos < len(n.Pipe.Commands)-1 ||
				c.state == stateAttr && c.delim == delimSpaceOrTagEnd && ident == "html" {
				return context{
					state: stateError,
					err:   errorf(ErrPredefinedEscaper, n, n.Line, "predefined escaper %q disallowed in template", ident),
				}
			}
		}
	}
	s := make([]string, 0, 3)
	switch c.state {
	case stateError:
		return c
	case stateURL, stateCSSDqStr, stateCSSSqStr, stateCSSDqURL, stateCSSSqURL, stateCSSURL:
		switch c.urlPart {
		case urlPartNone:
			s = append(s, "_html_template_urlfilter")
			fallthrough
		case urlPartPreQuery:
			switch c.state {
			case stateCSSDqStr, stateCSSSqStr:
				s = append(s, "_html_template_cssescaper")
			default:
				s = append(s, "_html_template_urlnormalizer")
			}
		case urlPartQueryOrFrag:
			s = append(s, "_html_template_urlescaper")
		case urlPartUnknown:
			return context{
				state: stateError,
				err:   errorf(ErrAmbigContext, n, n.Line, "%s appears in an ambiguous context within a URL", n),
			}
		default:
			panic(c.urlPart.String())
		}
	case stateMetaContent:
		// Handled below in delim check.
	case stateMetaContentURL:
		// Changed: html/template's htmlmetacontenturlescape GODEBUG is not read;
		// the default, filtering the URL, always applies.
		s = append(s, "_html_template_urlfilter")
	case stateJS:
		s = append(s, "_html_template_jsvalescaper")
		// A slash after a value starts a div operator.
		c.jsCtx = jsCtxDivOp
	case stateJSDqStr, stateJSSqStr:
		s = append(s, "_html_template_jsstrescaper")
	case stateJSTmplLit:
		s = append(s, "_html_template_jstmpllitescaper")
	case stateJSRegexp:
		s = append(s, "_html_template_jsregexpescaper")
	case stateCSS:
		s = append(s, "_html_template_cssvaluefilter")
	case stateText:
		s = append(s, "_html_template_htmlescaper")
	case stateRCDATA:
		s = append(s, "_html_template_rcdataescaper")
	case stateAttr:
		// Handled below in delim check.
	case stateAttrName, stateTag:
		c.state = stateAttrName
		s = append(s, "_html_template_htmlnamefilter")
	case stateSrcset:
		s = append(s, "_html_template_srcsetescaper")
	default:
		if isComment(c.state) {
			s = append(s, "_html_template_commentescaper")
		} else {
			panic("unexpected state " + c.state.String())
		}
	}
	switch c.delim {
	case delimNone:
		// No extra-escaping needed for raw text content.
	case delimSpaceOrTagEnd:
		s = append(s, "_html_template_nospaceescaper")
	default:
		s = append(s, "_html_template_attrescaper")
	}
	e.editActionNode(planKey{e.instance, n}, s)
	return c
}

// Changed: ensurePipelineContains is planPipeline in esc_plan.go, with
// equivEscapers, escFnsEq, normalizeEscFn, redundantFuncs, appendCmd and
// newIdentCmd.

// predefinedEscapers contains template predefined escapers that are equivalent
// to some contextual escapers. Keep in sync with equivEscapers.
var predefinedEscapers = map[string]bool{
	"html":     true,
	"urlquery": true,
}

// nudge returns the context that would result from following empty string
// transitions from the input context.
// For example, parsing:
//
//	`<a href=`
//
// will end in context{stateBeforeValue, attrURL}, but parsing one extra rune:
//
//	`<a href=x`
//
// will end in context{stateURL, delimSpaceOrTagEnd, ...}.
// There are two transitions that happen when the 'x' is seen:
// (1) Transition from a before-value state to a start-of-value state without
//
//	consuming any character.
//
// (2) Consume 'x' and transition past the first value character.
// In this case, nudging produces the context after (1) happens.
func nudge(c context) context {
	switch c.state {
	case stateTag:
		// In `<foo {{.}}`, the action should emit an attribute.
		c.state = stateAttrName
	case stateBeforeValue:
		// In `<foo bar={{.}}`, the action is an undelimited value.
		c.state, c.delim, c.attr = attrStartStates[c.attr], delimSpaceOrTagEnd, attrNone
	case stateAfterName:
		// In `<foo bar {{.}}`, the action is an attribute name.
		c.state, c.attr = stateAttrName, attrNone
	}
	return c
}

// join joins the two contexts of a branch template node. The result is an
// error context if either of the input contexts are error contexts, or if the
// input contexts differ.
func join(a, b context, node htt.Node, nodeName string) context {
	if a.state == stateError {
		return a
	}
	if b.state == stateError {
		return b
	}
	if a.state == stateDead {
		return b
	}
	if b.state == stateDead {
		return a
	}
	if a.eq(b) {
		return a
	}

	c := a
	c.urlPart = b.urlPart
	if c.eq(b) {
		// The contexts differ only by urlPart.
		c.urlPart = urlPartUnknown
		return c
	}

	c = a
	c.jsCtx = b.jsCtx
	if c.eq(b) {
		// The contexts differ only by jsCtx.
		c.jsCtx = jsCtxUnknown
		return c
	}

	// Allow a nudged context to join with an unnudged one.
	// This means that
	//   <p title={{if .C}}{{.}}{{end}}
	// ends in an unquoted value state even though the else branch
	// ends in stateBeforeValue.
	if c, d := nudge(a), nudge(b); !(c.eq(a) && d.eq(b)) {
		if e := join(c, d, node, nodeName); e.state != stateError {
			return e
		}
	}

	return context{
		state: stateError,
		err:   errorf(ErrBranchEnd, node, 0, "{{%s}} branches end in different contexts: %v, %v", nodeName, a, b),
	}
}

// escapeBranch escapes a branch template node: "if", "range" and "with".
// Changed: the lists are escaped with the trim markers the executor applies,
// and node is the if, range or with that n belongs to, since a BranchNode
// is not a Node.
func (e *escaper) escapeBranch(c context, node htt.Node, n *htt.BranchNode, nodeName string) context {
	if nodeName == "range" {
		e.rangeContext = &rangeContext{outer: e.rangeContext}
	}
	c0 := e.escapeList(c.clone(), n.List, n.StartTrimRight, n.BodyTrimLast())
	if nodeName == "range" {
		if c0.state != stateError {
			c0 = joinRange(c0, e.rangeContext)
		}
		e.rangeContext = e.rangeContext.outer
		if c0.state == stateError {
			return c0
		}

		// The "true" branch of a "range" node can execute multiple times.
		// We check that executing n.List once results in the same context
		// as executing n.List twice.
		e.rangeContext = &rangeContext{outer: e.rangeContext}
		c1, _ := e.escapeListConditionally(c0, n.List, n.StartTrimRight, n.BodyTrimLast(), nil)
		c0 = join(c0, c1, node, nodeName)
		if c0.state == stateError {
			e.rangeContext = e.rangeContext.outer
			// Make clear that this is a problem on loop re-entry
			// since developers tend to overlook that branch when
			// debugging templates.
			c0.err.Line = n.Line
			c0.err.Description = "on range loop re-entry: " + c0.err.Description
			return c0
		}
		c0 = joinRange(c0, e.rangeContext)
		e.rangeContext = e.rangeContext.outer
		if c0.state == stateError {
			return c0
		}
	}
	c1 := e.escapeList(c.clone(), n.ElseList, n.ElseTrimRight, n.EndTrimLeft)
	return join(c0, c1, node, nodeName)
}

// joinRange merges the contexts at break and continue statements into the
// context of the loop body.
func joinRange(c0 context, rc *rangeContext) context {
	// Merge contexts at break and continue statements into overall body context.
	// In theory we could treat breaks differently from continues, but for now it is
	// enough to treat them both as going back to the start of the loop (which may then stop).
	for _, c := range rc.breaks {
		c0 = join(c0, c, c.n, "range")
		if c0.state == stateError {
			c0.err.Line = c.n.(*htt.BreakNode).Line
			c0.err.Description = "at range loop break: " + c0.err.Description
			return c0
		}
	}
	for _, c := range rc.continues {
		c0 = join(c0, c, c.n, "range")
		if c0.state == stateError {
			c0.err.Line = c.n.(*htt.ContinueNode).Line
			c0.err.Description = "at range loop continue: " + c0.err.Description
			return c0
		}
	}
	return c0
}

// escapeList escapes a list template node.
// Changed: trimFirst and trimLast say whether the first and last text node
// lose their leading and trailing space, as the executor decides it.
func (e *escaper) escapeList(c context, n *htt.ListNode, trimFirst, trimLast bool) context {
	if n == nil {
		return c
	}
	for i, m := range n.Nodes {
		if tn, ok := m.(*htt.TextNode); ok {
			c = e.escapeText(c, tn, htt.TrimmedText(n.Nodes, i, trimFirst, trimLast))
		} else {
			c = e.escape(c, m)
		}
		if c.state == stateDead {
			break
		}
	}
	return c
}

// escapeListConditionally escapes a list node but only preserves edits and
// inferences in e if the inferences and output context satisfy filter.
// It returns the best guess at an output context, and the result of the filter
// which is the same as whether e was updated.
func (e *escaper) escapeListConditionally(c context, n *htt.ListNode, trimFirst, trimLast bool, filter func(*escaper, context) bool) (context, bool) {
	e1 := makeEscaper(e.trees)
	e1.instance = e.instance
	e1.rangeContext = e.rangeContext
	// Make type inferences available to f.
	maps.Copy(e1.output, e.output)
	c = e1.escapeList(c, n, trimFirst, trimLast)
	ok := filter != nil && filter(&e1, c)
	if ok {
		// Copy inferences and edits from e1 back into e.
		maps.Copy(e.output, e1.output)
		maps.Copy(e.called, e1.called)
		for k, v := range e1.actionNodeEdits {
			e.editActionNode(k, v)
		}
		for k, v := range e1.textNodeEdits {
			e.editTextNode(k, v)
		}
		if e1.unsupported != "" {
			e.unsupportedBecause(e1.unsupported)
		}
	}
	return c, ok
}

// escapeTemplate escapes a {{template}} call node.
func (e *escaper) escapeTemplate(c context, n *htt.TemplateNode) context {
	c, name := e.escapeTree(c, n, n.Name, n.Line)
	if name != n.Name {
		// UNSUPPORTED: a call outside element text runs a callee as another
		// instance, which Plan.Call names.
		e.unsupportedBecause("a template is called outside element text")
	}
	return c
}

// escapeTree escapes the named template starting in the given context as
// necessary and returns its output context.
// Changed: the template is not cloned for a context other than the default;
// its edits are keyed by the mangled name.
func (e *escaper) escapeTree(c context, node htt.Node, name string, line int) (context, string) {
	// Mangle the template name with the input context to produce a reliable
	// identifier.
	dname := c.mangle(name)
	e.called[dname] = true
	if out, ok := e.output[dname]; ok {
		// Already escaped.
		return out, dname
	}
	t := e.trees[name]
	if t == nil {
		// UNSUPPORTED: a template with no shadow tree. html/template tells
		// a template that is empty from one never mentioned; this does not.
		e.unsupportedBecause("a template has no shadow tree")
		return context{
			state: stateError,
			err:   errorf(ErrNoSuchTemplate, node, line, "no such template %q", name),
		}, dname
	}
	return e.computeOutCtx(c, dname, t), dname
}

// computeOutCtx takes a template and its start context and computes the output
// context while storing any inferences in e.
func (e *escaper) computeOutCtx(c context, dname string, t *htt.Tree) context {
	// Propagate context over the body.
	c1, ok := e.escapeTemplateBody(c, dname, t)
	if !ok {
		// Look for a fixed point by assuming c1 as the output context.
		if c2, ok2 := e.escapeTemplateBody(c1, dname, t); ok2 {
			c1, ok = c2, true
		}
		// Use c1 as the error context if neither assumption worked.
	}
	if !ok && c1.state != stateError {
		return context{
			state: stateError,
			err:   errorf(ErrOutputContext, t.Root, 0, "cannot compute output context for template %s", dname),
		}
	}
	return c1
}

// escapeTemplateBody escapes the given template assuming the given output
// context, and returns the best guess at the output context and whether the
// assumption was correct.
func (e *escaper) escapeTemplateBody(c context, dname string, t *htt.Tree) (context, bool) {
	filter := func(e1 *escaper, c1 context) bool {
		if c1.state == stateError {
			// Do not update the input escaper, e.
			return false
		}
		if !e1.called[dname] {
			// If t is not recursively called, then c1 is an
			// accurate output context.
			return true
		}
		// c1 is accurate if it matches our assumed output context.
		return c.eq(c1)
	}
	// We need to assume an output context so that recursive template calls
	// take the fast path out of escapeTree instead of infinitely recurring.
	// Naively assuming that the input context is the same as the output
	// works >90% of the time.
	e.output[dname] = c
	outer := e.instance
	e.instance = dname
	defer func() { e.instance = outer }()
	return e.escapeListConditionally(c, t.Root, t.RootTrimFirst, t.RootTrimLast, filter)
}

// delimEnds maps each delim to a string of characters that terminate it.
var delimEnds = [...]string{
	delimDoubleQuote: `"`,
	delimSingleQuote: "'",
	// Determined empirically by running the below in various browsers.
	// var div = document.createElement("DIV");
	// for (var i = 0; i < 0x10000; ++i) {
	//   div.innerHTML = "<span title=x" + String.fromCharCode(i) + "-bar>";
	//   if (div.getElementsByTagName("SPAN")[0].title.indexOf("bar") < 0)
	//     document.write("<p>U+" + i.toString(16));
	// }
	delimSpaceOrTagEnd: " \t\n\f\r>",
}

var (
	// Per WHATWG HTML specification, section 4.12.1.3, there are extremely
	// complicated rules for how to handle the set of opening tags <!--,
	// <script, and </script when they appear in JS literals (i.e. strings,
	// regexs, and comments). The specification suggests a simple solution,
	// rather than implementing the arcane ABNF, which involves simply escaping
	// the opening bracket with \x3C. We use the below regex for this, since it
	// makes doing the case-insensitive find-replace much simpler.
	specialScriptTagRE          = regexp.MustCompile("(?i)<(script|/script|!--)")
	specialScriptTagReplacement = []byte("\\x3C$1")
)

func containsSpecialScriptTag(s []byte) bool {
	return specialScriptTagRE.Match(s)
}

func escapeSpecialScriptTags(s []byte) []byte {
	return specialScriptTagRE.ReplaceAll(s, specialScriptTagReplacement)
}

var doctypeBytes = []byte("<!DOCTYPE")

// escapeText escapes a text template node.
// Changed: text is the node's text with its trim markers applied, since the
// parser does not trim it; n keys the edit that replaces its text.
func (e *escaper) escapeText(c context, n *htt.TextNode, text []byte) context {
	s, written, i, b := text, 0, 0, new(bytes.Buffer)
	for i != len(s) {
		c1, nread := contextAfterText(c, s[i:])
		i1 := i + nread
		if c.state == stateText || c.state == stateRCDATA {
			end := i1
			if c1.state != c.state {
				for j := end - 1; j >= i; j-- {
					if s[j] == '<' {
						end = j
						break
					}
				}
			}
			for j := i; j < end; j++ {
				if s[j] == '<' && !bytes.HasPrefix(bytes.ToUpper(s[j:]), doctypeBytes) {
					b.Write(s[written:j])
					b.WriteString("&lt;")
					written = j + 1
				}
			}
		} else if isComment(c.state) && c.delim == delimNone {
			switch c.state {
			case stateJSBlockCmt:
				// https://es5.github.io/#x7.4:
				// "Comments behave like white space and are
				// discarded except that, if a MultiLineComment
				// contains a line terminator character, then
				// the entire comment is considered to be a
				// LineTerminator for purposes of parsing by
				// the syntactic grammar."
				if bytes.ContainsAny(s[written:i1], "\n\r  ") {
					b.WriteByte('\n')
				} else {
					b.WriteByte(' ')
				}
			case stateCSSBlockCmt:
				b.WriteByte(' ')
			}
			written = i1
		}
		if c.state != c1.state && isComment(c1.state) && c1.delim == delimNone {
			// Preserve the portion between written and the comment start.
			cs := i1 - 2
			if c1.state == stateHTMLCmt || c1.state == stateJSHTMLOpenCmt {
				// "<!--" instead of "/*" or "//"
				cs -= 2
			} else if c1.state == stateJSHTMLCloseCmt {
				// "-->" instead of "/*" or "//"
				cs -= 1
			}
			b.Write(s[written:cs])
			written = i1
		}
		if isInScriptLiteral(c.state) && containsSpecialScriptTag(s[i:i1]) {
			b.Write(s[written:i])
			b.Write(escapeSpecialScriptTags(s[i:i1]))
			written = i1
		}
		if i == i1 && c.state == c1.state {
			panic(fmt.Sprintf("infinite loop from %v to %v on %q..%q", c, c1, s[:i], s[i:]))
		}
		c, i = c1, i1
	}

	if written != 0 && c.state != stateError {
		if !isComment(c.state) || c.delim != delimNone {
			b.Write(text[written:])
		}
		// The executor writes this in place of n, so it is not trimmed again.
		e.editTextNode(planKey{e.instance, n}, b.Bytes())
	}
	return c
}

// contextAfterText starts in context c, consumes some tokens from the front of
// s, then returns the context after those tokens and the unprocessed suffix.
func contextAfterText(c context, s []byte) (context, int) {
	if c.delim == delimNone {
		c1, i := tSpecialTagEnd(c, s)
		if i == 0 {
			// A special end tag (`</script>`) has been seen and
			// all content preceding it has been consumed.
			return c1, 0
		}
		// Consider all content up to any end tag.
		return transitionFunc[c.state](c, s[:i])
	}

	// We are at the beginning of an attribute value.

	i := bytes.IndexAny(s, delimEnds[c.delim])
	if i == -1 {
		i = len(s)
	}
	if c.delim == delimSpaceOrTagEnd {
		// https://www.w3.org/TR/html5/syntax.html#attribute-value-(unquoted)-state
		// lists the runes below as error characters.
		// Error out because HTML parsers may differ on whether
		// "<a id= onclick=f("     ends inside id's or onclick's value,
		// "<a class=`foo "        ends inside a value,
		// "<a style=font:'Arial'" needs open-quote fixup.
		// IE treats '`' as a quotation character.
		if j := bytes.IndexAny(s[:i], "\"'<=`"); j >= 0 {
			return context{
				state: stateError,
				err:   errorf(ErrBadHTML, nil, 0, "%q in unquoted attr: %q", s[j:j+1], s[:i]),
			}, len(s)
		}
	}
	if i == len(s) {
		// Remain inside the attribute.
		// Decode the value so non-HTML rules can easily handle
		//     <button onclick="alert(&quot;Hi!&quot;)">
		// without having to entity decode token boundaries.
		for u := []byte(html.UnescapeString(string(s))); len(u) != 0; {
			c1, i1 := transitionFunc[c.state](c, u)
			c, u = c1, u[i1:]
		}
		return c, len(s)
	}

	element := c.element

	// If this is a non-JS "type" attribute inside "script" tag, do not treat the contents as JS.
	if c.state == stateAttr && c.element == elementScript && c.attr == attrScriptType && !isJSType(string(s[:i])) {
		element = elementNone
	}

	if c.delim != delimSpaceOrTagEnd {
		// Consume any quote.
		i++
	}
	// On exiting an attribute, we discard all state information
	// except the state and element.
	return context{state: stateTag, element: element}, i
}

// editTextNode records a change to a text node for later commit.
// Changed: the key names the instance as well as the node.
func (e *escaper) editTextNode(k planKey, text []byte) {
	if _, ok := e.textNodeEdits[k]; ok {
		panic(fmt.Sprintf("node %s shared between templates", k.node))
	}
	e.textNodeEdits[k] = text
}

// editActionNode records a change to an action pipeline for later commit.
// Changed: the key names the instance as well as the node.
func (e *escaper) editActionNode(k planKey, cmds []string) {
	if _, ok := e.actionNodeEdits[k]; ok {
		panic(fmt.Sprintf("node %s shared between templates", k.node))
	}
	e.actionNodeEdits[k] = cmds
}

// Changed: commit is in esc_plan.go; template and arbitraryTemplate are not
// needed, and the forwarding functions are in escape.go.
