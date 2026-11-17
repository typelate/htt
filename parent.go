package htt

import "reflect"

// nodeParent records a structural node's lexical parent. The nodes inside
// an action have none.
type nodeParent struct {
	parent Node
}

// Parent returns the node's lexical parent. It is nil for a root, and for a
// DefineNode or BlockNode, which root a template of their own.
func (n nodeParent) Parent() Node { return n.parent }

func (n *nodeParent) setParent(p Node) { n.parent = p }

// parentSetter is every node that embeds nodeParent.
type parentSetter interface {
	setParent(Node)
}

// setParents assigns each structural node its parent, once, after parsing.
func (t *Tree) setParents() {
	assignParents(t.Root, nil)
}

func assignParents(n, parent Node) {
	if isNilNode(n) {
		return
	}
	self := parent
	switch n.(type) {
	case *DefineNode, *BlockNode:
		self = nil // the root of its own template
	}
	if ps, ok := n.(parentSetter); ok {
		ps.setParent(self)
	}
	forEachStructuralChild(n, func(child Node) {
		assignParents(child, n)
	})
}

// isNilNode reports whether n is a nil interface or a typed nil pointer.
func isNilNode(n Node) bool {
	if n == nil {
		return true
	}
	v := reflect.ValueOf(n)
	return v.Kind() == reflect.Ptr && v.IsNil()
}

// forEachStructuralChild calls fn for each structural child of n, in source
// order.
func forEachStructuralChild(n Node, fn func(Node)) {
	switch n := n.(type) {
	case *ListNode:
		for _, c := range n.Nodes {
			fn(c)
		}
	case *HTMLDocumentFragmentNode:
		for _, c := range n.Children {
			fn(c)
		}
	case *HTMLElementNode:
		for _, a := range n.Attr {
			fn(a)
		}
		for _, c := range n.Children {
			fn(c)
		}
	case *HTMLAttributeNode:
		for _, k := range n.Key {
			fn(k)
		}
		for _, v := range n.Value {
			fn(v)
		}
	case *HTMLCommentNode:
		for _, c := range n.Children {
			fn(c)
		}
	case *IfNode:
		branchChildren(&n.BranchNode, fn)
	case *RangeNode:
		branchChildren(&n.BranchNode, fn)
	case *WithNode:
		branchChildren(&n.BranchNode, fn)
	case *DefineNode:
		fn(n.Body)
	case *BlockNode:
		fn(n.Body)
	}
}

func branchChildren(b *BranchNode, fn func(Node)) {
	if b.List != nil {
		fn(b.List)
	}
	if b.ElseList != nil {
		fn(b.ElseList)
	}
}
