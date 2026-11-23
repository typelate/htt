package htt_test

import (
	"testing"
	"text/template/parse"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

func TestTreeErrorContextMatchesTextTemplate(t *testing.T) {
	for _, text := range []string{
		`{{.X}}`,
		`{{ .X }}`,
		"a\nbc {{-   .X   |  printf   \"%s\"  -}} d",
		"\n\n  {{  $x  :=  .A.B  }}",
		`{{ index  .M  "k" }}`,
	} {
		t.Run(text, func(t *testing.T) {
			std := parse.New("t")
			std.Mode = parse.SkipFuncCheck
			if _, err := std.Parse(text, "{{", "}}", make(map[string]*parse.Tree)); err != nil {
				t.Fatal(err)
			}
			var stdAction *parse.ActionNode
			for _, n := range std.Root.Nodes {
				if a, ok := n.(*parse.ActionNode); ok {
					stdAction = a
					break
				}
			}
			if stdAction == nil {
				t.Fatal("text/template parsed no action")
			}
			wantLocation, wantContext := std.ErrorContext(stdAction)

			tree, err := htt.ParseTree("t", "t", text, nil, lex.Options{LeftDelim: "{{", RightDelim: "}}"})
			if err != nil {
				t.Fatal(err)
			}
			var action *htt.ActionNode
			for _, n := range tree.Root.Nodes {
				if a, ok := n.(*htt.ActionNode); ok {
					action = a
					break
				}
			}
			if action == nil {
				t.Fatal("htt parsed no action")
			}
			location, context := tree.ErrorContext(action)
			if location != wantLocation || context != wantContext {
				t.Errorf("ErrorContext = %q, %q, want %q, %q", location, context, wantLocation, wantContext)
			}
		})
	}
}
