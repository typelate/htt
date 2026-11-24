package htt

import (
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/typelate/htt/lex"
)

// copyCorpus holds sources that between them parse to every node type, as
// text and with HTML structure.
var copyCorpus = []string{
	`{{/* c */}}<!DOCTYPE html><!-- c --><div id="a" class={{.C}} data-x="p{{.X}}q" hidden>t{{.A | printf "%s" | html}}<br><script>var x = {{.J}};</script></div>`,
	`{{define "d"}}<td>{{.}}</td>{{end}}{{block "b" .}}<p>{{.}}</p>{{end}}{{template "d" .X}}`,
	`{{if .A}}a{{else if .B}}b{{else}}c{{end}}{{with $x := .M.k}}{{$x}}{{else with .N}}{{.}}{{else}}n{{end}}`,
	`{{range $i, $e := .L}}{{if eq $i 1}}{{continue}}{{end}}{{if gt $i 5}}{{break}}{{end}}{{$e}}{{else}}none{{end}}`,
	`{{$v := 1}}{{$v = 2.5}}{{print nil true 'a' 0x10 1e3 "s" ` + "`raw`" + ` .F.G $v (len .L) (.M).k}}{{- .T -}}`,
	`<ul>{{range .L}}<li title='{{.}}'>{{.}}</li>{{end}}</ul><textarea>{{.T}}</textarea><input value={{.V}} disabled>`,
	``,
}

// copyNodeTypes are the node types a parsed tree can hold.
var copyNodeTypes = []string{
	"ActionNode", "BlockNode", "BoolNode", "BreakNode", "ChainNode", "CommandNode", "CommentNode",
	"ContinueNode", "DefineNode", "DotNode", "FieldNode", "HTMLAttributeNode", "HTMLCommentNode",
	"HTMLDoctypeNode", "HTMLDocumentFragmentNode", "HTMLElementNode", "IdentifierNode", "IfNode",
	"ListNode", "NilNode", "NumberNode", "PipeNode", "RangeNode", "StringNode", "TemplateNode",
	"TextNode", "VariableNode", "WithNode",
}

// copyCorpusTrees parses the corpus as text and with HTML structure and
// returns every tree those parses give: each source's own, the trees its
// definitions register, and the trees Extract makes. The trees with HTML
// structure are annotated, so their actions hold contexts.
func copyCorpusTrees(t *testing.T) map[string]*Tree {
	t.Helper()
	trees := make(map[string]*Tree)
	for i, text := range copyCorpus {
		for _, mode := range []string{"text", "html"} {
			options := lex.Options{EmitComment: true, BreakOK: true, ContinueOK: true, ParseHTML: mode == "html"}
			treeSet := make(map[string]*Tree)
			root, err := ParseTree("corpus", "corpus", text, treeSet, options)
			if err != nil {
				t.Fatalf("corpus %d as %s: %v", i, mode, err)
			}
			if mode == "html" {
				AnnotateHTMLContexts(root.Root)
			}
			key := func(kind, name string) string {
				return mode + " " + kind + " " + name + " of " + text
			}
			trees[key("root", root.Name)] = root
			for name, tree := range treeSet {
				trees[key("registered", name)] = tree
			}
			for name, tree := range Extract(root) {
				trees[key("extracted", name)] = tree
			}
		}
	}
	return trees
}

func TestCopyOfANilTreeIsNil(t *testing.T) {
	if got := (*Tree)(nil).Copy(); got != nil {
		t.Errorf("Copy of a nil tree = %v, want nil", got)
	}
}

func TestCorpusForCopyHoldsEveryNodeType(t *testing.T) {
	seen := make(map[string]bool)
	for _, tree := range copyCorpusTrees(t) {
		Walk(tree, func(_ *Tree, n Node) {
			seen[reflect.TypeOf(n).Elem().Name()] = true
		})
		// Walk does not visit the nodes inside a pipeline.
		collectNodeTypes(reflect.ValueOf(tree.Root), seen, make(map[uintptr]bool))
	}
	for _, name := range copyNodeTypes {
		if !seen[name] {
			t.Errorf("no tree of the corpus holds a %s", name)
		}
	}
}

// collectNodeTypes records the name of every struct type whose name ends in
// Node that v reaches.
func collectNodeTypes(v reflect.Value, seen map[string]bool, visited map[uintptr]bool) {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() || visited[v.Pointer()] || v.Type() == reflect.TypeFor[*Tree]() {
			return
		}
		visited[v.Pointer()] = true
		collectNodeTypes(v.Elem(), seen, visited)
	case reflect.Interface:
		if !v.IsNil() {
			collectNodeTypes(v.Elem(), seen, visited)
		}
	case reflect.Struct:
		if name := v.Type().Name(); len(name) > 4 && name[len(name)-4:] == "Node" {
			seen[name] = true
		}
		for i := range v.NumField() {
			collectNodeTypes(v.Field(i), seen, visited)
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			collectNodeTypes(v.Index(i), seen, visited)
		}
	}
}

