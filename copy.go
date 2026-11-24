package htt

import (
	"fmt"
	"slices"
)

// Copy returns a copy of the tree, as text/template/parse's Copy does. The
// nodes are copied, so a change to one tree does not reach the other. The
// copy has not run, so its coverage counters are zero. Parsing state is
// discarded.
func (t *Tree) Copy() *Tree {
	if t == nil {
		return nil
	}
	cp := &Tree{
		Name:          t.Name,
		ParseName:     t.ParseName,
		Filepath:      t.Filepath,
		Mode:          t.Mode,
		HTMLMode:      t.HTMLMode,
		Diagnostics:   slices.Clone(t.Diagnostics),
		RootTrimFirst: t.RootTrimFirst,
		RootTrimLast:  t.RootTrimLast,
		text:          t.text,
		leftDelim:     t.leftDelim,
		rightDelim:    t.rightDelim,
		define:        t.define,
		functions:     t.functions, // read only, so shared
		maxHTMLDepth:  t.maxHTMLDepth,
	}
	c := copier{
		tree:     cp,
		seen:     make(map[Node]Node),
		contexts: make(map[*HTMLContext]*HTMLContext),
	}
	cp.Root = c.list(t.Root)
	c.linkParents()
	return cp
}

// copier copies the nodes of one tree into tree. seen maps each original node
// to its copy, so a node that is reached twice is copied once.
type copier struct {
	tree     *Tree
	seen     map[Node]Node
	contexts map[*HTMLContext]*HTMLContext
}

// node returns the copy of n. A nil interface and a nil pointer stay as they are.
func (c *copier) node(n Node) Node {
	if isNilNode(n) {
		return n
	}
	if cp, ok := c.seen[n]; ok {
		return cp
	}
	cp := c.build(n)
	c.seen[n] = cp
	return cp
}

// linkParents gives each copied node the copy of its original parent. A
// parent outside the copy, such as the define that holds an extracted tree's
// root, leaves the link nil. The original's links are copied, not recomputed,
// because a tree registered by a define was never parented in the original.
func (c *copier) linkParents() {
	for orig, cp := range c.seen {
		ps, ok := cp.(parentSetter)
		op, okOrig := orig.(interface{ Parent() Node })
		if !ok || !okOrig {
			continue
		}
		ps.setParent(c.seen[op.Parent()])
	}
}

// build returns a new node with the fields of n. The counters stay zero.
// Parents are left for linkParents.
func (c *copier) build(n Node) Node {
	switch n := n.(type) {
	case *ListNode:
		return &ListNode{tr: c.tree, start: n.start, end: n.end, Nodes: c.nodeList(n.Nodes)}
	case *TextNode:
		return &TextNode{tr: c.tree, start: n.start, end: n.end, Text: slices.Clone(n.Text), Verbatim: n.Verbatim}
	case *CommentNode:
		return &CommentNode{tr: c.tree, start: n.start, end: n.end, Text: n.Text, TrimLeft: n.TrimLeft, TrimRight: n.TrimRight}
	case *PipeNode:
		return &PipeNode{
			tr: c.tree, start: n.start, end: n.end, Line: n.Line, IsAssign: n.IsAssign,
			Decl: c.variables(n.Decl), Commands: c.commands(n.Commands),
		}
	case *ActionNode:
		return &ActionNode{
			tr: c.tree, start: n.start, end: n.end, Line: n.Line,
			Pipe: c.pipe(n.Pipe), HTMLContext: c.htmlContext(n.HTMLContext),
			TrimLeft: n.TrimLeft, TrimRight: n.TrimRight,
		}
	case *CommandNode:
		return &CommandNode{tr: c.tree, start: n.start, end: n.end, Args: c.nodeList(n.Args)}
	case *IdentifierNode:
		return &IdentifierNode{tr: c.tree, start: n.start, end: n.end, Ident: n.Ident}
	case *VariableNode:
		return &VariableNode{tr: c.tree, start: n.start, end: n.end, Ident: slices.Clone(n.Ident)}
	case *DotNode:
		return &DotNode{tr: c.tree, start: n.start, end: n.end}
	case *NilNode:
		return &NilNode{tr: c.tree, start: n.start, end: n.end}
	case *FieldNode:
		return &FieldNode{tr: c.tree, start: n.start, end: n.end, Ident: slices.Clone(n.Ident)}
	case *ChainNode:
		return &ChainNode{tr: c.tree, start: n.start, end: n.end, Node: c.node(n.Node), Field: slices.Clone(n.Field)}
	case *BoolNode:
		return &BoolNode{tr: c.tree, start: n.start, end: n.end, True: n.True}
	case *StringNode:
		return &StringNode{tr: c.tree, start: n.start, end: n.end, Quoted: n.Quoted, Text: n.Text}
	case *NumberNode:
		return &NumberNode{
			tr: c.tree, start: n.start, end: n.end,
			IsInt: n.IsInt, IsUint: n.IsUint, IsFloat: n.IsFloat, IsComplex: n.IsComplex,
			Int64: n.Int64, Uint64: n.Uint64, Float64: n.Float64, Complex128: n.Complex128,
			Text: n.Text,
		}
	case *BreakNode:
		return &BreakNode{
			tr: c.tree, start: n.start, end: n.end, keyword: n.keyword, Line: n.Line,
			TrimLeft: n.TrimLeft, TrimRight: n.TrimRight,
		}
	case *ContinueNode:
		return &ContinueNode{
			tr: c.tree, start: n.start, end: n.end, keyword: n.keyword, Line: n.Line,
			TrimLeft: n.TrimLeft, TrimRight: n.TrimRight,
		}
	case *IfNode:
		cp := &IfNode{}
		c.fillBranch(&cp.BranchNode, &n.BranchNode)
		return cp
	case *RangeNode:
		cp := &RangeNode{}
		c.fillBranch(&cp.BranchNode, &n.BranchNode)
		return cp
	case *WithNode:
		cp := &WithNode{}
		c.fillBranch(&cp.BranchNode, &n.BranchNode)
		return cp
	case *TemplateNode:
		return &TemplateNode{
			tr: c.tree, start: n.start, end: n.end, nameStart: n.nameStart, Line: n.Line, Name: n.Name,
			Pipe: c.pipe(n.Pipe), HTMLContext: c.htmlContext(n.HTMLContext),
			TrimLeft: n.TrimLeft, TrimRight: n.TrimRight,
		}
	case *DefineNode:
		return &DefineNode{
			tr: c.tree, start: n.start, end: n.end, Line: n.Line, Name: n.Name, Body: c.list(n.Body),
			StartTrimLeft: n.StartTrimLeft, StartTrimRight: n.StartTrimRight,
			EndTrimLeft: n.EndTrimLeft, EndTrimRight: n.EndTrimRight,
		}
	case *BlockNode:
		return &BlockNode{
			tr: c.tree, start: n.start, end: n.end, Line: n.Line, Name: n.Name,
			Pipe: c.pipe(n.Pipe), Body: c.list(n.Body),
			StartTrimLeft: n.StartTrimLeft, StartTrimRight: n.StartTrimRight,
			EndTrimLeft: n.EndTrimLeft, EndTrimRight: n.EndTrimRight,
			call: c.template(n.call),
		}
	case *HTMLDocumentFragmentNode:
		return &HTMLDocumentFragmentNode{tr: c.tree, start: n.start, end: n.end, Children: c.nodeList(n.Children)}
	case *HTMLElementNode:
		return &HTMLElementNode{
			tr: c.tree, start: n.start, end: n.end, TagName: n.TagName,
			Attr: c.nodeList(n.Attr), Children: c.nodeList(n.Children),
			Void: n.Void, Closed: n.Closed, Misnested: n.Misnested,
			AttrStart: n.AttrStart, AttrEnd: n.AttrEnd, EndTagStart: n.EndTagStart,
		}
	case *HTMLAttributeNode:
		return &HTMLAttributeNode{
			tr: c.tree, start: n.start, end: n.end, Namespace: n.Namespace,
			Key: c.nodeList(n.Key), Value: c.nodeList(n.Value), HasValue: n.HasValue,
		}
	case *HTMLCommentNode:
		return &HTMLCommentNode{tr: c.tree, start: n.start, end: n.end, Children: c.nodeList(n.Children)}
	case *HTMLDoctypeNode:
		return &HTMLDoctypeNode{tr: c.tree, start: n.start, end: n.end, Text: n.Text}
	default:
		panic(fmt.Sprintf("htt: Copy of unknown node %T", n))
	}
}

