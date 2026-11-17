package htt_test

import (
	"testing"
	"text/template"
	"text/template/parse"

	"github.com/stretchr/testify/require"

	"github.com/typelate/htt"
)

// stdPositions collects, in source order, the position text/template records
// for each template call, break and continue under n.
func stdPositions(n parse.Node, out *[]int) {
	switch n := n.(type) {
	case *parse.ListNode:
		if n == nil {
			return
		}
		for _, c := range n.Nodes {
			stdPositions(c, out)
		}
	case *parse.TemplateNode:
		*out = append(*out, int(n.Position()))
	case *parse.BreakNode:
		*out = append(*out, int(n.Position()))
	case *parse.ContinueNode:
		*out = append(*out, int(n.Position()))
	case *parse.IfNode:
		stdPositions(n.List, out)
		stdPositions(n.ElseList, out)
	case *parse.RangeNode:
		stdPositions(n.List, out)
		stdPositions(n.ElseList, out)
	case *parse.WithNode:
		stdPositions(n.List, out)
		stdPositions(n.ElseList, out)
	}
}

// httPositions collects NameStart or KeywordStart for each node htt.Walk visits,
// in source order. A block contributes the offset of its call's name.
func httPositions(t *testing.T, tree *htt.Tree) []int {
	t.Helper()
	var got []int
	htt.Walk(tree, func(_ *htt.Tree, n htt.Node) {
		switch n := n.(type) {
		case *htt.TemplateNode:
			got = append(got, int(n.NameStart()))
		case *htt.BlockNode:
			got = append(got, int(n.Call().NameStart()))
		case *htt.BreakNode:
			got = append(got, int(n.KeywordStart()))
		case *htt.ContinueNode:
			got = append(got, int(n.KeywordStart()))
		}
	})
	return got
}

func TestNameAndKeywordOffsetsMatchTextTemplate(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{name: "template", src: `{{template "foo"}}`},
		{name: "template spaces and pipe", src: `{{  template   "foo" . }}`},
		{name: "template trimmed", src: `{{- template "foo" -}}`},
		{name: "template on line three", src: "a\nb\n{{template \"foo\" .}}\n"},
		{name: "break", src: `{{range .}}{{break}}{{end}}`},
		{name: "continue and break", src: `{{range .}}{{if .}}{{- continue -}}{{end}}{{ break }}{{end}}`},
		{name: "raw string name", src: "{{template `foo`}}"},
		{name: "block", src: `{{block "b" .}}x{{end}}`},
		{name: "template in tag", src: `<p {{template "foo"}}>`},
		{name: "template in attribute", src: `<a title="{{template "foo"}}">x</a>`},
	}
	for _, tc := range tests {
		t.Run(tc.name+"/text", func(t *testing.T) {
			std, err := template.New("z").Parse(tc.src)
			require.NoError(t, err)
			var want []int
			stdPositions(std.Tree.Root, &want)

			tree := parseText(t, "z", tc.src)
			require.Equal(t, want, httPositions(t, tree), "text mode offsets for %q", tc.src)
		})
		t.Run(tc.name+"/html", func(t *testing.T) {
			std, err := template.New("z").Parse(tc.src)
			require.NoError(t, err)
			var want []int
			stdPositions(std.Tree.Root, &want)

			tree := parseHTML(t, "z", tc.src)
			require.Equal(t, want, httPositions(t, tree), "html mode offsets for %q", tc.src)
		})
	}
}
