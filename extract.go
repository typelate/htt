// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// Modifications copyright 2026 christopher hunter. all rights reserved.

package htt

import (
	"iter"

	"github.com/typelate/htt/lex"
)

// Extract yields a tree, by name, for each {{define}} and {{block}} in src.
// It does not yield src itself or look inside a definition's body.
func Extract(src *Tree) iter.Seq2[string, *Tree] {
	return func(yield func(string, *Tree) bool) {
		extractDefines(yield, src, src.Root)
	}
}

// DefinedTrees returns the trees a parse defined, by name, as a
// text/template set would hold them. root is the tree ParseTree returned
// and treeSet the set it filled. A define's tree comes from Extract and
// carries the diagnostics recorded in its body.
func DefinedTrees(root *Tree, treeSet map[string]*Tree) map[string]*Tree {
	extracted := make(map[*ListNode]*Tree)
	for _, tree := range Extract(root) {
		extracted[tree.Root] = tree
	}
	trees := make(map[string]*Tree, len(treeSet))
	for name, kept := range treeSet {
		if tree, ok := extracted[kept.Root]; ok && kept.define {
			trees[name] = tree
			continue
		}
		trees[name] = kept
	}
	return trees
}

// extractDefines yields a tree for each define and block under n. It
// reports whether to go on.
func extractDefines(yield func(string, *Tree) bool, parent *Tree, n Node) bool {
	if n == nil {
		return true
	}
	switch n := n.(type) {
	case *DefineNode:
		dt := &Tree{
			Name: n.Name,
		}
		dt.Root = n.Body
		dt.ParseName = parent.ParseName
		dt.Filepath = parent.Filepath
		dt.text = parent.text
		dt.leftDelim, dt.rightDelim = parent.leftDelim, parent.rightDelim
		dt.Mode = parent.Mode
		dt.HTMLMode = parent.HTMLMode
		dt.Diagnostics = diagnosticsInRange(parent.Diagnostics, n.Body.Start(), n.Body.End())
		dt.RootTrimFirst = n.StartTrimRight
		dt.RootTrimLast = n.EndTrimLeft
		return yield(n.Name, dt)
	case *BlockNode:
		// No root trim: a block executes as the tree blockControl
		// registered. This one serves source ranges and structure.
		dt := &Tree{
			Name: n.Name,
		}
		dt.Root = n.Body
		dt.ParseName = parent.ParseName
		dt.Filepath = parent.Filepath
		dt.text = parent.text
		dt.leftDelim, dt.rightDelim = parent.leftDelim, parent.rightDelim
		dt.Mode = parent.Mode
		dt.HTMLMode = parent.HTMLMode
		dt.Diagnostics = diagnosticsInRange(parent.Diagnostics, n.Body.Start(), n.Body.End())
		return yield(n.Name, dt)
	case *ListNode:
		if n != nil {
			for _, child := range n.Nodes {
				if !extractDefines(yield, parent, child) {
					return false
				}
			}
		}
	case *HTMLDocumentFragmentNode:
		for _, child := range n.Children {
			if !extractDefines(yield, parent, child) {
				return false
			}
		}
	case *HTMLElementNode:
		for _, attr := range n.Attr {
			if !extractDefines(yield, parent, attr) {
				return false
			}
		}
		for _, child := range n.Children {
			if !extractDefines(yield, parent, child) {
				return false
			}
		}
	case *IfNode:
		if !extractDefines(yield, parent, n.List) {
			return false
		}
		if !extractDefines(yield, parent, n.ElseList) {
			return false
		}
	case *RangeNode:
		if !extractDefines(yield, parent, n.List) {
			return false
		}
		if !extractDefines(yield, parent, n.ElseList) {
			return false
		}
	case *WithNode:
		if !extractDefines(yield, parent, n.List) {
			return false
		}
		if !extractDefines(yield, parent, n.ElseList) {
			return false
		}
	}
	return true
}

// diagnosticsInRange returns the diagnostics positioned in [start, end).
func diagnosticsInRange(ds Diagnostics, start, end lex.Pos) Diagnostics {
	var out Diagnostics
	for _, d := range ds {
		if d.Pos >= start && d.Pos < end {
			out = append(out, d)
		}
	}
	return out
}
