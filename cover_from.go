package htt

import (
	"cmp"
	"slices"
	"sync/atomic"

	"github.com/typelate/htt/lex"
)

// CoverFrom sets t's counts from text, a tree parsed without HTML structure
// from the same source, matching nodes by source offset. It is how a tree
// with HTML structure reports a run that executed only text. A block that
// text does not cover keeps its count, since a define body is listed by
// the tree that holds it and by its own tree.
func (t *Tree) CoverFrom(text *Tree) {
	if t == nil || text == nil {
		return
	}
	idx := newCoverIndex(text)
	idx.set(t.Root)
}

// coverIndex holds a text tree's counters by source offset.
type coverIndex struct {
	spans    []coverSpan               // text nodes, in source order
	leaves   map[lex.Pos]*atomic.Int64 // actions, template calls, break and continue, by start
	branches map[lex.Pos]*BranchNode   // if, range and with, by start
}

type coverSpan struct {
	start, end lex.Pos
	count      *atomic.Int64
}

func newCoverIndex(text *Tree) *coverIndex {
	idx := &coverIndex{
		leaves:   make(map[lex.Pos]*atomic.Int64),
		branches: make(map[lex.Pos]*BranchNode),
	}
	Walk(text, func(_ *Tree, node Node) {
		switch n := node.(type) {
		case *TextNode:
			idx.spans = append(idx.spans, coverSpan{start: n.Start(), end: n.End(), count: &n.count})
		case *ActionNode:
			idx.leaves[n.Start()] = &n.count
		case *TemplateNode:
			idx.leaves[n.Start()] = &n.count
		case *BreakNode:
			idx.leaves[n.Start()] = &n.count
		case *ContinueNode:
			idx.leaves[n.Start()] = &n.count
		case *IfNode:
			idx.branches[n.start] = &n.BranchNode
		case *RangeNode:
			idx.branches[n.start] = &n.BranchNode
		case *WithNode:
			idx.branches[n.start] = &n.BranchNode
		}
	})
	return idx
}

// store sets dst to the count of the text node at pos: an action, template
// call, break or continue starting at pos, else the text node covering pos.
// It leaves dst alone when no text node is at pos.
func (idx *coverIndex) store(dst *atomic.Int64, pos lex.Pos) {
	if c, ok := idx.leaves[pos]; ok {
		dst.Store(c.Load())
		return
	}
	i, found := slices.BinarySearchFunc(idx.spans, pos, func(s coverSpan, p lex.Pos) int {
		return cmp.Compare(s.start, p)
	})
	if found {
		dst.Store(idx.spans[i].count.Load())
		return
	}
	if i > 0 && pos < idx.spans[i-1].end {
		dst.Store(idx.spans[i-1].count.Load())
	}
}

// set stores the text counts on the HTML nodes of node and their descendants.
func (idx *coverIndex) set(node Node) {
	switch n := node.(type) {
	case *ListNode:
		if n == nil {
			return
		}
		for _, child := range n.Nodes {
			idx.set(child)
		}
	case *HTMLDocumentFragmentNode:
		for _, child := range n.Children {
			idx.set(child)
		}
	case *TextNode:
		idx.store(&n.count, n.Start())
	case *ActionNode:
		idx.store(&n.count, n.Start())
	case *TemplateNode:
		idx.store(&n.count, n.Start())
	case *BreakNode:
		idx.store(&n.count, n.Start())
	case *ContinueNode:
		idx.store(&n.count, n.Start())
	case *HTMLAttributeNode:
		idx.store(&n.count, n.Start())
	case *HTMLCommentNode:
		idx.store(&n.count, n.Start())
	case *HTMLDoctypeNode:
		idx.store(&n.count, n.Start())
	case *IfNode:
		idx.setBranch(&n.BranchNode)
	case *RangeNode:
		idx.setBranch(&n.BranchNode)
	case *WithNode:
		idx.setBranch(&n.BranchNode)
	case *HTMLElementNode:
		idx.setElement(n)
	case *DefineNode:
		idx.set(n.Body)
	case *BlockNode:
		idx.set(n.Body)
	}
}

// setBranch takes the opener, else and end counts from the text branch that
// starts at the same offset.
func (idx *coverIndex) setBranch(b *BranchNode) {
	if tb, ok := idx.branches[b.start]; ok {
		b.openCount.Store(tb.openCount.Load())
		b.elseCount.Store(tb.elseCount.Load())
		b.endCount.Store(tb.endCount.Load())
	}
	idx.set(b.List)
	idx.set(b.ElseList)
}

// setElement mirrors coverWalkElement's blocks: "<tag", the attributes and
// the '>' after them, the children, and the end tag.
func (idx *coverIndex) setElement(elem *HTMLElementNode) {
	idx.store(&elem.openCount, elem.Start())
	for _, attr := range elem.Attr {
		idx.set(attr)
	}
	attrCloseStart := elem.AttrStart
	if len(elem.Attr) > 0 {
		attrCloseStart = elem.Attr[len(elem.Attr)-1].End()
	}
	idx.store(&elem.attrCount, attrCloseStart)
	if elem.Void {
		return
	}
	for _, child := range elem.Children {
		idx.set(child)
	}
	if elem.Closed {
		idx.store(&elem.closeCount, elem.EndTagStart)
	}
}
