package htt_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

func TestDefinedTreesOfAnHTMLParseEscape(t *testing.T) {
	const text = `{{define "d"}}{{.}}{{end}}{{block "b" .}}{{.}}{{end}}`
	for _, tt := range []struct {
		name    string
		options lex.Options
		want    string
	}{
		{name: "html", options: lex.NewOptions(), want: "&lt;i&gt;"},
		{name: "text", options: lex.Options{}, want: "<i>"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			treeSet := make(map[string]*htt.Tree)
			root, err := htt.ParseTree("t", "t", text, treeSet, tt.options)
			require.NoError(t, err)

			for _, name := range []string{"d", "b"} {
				tree := htt.DefinedTrees(root, treeSet)[name]
				require.NotNil(t, tree, name)
				var out strings.Builder
				require.NoError(t, htt.ExecuteTemplate(&out, tree, "<i>"), name)
				assert.Equal(t, tt.want, out.String(), name)
			}
		})
	}
}

func TestExtractedTreesOfAnHTMLParseEscape(t *testing.T) {
	root, err := htt.ParseTree("t", "t", `{{define "d"}}{{.}}{{end}}{{block "b" .}}{{.}}{{end}}`, nil, lex.NewOptions())
	require.NoError(t, err)

	for name, tree := range htt.Extract(root) {
		var out strings.Builder
		require.NoError(t, htt.ExecuteTemplate(&out, tree, "<i>"), name)
		assert.Equal(t, "&lt;i&gt;", out.String(), name)
	}
}
