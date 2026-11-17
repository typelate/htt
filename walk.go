package htt

// Walk calls fn for each node of tree in source order, a parent before its
// children. It does not enter actions.
func Walk(tree *Tree, fn func(*Tree, Node)) {
	WalkPath(tree, func(t *Tree, _ []Node, n Node) { fn(t, n) })
}

// WalkPath is Walk with the node's ancestors, outermost first. The path
// includes list and branch nodes. fn must not keep path, whose array is
// reused.
func WalkPath(tree *Tree, fn func(t *Tree, path []Node, n Node)) {
	if tree == nil || tree.Root == nil {
		return
	}
	walkPathNode(tree, make([]Node, 0, 16), tree.Root, fn)
}

func walkPathNode(tree *Tree, path []Node, node Node, fn func(*Tree, []Node, Node)) {
	if node == nil {
		return
	}
	fn(tree, path, node)
	path = append(path, node)
	switch n := node.(type) {
	case *ListNode:
		for _, child := range n.Nodes {
			walkPathNode(tree, path, child, fn)
		}
	case *HTMLDocumentFragmentNode:
		for _, child := range n.Children {
			walkPathNode(tree, path, child, fn)
		}
	case *HTMLElementNode:
		for _, attr := range n.Attr {
			walkPathNode(tree, path, attr, fn)
		}
		for _, child := range n.Children {
			walkPathNode(tree, path, child, fn)
		}
	case *IfNode:
		if n.List != nil {
			walkPathNode(tree, path, n.List, fn)
		}
		if n.ElseList != nil {
			walkPathNode(tree, path, n.ElseList, fn)
		}
	case *RangeNode:
		if n.List != nil {
			walkPathNode(tree, path, n.List, fn)
		}
		if n.ElseList != nil {
			walkPathNode(tree, path, n.ElseList, fn)
		}
	case *WithNode:
		if n.List != nil {
			walkPathNode(tree, path, n.List, fn)
		}
		if n.ElseList != nil {
			walkPathNode(tree, path, n.ElseList, fn)
		}
	case *HTMLAttributeNode:
		for _, k := range n.Key {
			walkPathNode(tree, path, k, fn)
		}
		for _, v := range n.Value {
			walkPathNode(tree, path, v, fn)
		}
	case *DefineNode:
		walkPathNode(tree, path, n.Body, fn)
	case *BlockNode:
		walkPathNode(tree, path, n.Body, fn)
	case *HTMLCommentNode:
		for _, child := range n.Children {
			walkPathNode(tree, path, child, fn)
		}
	}
}
