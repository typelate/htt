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

// Action calls f.
func (f planFunc) Action(instance string, n *htt.ActionNode) (htt.ActionEdit, bool) {
	return f(instance, n)
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
