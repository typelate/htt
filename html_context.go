package htt

import (
	"slices"
	"strings"
)

// HTMLContext is the escaping context of an action.
type HTMLContext struct {
	State   string // "text", "attr", "comment"
	Element string // element name (e.g., "script", "style", "p") or ""
	Attr    string // attribute name (e.g., "href", "onclick", "class") or ""
	// JSContext is set in script content: "expr", "str" in a quoted
	// string, "tpl" in a template literal, "regex" or "comment". A ${...}
	// in a template literal is "expr".
	JSContext string
}

// AnnotateHTMLContexts sets HTMLContext on each ActionNode and TemplateNode
// under root from its place in the tree.
func AnnotateHTMLContexts(root Node) {
	annotateContexts(root, "", "", false, "")
}

func annotateContexts(n Node, parentElement, parentAttr string, inComment bool, jsCtx string) {
	if n == nil {
		return
	}
	switch n := n.(type) {
	case *ListNode:
		if n != nil {
			for _, child := range n.Nodes {
				annotateContexts(child, parentElement, parentAttr, inComment, jsCtx)
			}
		}
	case *HTMLDocumentFragmentNode:
		for _, child := range n.Children {
			annotateContexts(child, "", "", false, "")
		}
	case *HTMLElementNode:
		for _, attr := range n.Attr {
			annotateContexts(attr, n.TagName, "", false, "")
		}
		if strings.EqualFold(n.TagName, "script") {
			annotateScriptChildren(n.Children)
			return
		}
		for _, child := range n.Children {
			annotateContexts(child, n.TagName, "", false, "")
		}
	case *HTMLAttributeNode:
		attrName := ""
		if len(n.Key) > 0 {
			if tn, ok := n.Key[0].(*TextNode); ok {
				attrName = strings.ToLower(string(tn.Text))
			}
		}
		for _, v := range n.Value {
			annotateContexts(v, parentElement, attrName, false, "")
		}
	case *HTMLCommentNode:
		for _, child := range n.Children {
			annotateContexts(child, "", "", true, "")
		}
	case *ActionNode:
		if inComment {
			n.HTMLContext = &HTMLContext{State: "comment"}
		} else {
			state := "text"
			if parentAttr != "" {
				state = "attr"
			}
			n.HTMLContext = &HTMLContext{
				State:     state,
				Element:   strings.ToLower(parentElement),
				Attr:      parentAttr,
				JSContext: jsCtx,
			}
		}
	case *TemplateNode:
		if inComment {
			n.HTMLContext = &HTMLContext{State: "comment"}
		} else {
			state := "text"
			if parentAttr != "" {
				state = "attr"
			}
			n.HTMLContext = &HTMLContext{
				State:     state,
				Element:   strings.ToLower(parentElement),
				Attr:      parentAttr,
				JSContext: jsCtx,
			}
		}
	case *IfNode:
		annotateBranch(&n.BranchNode, parentElement, parentAttr, inComment, jsCtx)
	case *RangeNode:
		annotateBranch(&n.BranchNode, parentElement, parentAttr, inComment, jsCtx)
	case *WithNode:
		annotateBranch(&n.BranchNode, parentElement, parentAttr, inComment, jsCtx)
	case *DefineNode:
		if n.Body != nil {
			for _, child := range n.Body.Nodes {
				annotateContexts(child, parentElement, parentAttr, inComment, jsCtx)
			}
		}
	case *BlockNode:
		if n.Body != nil {
			for _, child := range n.Body.Nodes {
				annotateContexts(child, parentElement, parentAttr, inComment, jsCtx)
			}
		}
	}
}

func annotateBranch(n *BranchNode, parentElement, parentAttr string, inComment bool, jsCtx string) {
	if n.List != nil {
		for _, child := range n.List.Nodes {
			annotateContexts(child, parentElement, parentAttr, inComment, jsCtx)
		}
	}
	if n.ElseList != nil {
		for _, child := range n.ElseList.Nodes {
			annotateContexts(child, parentElement, parentAttr, inComment, jsCtx)
		}
	}
}

// annotateScriptChildren scans a script element's text as JavaScript to
// find the context of each action. It tracks only the delimiters of
// strings, template literals and comments, and the braces of ${...}.
// Where it cannot tell, it reports "expr", which gets the stronger escape.
func annotateScriptChildren(children []Node) {
	state := scriptScanState{ctx: "expr"}
	for _, child := range children {
		state = annotateScriptNode(child, state)
	}
}

