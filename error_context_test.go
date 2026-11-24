package htt_test

import (
	"fmt"
	"testing"
	"text/template/parse"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

// For every node of a tree, ErrorContext returns the location and the text
// that text/template/parse's ErrorContext returns for the same node of the
// same source.
func TestTreeErrorContextMatchesTextTemplate(t *testing.T) {
	for _, text := range []string{
		`{{.X}}`,
		`{{ .X }}`,
		"a\nbc{{-   .X   |  printf   \"%s\"  -}}d",
		"\n\n  {{  $x  :=  .A.B  }}{{  $x  =  1  }}",
		`{{ index  .M  "k" }}`,
		`{{ if  .A }}x{{ else }}{{ if .B }}y{{ else }}z{{ end }}{{ end }}`,
		"{{range  $i , $e := .L }}\n {{ $e }}{{ break }}{{ continue }}{{ else }}n{{ end }}",
		`{{ with  $x := .M.k }}{{ $x.y }}{{ else }}{{ . }}{{ end }}{{ template  "t"  .X }}{{ template "u" }}`,
		"x{{/* a comment */}}\ny{{- /* trimmed */ -}}z",
		`{{ print  nil true 'a' 0x10 1.5 "s" ` + "`raw`" + ` .F.G $.H (len .L) (.M).k  printf }}`,
	} {
		t.Run(text, func(t *testing.T) {
			std := parse.New("t")
			std.Mode = parse.SkipFuncCheck | parse.ParseComments
			if _, err := std.Parse(text, "{{", "}}", make(map[string]*parse.Tree)); err != nil {
				t.Fatal(err)
			}
			tree, err := htt.ParseTree("t", "t", text, nil, lex.Options{
				LeftDelim: "{{", RightDelim: "}}", EmitComment: true, BreakOK: true, ContinueOK: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			want := stdErrorContexts(std, std.Root, nil)
			got := httErrorContexts(tree, tree.Root, nil)
			for i := 0; i < len(want) || i < len(got); i++ {
				var w, g string
				if i < len(want) {
					w = want[i]
				}
				if i < len(got) {
					g = got[i]
				}
				if g != w {
					t.Errorf("node %d:\n  htt:           %s\n  text/template: %s", i, g, w)
				}
			}
		})
	}
}

// The tree keeps what the source has, so two results are not
// text/template's: text next to a trim marker keeps its white space, and
// the list of an {{else if}} is located at {{else.
func TestTreeErrorContextKeepsTheShapeOfTheSource(t *testing.T) {
	for _, tt := range []struct {
		name, text string
		node       int // index into the lines of the walk
		want, std  string
	}{
		{
			name: "text before a trim marker", text: "a {{- .X -}} b", node: 1,
			want: `TextNode t:1:0 "a "`, std: `TextNode t:1:0 "a"`,
		},
		{
			name: "text after a trim marker", text: "a {{- .X -}} b", node: 6,
			want: `TextNode t:1:12 " b"`, std: `TextNode t:1:13 "b"`,
		},
		{
			name: "the list of an else if", text: "{{if .A}}x{{else if .B}}y{{end}}", node: 7,
			want: `ListNode t:1:10 "{{if .B}}y{{end}}"`, std: `ListNode t:1:17 "{{if .B}}y{{end}}"`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			std := parse.New("t")
			std.Mode = parse.SkipFuncCheck
			if _, err := std.Parse(tt.text, "{{", "}}", make(map[string]*parse.Tree)); err != nil {
				t.Fatal(err)
			}
			tree, err := htt.ParseTree("t", "t", tt.text, nil, lex.Options{LeftDelim: "{{", RightDelim: "}}"})
			if err != nil {
				t.Fatal(err)
			}
			if got := stdErrorContexts(std, std.Root, nil)[tt.node]; got != tt.std {
				t.Errorf("text/template gives %s, and the test expects %s", got, tt.std)
			}
			if got := httErrorContexts(tree, tree.Root, nil)[tt.node]; got != tt.want {
				t.Errorf("ErrorContext gives %s, want %s", got, tt.want)
			}
		})
	}
}

// stdErrorContexts appends a line for n and for each node under it, in
// source order: the node's kind, its location and its text.
func stdErrorContexts(tree *parse.Tree, n parse.Node, lines []string) []string {
	location, context := tree.ErrorContext(n)
	lines = append(lines, fmt.Sprintf("%s %s %q", kindName(n), location, context))
	var children []parse.Node
	switch n := n.(type) {
	case *parse.ListNode:
		children = n.Nodes
	case *parse.ActionNode:
		children = append(children, n.Pipe)
	case *parse.PipeNode:
		for _, v := range n.Decl {
			children = append(children, v)
		}
		for _, c := range n.Cmds {
			children = append(children, c)
		}
	case *parse.CommandNode:
		children = n.Args
	case *parse.ChainNode:
		children = append(children, n.Node)
	case *parse.IfNode:
		children = stdBranchChildren(&n.BranchNode)
	case *parse.RangeNode:
		children = stdBranchChildren(&n.BranchNode)
	case *parse.WithNode:
		children = stdBranchChildren(&n.BranchNode)
	case *parse.TemplateNode:
		if n.Pipe != nil {
			children = append(children, n.Pipe)
		}
	}
	for _, child := range children {
		lines = stdErrorContexts(tree, child, lines)
	}
	return lines
}

func stdBranchChildren(b *parse.BranchNode) []parse.Node {
	children := []parse.Node{b.Pipe, b.List}
	if b.ElseList != nil {
		children = append(children, b.ElseList)
	}
	return children
}

// httErrorContexts is stdErrorContexts for a tree of this package.
func httErrorContexts(tree *htt.Tree, n htt.Node, lines []string) []string {
	location, context := tree.ErrorContext(n)
	lines = append(lines, fmt.Sprintf("%s %s %q", kindName(n), location, context))
	var children []htt.Node
	switch n := n.(type) {
	case *htt.ListNode:
		children = n.Nodes
	case *htt.ActionNode:
		children = append(children, n.Pipe)
	case *htt.PipeNode:
		for _, v := range n.Decl {
			children = append(children, v)
		}
		for _, c := range n.Commands {
			children = append(children, c)
		}
	case *htt.CommandNode:
		children = n.Args
	case *htt.ChainNode:
		children = append(children, n.Node)
	case *htt.IfNode:
		children = httBranchChildren(&n.BranchNode)
	case *htt.RangeNode:
		children = httBranchChildren(&n.BranchNode)
	case *htt.WithNode:
		children = httBranchChildren(&n.BranchNode)
	case *htt.TemplateNode:
		if n.Pipe != nil {
			children = append(children, n.Pipe)
		}
	}
	for _, child := range children {
		lines = httErrorContexts(tree, child, lines)
	}
	return lines
}

func httBranchChildren(b *htt.BranchNode) []htt.Node {
	children := []htt.Node{b.Pipe, b.List}
	if b.ElseList != nil {
		children = append(children, b.ElseList)
	}
	return children
}

// kindName returns the name of n's type without its package, which is the
// same in both packages for the same kind of node.
func kindName(n any) string {
	name := fmt.Sprintf("%T", n)
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '.' {
			return name[i+1:]
		}
	}
	return name
}
