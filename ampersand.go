package htt

import (
	"bytes"
	"strings"

	"github.com/typelate/htt/lex"
)

// checkUnescapedAmpersands records a DiagUnescapedAmpersand for each
// decoded text that ends with an unfinished character reference and is
// followed by an action. The browser reads the reference and the action's
// value as one reference, so the value can complete it.
func (t *Tree) checkUnescapedAmpersands(n Node) {
	t.checkAmpersandNode(n, true)
}

// checkAmpersandNode walks n. decoded says whether a browser decodes the
// character references in n's text.
func (t *Tree) checkAmpersandNode(n Node, decoded bool) {
	switch n := n.(type) {
	case *ListNode:
		if n != nil {
			t.checkAmpersandList(n.Nodes, decoded)
		}
	case *HTMLDocumentFragmentNode:
		t.checkAmpersandList(n.Children, decoded)
	case *HTMLElementNode:
		t.checkAmpersandList(n.Attr, true)
		t.checkAmpersandList(n.Children, !rawTextNotDecoded[strings.ToLower(n.TagName)])
	case *HTMLAttributeNode:
		t.checkAmpersandList(n.Value, true)
	case *IfNode:
		t.checkAmpersandBranch(&n.BranchNode, decoded)
	case *RangeNode:
		t.checkAmpersandBranch(&n.BranchNode, decoded)
	case *WithNode:
		t.checkAmpersandBranch(&n.BranchNode, decoded)
	case *DefineNode:
		t.checkAmpersandNode(n.Body, decoded)
	case *BlockNode:
		t.checkAmpersandNode(n.Body, decoded)
	}
}

// rawTextNotDecoded is the set of raw text elements whose text a browser
// does not decode.
var rawTextNotDecoded = map[string]bool{"script": true, "style": true}

func (t *Tree) checkAmpersandBranch(b *BranchNode, decoded bool) {
	t.checkAmpersandNode(b.List, decoded)
	t.checkAmpersandNode(b.ElseList, decoded)
}

// checkAmpersandList checks each text in nodes against the node after it.
func (t *Tree) checkAmpersandList(nodes []Node, decoded bool) {
	for i, n := range nodes {
		text, ok := n.(*TextNode)
		if !ok {
			t.checkAmpersandNode(n, decoded)
			continue
		}
		if decoded {
			t.checkAmpersandText(text, nodes, i)
		}
	}
}

// checkAmpersandText records a warning when text, as the executor writes
// it, ends with an unfinished reference and an action writes next.
func (t *Tree) checkAmpersandText(text *TextNode, nodes []Node, i int) {
	written := text.Text
	if i+1 < len(nodes) && TrimsLeft(nodes[i+1]) {
		written = bytes.TrimRight(written, trimCutset)
	}
	amp := unfinishedReference(written)
	if amp < 0 || !writesNext(nodes, i) {
		return
	}
	t.diag(text.Start()+lex.Pos(amp), text.End(), SeverityWarning, DiagUnescapedAmpersand, msgUnescapedAmpersand)
}

// unfinishedReference returns the index of the & that starts the reference
// text ends with, or -1. The reference is unfinished when the text ends with
// & and any of #, digits and letters after it.
func unfinishedReference(text []byte) int {
	j := len(text)
	for j > 0 && isReferenceChar(text[j-1]) {
		j--
	}
	if j > 0 && text[j-1] == '&' {
		return j - 1
	}
	return -1
}

func isReferenceChar(c byte) bool {
	return c == '#' || ('0' <= c && c <= '9') || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

// writesNext reports whether the first node after nodes[i] that is not a
// template comment writes output.
func writesNext(nodes []Node, i int) bool {
	for _, n := range nodes[i+1:] {
		switch n.(type) {
		case *CommentNode:
			continue
		case *ActionNode, *IfNode, *RangeNode, *WithNode, *TemplateNode, *BlockNode:
			return true
		default:
			return false
		}
	}
	return false
}