type scriptScanState struct {
	ctx   string // "expr", "str", "tpl", "regex", "lcomment", "bcomment"
	quote byte   // for "str": '\'' or '"'
	// braceDepth counts the unmatched '{' in each enclosing ${...},
	// innermost last. A '}' that takes a count below zero closes that
	// interpolation.
	braceDepth []int
}

// clone returns a copy with its own braceDepth, so that scanning a branch
// body does not write to the enclosing scan.
func (s scriptScanState) clone() scriptScanState {
	s.braceDepth = slices.Clone(s.braceDepth)
	return s
}

func annotateScriptNode(n Node, state scriptScanState) scriptScanState {
	switch n := n.(type) {
	case *TextNode:
		return scanScriptText(string(n.Text), state)
	case *ActionNode:
		ctx := state.ctx
		if ctx == "lcomment" || ctx == "bcomment" {
			ctx = "comment"
		}
		n.HTMLContext = &HTMLContext{
			State:     "text",
			Element:   "script",
			JSContext: ctx,
		}
		return state
	case *TemplateNode:
		ctx := state.ctx
		if ctx == "lcomment" || ctx == "bcomment" {
			ctx = "comment"
		}
		n.HTMLContext = &HTMLContext{
			State:     "text",
			Element:   "script",
			JSContext: ctx,
		}
		return state
	case *IfNode:
		// A branch may open a string that its else closes, so each is
		// scanned from the state at entry, which is also taken for the
		// state after.
		annotateScriptBranch(&n.BranchNode, state)
		return state
	case *RangeNode:
		annotateScriptBranch(&n.BranchNode, state)
		return state
	case *WithNode:
		annotateScriptBranch(&n.BranchNode, state)
		return state
	}
	return state
}

func annotateScriptBranch(b *BranchNode, state scriptScanState) {
	if b.List != nil {
		s := state.clone()
		for _, child := range b.List.Nodes {
			s = annotateScriptNode(child, s)
		}
	}
	if b.ElseList != nil {
		s := state.clone()
		for _, child := range b.ElseList.Nodes {
			s = annotateScriptNode(child, s)
		}
	}
}

func scanScriptText(s string, state scriptScanState) scriptScanState {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch state.ctx {
		case "expr", "regex":
			switch c {
			case '\'', '"':
				state.ctx = "str"
				state.quote = c
			case '`':
				state.ctx = "tpl"
			case '/':
				if i+1 < len(s) {
					switch s[i+1] {
					case '/':
						state.ctx = "lcomment"
						i++
						continue
					case '*':
						state.ctx = "bcomment"
						i++
						continue
					}
				}
				// Telling a regex from a division takes JS tokens;
				// "expr" is the safer reading.
			case '{':
				if len(state.braceDepth) > 0 {
					state.braceDepth[len(state.braceDepth)-1]++
				}
			case '}':
				if len(state.braceDepth) > 0 {
					last := len(state.braceDepth) - 1
					state.braceDepth[last]--
					if state.braceDepth[last] < 0 {
						state.braceDepth = state.braceDepth[:last]
						state.ctx = "tpl"
					}
				}
			}
		case "str":
			switch c {
			case '\\':
				if i+1 < len(s) {
					i++
				}
			case state.quote:
				state.ctx = "expr"
				state.quote = 0
			case '\n':
				// An unterminated string: back to the stronger escape.
				state.ctx = "expr"
				state.quote = 0
			}
		case "tpl":
			switch c {
			case '\\':
				if i+1 < len(s) {
					i++
				}
			case '$':
				// "${" opens an interpolation, where code runs.
				if i+1 < len(s) && s[i+1] == '{' {
					state.braceDepth = append(state.braceDepth, 0)
					state.ctx = "expr"
					i++
				}
			case '`':
				state.ctx = "expr"
			}
		case "lcomment":
			if c == '\n' {
				state.ctx = "expr"
			}
		case "bcomment":
			if c == '*' && i+1 < len(s) && s[i+1] == '/' {
				state.ctx = "expr"
				i++
			}
		}
	}
	return state
}
