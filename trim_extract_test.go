package htt_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

// TestExtractedBlockHonorsOwnTrimMarkers is a regression test: a
// {{block "name" pipeline -}} ... {{- end}} clause's own trim markers must
// still apply once the block's body is pulled out into its own Tree — the
// shape every muxt-generated route handler executes, since
// ExecuteTemplate always operates on the extracted tree, never on the
// {{block}} node in place.
//
// blockControl (parse.go) registers the block's body into a Tree it puts
// in the parser's treeSet, which is the tree both real execution
// (template/template.go's Parse merges treeSet into the running set) and
// static analysis (check/internal/asteval's loader) end up using. That
// Tree's RootTrimFirst/RootTrimLast must carry the block's own
// StartTrimRight/EndTrimLeft: once extracted, the body has no siblings —
// nothing else could ever apply the block's own delimiter trim markers to
// its own leading/trailing whitespace.
func TestExtractedBlockHonorsOwnTrimMarkers(t *testing.T) {
	const src = `{{block "gimme" . -}}
<a href="/x">hi</a>
{{- end}}`
	treeSet := make(map[string]*htt.Tree)
	tree, err := htt.ParseTree("t", "t", src, treeSet, lex.Options{
		LeftDelim:  "{{",
		RightDelim: "}}",
		ParseHTML:  true,
	})
	require.NoError(t, err)
	htt.AnnotateHTMLContexts(tree.Root)

	gimme, ok := treeSet["gimme"]
	require.True(t, ok, "block %q registered in treeSet", "gimme")
	htt.AnnotateHTMLContexts(gimme.Root)

	var buf bytes.Buffer
	require.NoError(t, htt.ExecuteTemplate(&buf, gimme, nil))
	assert.Equal(t, `<a href="/x">hi</a>`, buf.String())
}

// TestExtractedDefineHonorsOwnTrimMarkers is TestExtractedBlockHonorsOwnTrimMarkers's
// sibling for {{define "name" -}} ... {{- end}}: htt.Extract, not
// blockControl, is what carries DefineNode's StartTrimRight/EndTrimLeft
// onto the standalone Tree it builds from the define's Body.
func TestExtractedDefineHonorsOwnTrimMarkers(t *testing.T) {
	const src = `{{define "gimme" -}}
<a href="/x">hi</a>
{{- end}}`
	tree, err := htt.ParseTree("t", "t", src, nil, lex.Options{
		LeftDelim:  "{{",
		RightDelim: "}}",
		ParseHTML:  true,
	})
	require.NoError(t, err)
	htt.AnnotateHTMLContexts(tree.Root)

	var gimme *htt.Tree
	for name, dt := range htt.Extract(tree) {
		if name == "gimme" {
			gimme = dt
		}
	}
	require.NotNil(t, gimme, "define %q extracted", "gimme")
	htt.AnnotateHTMLContexts(gimme.Root)

	var buf bytes.Buffer
	require.NoError(t, htt.ExecuteTemplate(&buf, gimme, nil))
	assert.Equal(t, `<a href="/x">hi</a>`, buf.String())
}
