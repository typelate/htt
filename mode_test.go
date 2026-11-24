package htt_test

import (
	"testing"
	"text/template/parse"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

func TestModeFlagsAreTextTemplates(t *testing.T) {
	if got, want := uint(htt.ParseComments), uint(parse.ParseComments); got != want {
		t.Errorf("ParseComments = %d, want %d", got, want)
	}
	if got, want := uint(htt.SkipFuncCheck), uint(parse.SkipFuncCheck); got != want {
		t.Errorf("SkipFuncCheck = %d, want %d", got, want)
	}
}

func TestParseTreeRecordsItsMode(t *testing.T) {
	const text = `{{/* c */}}{{define "d"}}{{.}}{{end}}{{block "b" .}}{{.}}{{end}}`
	for _, tt := range []struct {
		name    string
		options lex.Options
		want    htt.Mode
	}{
		{"no comments, no function list", lex.Options{}, htt.SkipFuncCheck},
		{"comments, no function list", lex.Options{EmitComment: true}, htt.ParseComments | htt.SkipFuncCheck},
		{"comments and a function list", lex.Options{EmitComment: true, Functions: map[string]bool{"f": true}}, htt.ParseComments},
		{"an empty function list", lex.Options{Functions: map[string]bool{}}, 0},
		{"HTML structure", lex.Options{EmitComment: true, ParseHTML: true}, htt.ParseComments | htt.SkipFuncCheck},
	} {
		t.Run(tt.name, func(t *testing.T) {
			treeSet := make(map[string]*htt.Tree)
			tree, err := htt.ParseTree("t", "t", text, treeSet, tt.options)
			if err != nil {
				t.Fatal(err)
			}
			if tree.Mode != tt.want {
				t.Errorf("Mode = %b, want %b", tree.Mode, tt.want)
			}
			for name, defined := range htt.DefinedTrees(tree, treeSet) {
				if defined.Mode != tt.want {
					t.Errorf("Mode of %q = %b, want %b", name, defined.Mode, tt.want)
				}
			}
		})
	}
}
