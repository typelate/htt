package htt_test

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

func TestMissingKeyModeReachesCalledTemplates(t *testing.T) {
	const text = `{{define "inner"}}{{.missing}}{{end}}{{template "inner" .}}`
	for _, tt := range []struct {
		name    string
		options lex.Options
	}{
		{name: "text", options: lex.Options{}},
		{name: "html", options: lex.NewOptions()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			treeSet := make(map[string]*htt.Tree)
			root, err := htt.ParseTree("t", "t", text, treeSet, tt.options)
			require.NoError(t, err)
			trees := htt.DefinedTrees(root, treeSet)
			finder := htt.FindTreeFunc(func(name string) (*htt.Tree, bool) {
				tree, ok := trees[name]
				return tree, ok
			})

			err = htt.ExecuteTemplate(io.Discard, root, map[string]any{},
				htt.WithFinder(finder), htt.WithMissingKey(htt.MissingKeyError))

			require.ErrorContains(t, err, `map has no entry for key "missing"`)
		})
	}
}