// A copy holds what the original holds, field for field, and shares no
// pointer, slice or map with it: every node of the copy belongs to the copy.
func TestCopySharesNothingAndDiffersInNothing(t *testing.T) {
	for name, tree := range copyCorpusTrees(t) {
		t.Run(name, func(t *testing.T) {
			cp := tree.Copy()
			if cp == nil {
				t.Fatal("Copy returned nil")
			}
			c := copyChecker{t: t, original: tree, copied: cp, pairs: make(map[uintptr]uintptr)}
			c.check(reflect.ValueOf(tree), reflect.ValueOf(cp), "tree")
			if got, want := cp.Root.String(), tree.Root.String(); got != want {
				t.Errorf("the copy's root prints %q, want %q", got, want)
			}
		})
	}
}

// A copy has not run: its counters are zero, and running it leaves the
// original's counters alone.
func TestCopyHasItsOwnCounters(t *testing.T) {
	for name, tree := range copyCorpusTrees(t) {
		t.Run(name, func(t *testing.T) {
			addToCounters(reflect.ValueOf(tree.Root), 3, make(map[uintptr]bool))
			defer addToCounters(reflect.ValueOf(tree.Root), -3, make(map[uintptr]bool))
			cp := tree.Copy()
			if got := sumOfCounters(reflect.ValueOf(cp.Root), make(map[uintptr]bool)); got != 0 {
				t.Errorf("the copy's counters sum to %d, want 0", got)
			}
			before := sumOfCounters(reflect.ValueOf(tree.Root), make(map[uintptr]bool))
			addToCounters(reflect.ValueOf(cp.Root), 5, make(map[uintptr]bool))
			if after := sumOfCounters(reflect.ValueOf(tree.Root), make(map[uintptr]bool)); after != before {
				t.Errorf("counting on the copy changed the original's counters from %d to %d", before, after)
			}
		})
	}
}

var (
	atomicInt64Type = reflect.TypeFor[atomic.Int64]()
	onceType        = reflect.TypeFor[sync.Once]()
	treePointerType = reflect.TypeFor[*Tree]()
)

// forEachCounter calls fn with a pointer to each counter v reaches.
func forEachCounter(v reflect.Value, visited map[uintptr]bool, fn func(*atomic.Int64)) {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() || visited[v.Pointer()] || v.Type() == treePointerType {
			return
		}
		visited[v.Pointer()] = true
		forEachCounter(v.Elem(), visited, fn)
	case reflect.Interface:
		if !v.IsNil() {
			forEachCounter(v.Elem(), visited, fn)
		}
	case reflect.Struct:
		if v.Type() == atomicInt64Type {
			fn((*atomic.Int64)(v.Addr().UnsafePointer()))
			return
		}
		for i := range v.NumField() {
			forEachCounter(v.Field(i), visited, fn)
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			forEachCounter(v.Index(i), visited, fn)
		}
	}
}

func addToCounters(v reflect.Value, delta int64, visited map[uintptr]bool) {
	forEachCounter(v, visited, func(c *atomic.Int64) { c.Add(delta) })
}

func sumOfCounters(v reflect.Value, visited map[uintptr]bool) int64 {
	var sum int64
	forEachCounter(v, visited, func(c *atomic.Int64) { sum += c.Load() })
	return sum
}

// copyTreeFieldsNotCompared are the fields of Tree a copy need not carry:
// the line table is a cache, the function names are read only, and the rest
// is parsing state, which text/template's Copy discards too.
var copyTreeFieldsNotCompared = []string{
	"linesOnce", "lines", "functions",
	"scanner", "token", "peekCount", "vars", "treeSet", "actionLine", "actionPos",
	"actionTrimLeft", "actionTrimRight", "rangeDepth", "stackDepth",
	"htmlAncestors", "htmlDegraded", "htmlDegradedMin", "htmlDepth",
}

// copyChecker compares an original tree with its copy by reflection, fields
// that are not exported included.
type copyChecker struct {
	t                *testing.T
	original, copied *Tree
	// pairs maps a pointer of the original to the pointer that stands for it
	// in the copy.
	pairs map[uintptr]uintptr
}

