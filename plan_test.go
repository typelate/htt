package htt_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

// planFunc adapts a function to htt.Plan.
type planFunc func(instance string, n *htt.ActionNode) (htt.ActionEdit, bool)

// Text leaves every text node as it is.
func (f planFunc) Text(string, *htt.TextNode) ([]byte, bool) {
	return nil, false
}

// Action calls f.
func (f planFunc) Action(instance string, n *htt.ActionNode) (htt.ActionEdit, bool) {
	return f(instance, n)
}

// Call names no instance.
func (f planFunc) Call(string, htt.Node) string {
	return ""
}

// editFor returns a plan that edits the action whose source text is text.
func editFor(text string, funcs ...func(args ...any) string) planFunc {
	return func(_ string, n *htt.ActionNode) (htt.ActionEdit, bool) {
		if n.String() != text {
			return htt.ActionEdit{}, false
		}
		return htt.ActionEdit{Funcs: funcs}, true
	}
}

func bracket(args ...any) string {
	return "[" + fmt.Sprint(args[0]) + "]"
}

func angle(args ...any) string {
	return "<" + fmt.Sprint(args[0]) + ">"
}

func TestPlanEditsAnActionsOutput(t *testing.T) {
	for _, tt := range []struct {
		name     string
		text     string
		data     any
		plan     planFunc
		instance string
		want     string
	}{
		{
			name: "one function wraps the value",
			text: `{{.}}`,
			data: "v",
			plan: editFor(`{{.}}`, bracket),
			want: "[v]",
		},
		{
			name: "two functions apply in order",
			text: `{{.}}`,
			data: "v",
			plan: editFor(`{{.}}`, bracket, angle),
			want: "<[v]>",
		},
		{
			name: "an action without an edit prints as usual",
			text: `a{{.}}b`,
			data: "v",
			plan: editFor(`{{.x}}`, bracket),
			want: "avb",
		},
		{
			name: "a variable declaration is not edited and a use is",
			text: `{{$x := .}}{{$x}}`,
			data: "v",
			plan: editFor(`{{$x}}`, bracket),
			want: "[v]",
		},
		{
			name: "an action in a called template is edited",
			text: `{{define "t"}}{{.}}{{end}}{{template "t" .}}`,
			data: "v",
			plan: editFor(`{{.}}`, bracket),
			want: "[v]",
		},
		{
			name: "the plan gets the instance of the root",
			text: `{{.}}`,
			data: "v",
			plan: func(instance string, _ *htt.ActionNode) (htt.ActionEdit, bool) {
				return htt.ActionEdit{Funcs: []func(args ...any) string{
					func(...any) string { return instance },
				}}, true
			},
			instance: "root-instance",
			want:     "root-instance",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			treeSet := make(map[string]*htt.Tree)
			root, err := htt.ParseTree("root", "root", tt.text, treeSet, lex.Options{})
			require.NoError(t, err)
			trees := htt.DefinedTrees(root, treeSet)
			finder := htt.FindTreeFunc(func(name string) (*htt.Tree, bool) {
				tree, ok := trees[name]
				return tree, ok
			})

			var buf bytes.Buffer
			err = htt.ExecuteTemplate(&buf, root, tt.data,
				htt.WithFinder(finder), htt.WithPlan(tt.plan, tt.instance))
			require.NoError(t, err)
			if got := buf.String(); got != tt.want {
				t.Errorf("execute(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestPlanDropsTheLastCommands(t *testing.T) {
	plan := planFunc(func(_ string, n *htt.ActionNode) (htt.ActionEdit, bool) {
		if n.String() != `{{.A | html}}` {
			return htt.ActionEdit{}, false
		}
		return htt.ActionEdit{Drop: 1, Funcs: []func(args ...any) string{bracket}}, true
	})
	text := `{{.A | html}}`
	treeSet := make(map[string]*htt.Tree)
	root, err := htt.ParseTree("root", "root", text, treeSet, lex.Options{})
	require.NoError(t, err)
	trees := htt.DefinedTrees(root, treeSet)
	finder := htt.FindTreeFunc(func(name string) (*htt.Tree, bool) {
		tree, ok := trees[name]
		return tree, ok
	})

	var buf bytes.Buffer
	err = htt.ExecuteTemplate(&buf, root, map[string]any{"A": "<b>"},
		htt.WithFinder(finder), htt.WithPlan(plan, ""))
	require.NoError(t, err)
	const want = "[<b>]"
	if got := buf.String(); got != want {
		t.Errorf("execute(%q) = %q, want %q", text, got, want)
	}
}

func TestPlanEvaluatesArgumentsWithoutTheCommand(t *testing.T) {
	join := func(args ...any) string { return fmt.Sprint(args...) }
	plan := planFunc(func(_ string, n *htt.ActionNode) (htt.ActionEdit, bool) {
		if n.String() != `{{html .A .B}}` {
			return htt.ActionEdit{}, false
		}
		return htt.ActionEdit{EvalArgs: true, Funcs: []func(args ...any) string{join, bracket}}, true
	})
	text := `{{html .A .B}}`
	treeSet := make(map[string]*htt.Tree)
	root, err := htt.ParseTree("root", "root", text, treeSet, lex.Options{})
	require.NoError(t, err)
	trees := htt.DefinedTrees(root, treeSet)
	finder := htt.FindTreeFunc(func(name string) (*htt.Tree, bool) {
		tree, ok := trees[name]
		return tree, ok
	})

	var buf bytes.Buffer
	err = htt.ExecuteTemplate(&buf, root, map[string]any{"A": "<b>", "B": "x"},
		htt.WithFinder(finder), htt.WithPlan(plan, ""))
	require.NoError(t, err)
	const want = "[<b>x]"
	if got := buf.String(); got != want {
		t.Errorf("execute(%q) = %q, want %q", text, got, want)
	}
}

// textPlan replaces text nodes through text and leaves actions alone.
type textPlan func(instance string, n *htt.TextNode) ([]byte, bool)

// Text calls f.
func (f textPlan) Text(instance string, n *htt.TextNode) ([]byte, bool) {
	return f(instance, n)
}

// Action leaves every action as it is.
func (f textPlan) Action(string, *htt.ActionNode) (htt.ActionEdit, bool) {
	return htt.ActionEdit{}, false
}

// Call names no instance.
func (f textPlan) Call(string, htt.Node) string {
	return ""
}

// replaceText returns a plan that replaces the text node whose source text is text.
func replaceText(text, repl string) textPlan {
	return func(_ string, n *htt.TextNode) ([]byte, bool) {
		if string(n.Text) != text {
			return nil, false
		}
		return []byte(repl), true
	}
}

func TestPlanReplacesText(t *testing.T) {
	for _, tt := range []struct {
		name string
		text string
		data any
		plan textPlan
		want string
	}{
		{
			name: "a replaced text node is written as the plan gives it",
			text: `hello`,
			plan: replaceText(`hello`, `bye`),
			want: "bye",
		},
		{
			name: "a text node without a replacement is written as it is",
			text: `keep`,
			plan: replaceText(`other`, `bye`),
			want: "keep",
		},
		{
			name: "a replacement next to a trim marker keeps its spaces",
			text: `a {{- .}}`,
			data: "v",
			plan: replaceText(`a `, ` x `),
			want: " x v",
		},
		{
			name: "an empty replacement writes nothing",
			text: `a{{.}}gone`,
			data: "v",
			plan: replaceText(`gone`, ``),
			want: "av",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			treeSet := make(map[string]*htt.Tree)
			root, err := htt.ParseTree("root", "root", tt.text, treeSet, lex.Options{})
			require.NoError(t, err)
			trees := htt.DefinedTrees(root, treeSet)
			finder := htt.FindTreeFunc(func(name string) (*htt.Tree, bool) {
				tree, ok := trees[name]
				return tree, ok
			})

			var buf bytes.Buffer
			err = htt.ExecuteTemplate(&buf, root, tt.data,
				htt.WithFinder(finder), htt.WithPlan(tt.plan, ""))
			require.NoError(t, err)
			if got := buf.String(); got != tt.want {
				t.Errorf("execute(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

// namingPlan names calls through call and writes each action with the instance it runs as.
type namingPlan struct {
	call func(instance string, n htt.Node) string
}

// Text leaves every text node as it is.
func (p namingPlan) Text(string, *htt.TextNode) ([]byte, bool) {
	return nil, false
}

// Action writes the value between the instance and brackets.
func (p namingPlan) Action(instance string, _ *htt.ActionNode) (htt.ActionEdit, bool) {
	return htt.ActionEdit{Funcs: []func(args ...any) string{
		func(args ...any) string { return "[" + instance + ":" + fmt.Sprint(args[0]) + "]" },
	}}, true
}

// Call returns what call returns for n.
func (p namingPlan) Call(instance string, n htt.Node) string {
	return p.call(instance, n)
}

func TestPlanNamesTheCalleesInstance(t *testing.T) {
	for _, tt := range []struct {
		name string
		text string
		call func(instance string, n htt.Node) string
		want string
	}{
		{
			name: "a call the plan renames runs as the new instance",
			text: `{{define "t"}}{{.}}{{end}}{{template "t" .}}`,
			call: func(_ string, n htt.Node) string {
				if _, ok := n.(*htt.TemplateNode); ok {
					return "t-renamed"
				}
				return ""
			},
			want: "[t-renamed:v]",
		},
		{
			name: "a call the plan does not rename runs as the callee's name",
			text: `{{define "t"}}{{.}}{{end}}{{template "t" .}}`,
			call: func(string, htt.Node) string { return "" },
			want: "[t:v]",
		},
		{
			name: "a block is passed to the plan as the block itself",
			text: `{{block "t" .}}{{.}}{{end}}`,
			call: func(_ string, n htt.Node) string {
				if _, ok := n.(*htt.BlockNode); ok {
					return "block-instance"
				}
				return ""
			},
			want: "[block-instance:v]",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			treeSet := make(map[string]*htt.Tree)
			root, err := htt.ParseTree("root", "root", tt.text, treeSet, lex.Options{})
			require.NoError(t, err)
			trees := htt.DefinedTrees(root, treeSet)
			finder := htt.FindTreeFunc(func(name string) (*htt.Tree, bool) {
				tree, ok := trees[name]
				return tree, ok
			})

			var buf bytes.Buffer
			err = htt.ExecuteTemplate(&buf, root, "v",
				htt.WithFinder(finder), htt.WithPlan(namingPlan{call: tt.call}, "root"))
			require.NoError(t, err)
			if got := buf.String(); got != tt.want {
				t.Errorf("execute(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}