// fillBranch sets the fields of dst, a copy of the branch src. A BranchNode
// holds atomic counters, so it is filled in place rather than returned.
func (c *copier) fillBranch(dst, src *BranchNode) {
	dst.tr = c.tree
	dst.start, dst.end, dst.Line = src.start, src.end, src.Line
	dst.Pipe = c.pipe(src.Pipe)
	dst.List = c.list(src.List)
	dst.ElseList = c.list(src.ElseList)
	dst.StartTrimLeft, dst.StartTrimRight = src.StartTrimLeft, src.StartTrimRight
	dst.ElseTrimLeft, dst.ElseTrimRight = src.ElseTrimLeft, src.ElseTrimRight
	dst.EndTrimLeft, dst.EndTrimRight = src.EndTrimLeft, src.EndTrimRight
	dst.elseChained = src.elseChained
}

func (c *copier) nodeList(ns []Node) []Node {
	if ns == nil {
		return nil
	}
	out := make([]Node, len(ns))
	for i, n := range ns {
		out[i] = c.node(n)
	}
	return out
}

func (c *copier) list(l *ListNode) *ListNode {
	if l == nil {
		return nil
	}
	return c.node(l).(*ListNode)
}

func (c *copier) pipe(p *PipeNode) *PipeNode {
	if p == nil {
		return nil
	}
	return c.node(p).(*PipeNode)
}

func (c *copier) command(cmd *CommandNode) *CommandNode {
	if cmd == nil {
		return nil
	}
	return c.node(cmd).(*CommandNode)
}

func (c *copier) commands(cmds []*CommandNode) []*CommandNode {
	if cmds == nil {
		return nil
	}
	out := make([]*CommandNode, len(cmds))
	for i, cmd := range cmds {
		out[i] = c.command(cmd)
	}
	return out
}

func (c *copier) variable(v *VariableNode) *VariableNode {
	if v == nil {
		return nil
	}
	return c.node(v).(*VariableNode)
}

func (c *copier) variables(vs []*VariableNode) []*VariableNode {
	if vs == nil {
		return nil
	}
	out := make([]*VariableNode, len(vs))
	for i, v := range vs {
		out[i] = c.variable(v)
	}
	return out
}

func (c *copier) template(t *TemplateNode) *TemplateNode {
	if t == nil {
		return nil
	}
	return c.node(t).(*TemplateNode)
}

func (c *copier) htmlContext(h *HTMLContext) *HTMLContext {
	if h == nil {
		return nil
	}
	if cp, ok := c.contexts[h]; ok {
		return cp
	}
	cp := *h
	c.contexts[h] = &cp
	return &cp
}