func (c *copyChecker) check(a, b reflect.Value, path string) {
	if a.Kind() != b.Kind() {
		c.t.Errorf("%s: the original holds a %s, the copy a %s", path, a.Kind(), b.Kind())
		return
	}
	switch a.Kind() {
	case reflect.Pointer:
		if a.IsNil() || b.IsNil() {
			if a.IsNil() != b.IsNil() {
				c.t.Errorf("%s: nil in one and not in the other (original nil: %t)", path, a.IsNil())
			}
			return
		}
		if a.Type() != b.Type() {
			c.t.Errorf("%s: the original holds a %s, the copy a %s", path, a.Type(), b.Type())
			return
		}
		if a.Type() == treePointerType && path != "tree" {
			// A node's tree: nodes of a define's extracted tree point at
			// the tree of the whole source, and the copy must not.
			if b.Pointer() != reflect.ValueOf(c.copied).Pointer() {
				c.t.Errorf("%s: a node of the copy does not belong to the copy", path)
			}
			return
		}
		if a.Pointer() == b.Pointer() {
			c.t.Errorf("%s: the copy shares a %s with the original", path, a.Type())
			return
		}
		if prev, ok := c.pairs[a.Pointer()]; ok {
			if prev != b.Pointer() {
				c.t.Errorf("%s: one %s of the original has two copies", path, a.Type())
			}
			return
		}
		c.pairs[a.Pointer()] = b.Pointer()
		c.check(a.Elem(), b.Elem(), path)
	case reflect.Interface:
		if a.IsNil() || b.IsNil() {
			if a.IsNil() != b.IsNil() {
				c.t.Errorf("%s: nil in one and not in the other (original nil: %t)", path, a.IsNil())
			}
			return
		}
		c.check(a.Elem(), b.Elem(), path)
	case reflect.Struct:
		if a.Type() != b.Type() {
			c.t.Errorf("%s: the original holds a %s, the copy a %s", path, a.Type(), b.Type())
			return
		}
		switch a.Type() {
		case atomicInt64Type, onceType:
			return // TestCopyHasItsOwnCounters checks the counters.
		}
		for i := range a.NumField() {
			field := a.Type().Field(i)
			if a.Type() == treePointerType.Elem() && slices.Contains(copyTreeFieldsNotCompared, field.Name) {
				continue
			}
			if field.Name == "parent" && path == "tree.Root.nodeParent" {
				// The root of a define's tree has the define for a parent,
				// which is outside the tree. The copy may have none.
				if !b.Field(i).IsNil() && !a.Field(i).IsNil() && b.Field(i).Elem().Pointer() == a.Field(i).Elem().Pointer() {
					c.t.Errorf("%s.parent: the copy's root shares its parent with the original", path)
				}
				continue
			}
			c.check(a.Field(i), b.Field(i), path+"."+field.Name)
		}
	case reflect.Slice:
		if a.Len() != b.Len() {
			c.t.Errorf("%s: length %d in the original, %d in the copy", path, a.Len(), b.Len())
			return
		}
		if a.Len() > 0 && a.Pointer() == b.Pointer() {
			c.t.Errorf("%s: the copy shares a %s with the original", path, a.Type())
			return
		}
		for i := range a.Len() {
			c.check(a.Index(i), b.Index(i), path+"[]")
		}
	case reflect.Array:
		for i := range a.Len() {
			c.check(a.Index(i), b.Index(i), path+"[]")
		}
	case reflect.Map:
		if a.Len() != b.Len() {
			c.t.Errorf("%s: %d entries in the original, %d in the copy", path, a.Len(), b.Len())
		}
		if a.Len() > 0 && a.Pointer() == b.Pointer() {
			c.t.Errorf("%s: the copy shares a %s with the original", path, a.Type())
		}
	case reflect.String:
		if a.String() != b.String() {
			c.t.Errorf("%s: %q in the original, %q in the copy", path, a.String(), b.String())
		}
	case reflect.Bool:
		if a.Bool() != b.Bool() {
			c.t.Errorf("%s: %t in the original, %t in the copy", path, a.Bool(), b.Bool())
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if a.Int() != b.Int() {
			c.t.Errorf("%s: %d in the original, %d in the copy", path, a.Int(), b.Int())
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if a.Uint() != b.Uint() {
			c.t.Errorf("%s: %d in the original, %d in the copy", path, a.Uint(), b.Uint())
		}
	case reflect.Float32, reflect.Float64:
		if a.Float() != b.Float() {
			c.t.Errorf("%s: %g in the original, %g in the copy", path, a.Float(), b.Float())
		}
	case reflect.Complex64, reflect.Complex128:
		if a.Complex() != b.Complex() {
			c.t.Errorf("%s: %g in the original, %g in the copy", path, a.Complex(), b.Complex())
		}
	default:
		if !a.IsNil() || !b.IsNil() {
			c.t.Errorf("%s: a %s the check does not know how to compare", path, a.Kind())
		}
	}
}
