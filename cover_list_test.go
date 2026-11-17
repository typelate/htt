package htt_test

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

func TestTreeCoverBlocksBeforeItRuns(t *testing.T) {
	tree, err := htt.ParseTree("page.gohtml", "page.gohtml",
		`<p class="x">{{.Name}}{{if .OK}}yes{{else}}no{{end}}</p>`, nil, lex.NewOptions())
	require.NoError(t, err)

	blocks := tree.CoverBlocks()
	require.NotEmpty(t, blocks)
	for i, b := range blocks {
		assert.Equal(t, "page.gohtml", b.ParseName, "CoverBlocks()[%d].ParseName", i)
		assert.Equal(t, int64(0), b.Count, "CoverBlocks()[%d].Count", i)
		if i > 0 {
			prev := blocks[i-1]
			assert.LessOrEqual(t, prev.StartLine, b.StartLine, "CoverBlocks()[%d] starts before CoverBlocks()[%d]", i, i-1)
			if prev.StartLine == b.StartLine {
				assert.LessOrEqual(t, prev.StartColumn, b.StartColumn, "CoverBlocks()[%d] starts before CoverBlocks()[%d]", i, i-1)
			}
		}
	}
}

func TestTreeCoverBlocksMatchTheProfile(t *testing.T) {
	t.Run("every block runs", func(t *testing.T) {
		tree, err := htt.ParseTree("all.gohtml", "all.gohtml", `<p class="x">{{.}}</p>`, nil, lex.NewOptions())
		require.NoError(t, err)
		cp := new(htt.CoverProfile)
		require.NoError(t, htt.ExecuteTemplate(io.Discard, tree, "hi", htt.WithCoverProfile(cp)))

		written := map[string]struct{}{}
		_, err = cp.WriteNewlyCovered(io.Discard, written)
		require.NoError(t, err)

		var want []string
		for k := range written {
			want = append(want, k)
		}
		var got []string
		for _, b := range tree.CoverBlocks() {
			got = append(got, b.String())
		}
		assert.ElementsMatch(t, want, got)
	})

	t.Run("else branch does not run", func(t *testing.T) {
		tree, err := htt.ParseTree("branch.gohtml", "branch.gohtml",
			`{{if .}}yes{{else}}no{{end}}`, nil, lex.NewOptions())
		require.NoError(t, err)
		cp := new(htt.CoverProfile)
		require.NoError(t, htt.ExecuteTemplate(io.Discard, tree, true, htt.WithCoverProfile(cp)))

		written := map[string]struct{}{}
		_, err = cp.WriteNewlyCovered(io.Discard, written)
		require.NoError(t, err)

		var ran []string
		var unran int
		for _, b := range tree.CoverBlocks() {
			if b.Count > 0 {
				ran = append(ran, b.String())
			} else {
				unran++
			}
		}
		var want []string
		for k := range written {
			want = append(want, k)
		}
		assert.ElementsMatch(t, want, ran)
		assert.Greater(t, unran, 0, "CoverBlocks() has no block with Count 0")
	})
}

func TestTreeCoverBlocksWithoutAParseName(t *testing.T) {
	tree, err := htt.ParseTree("x.gohtml", "x.gohtml", `<p>{{.}}</p>`, nil, lex.NewOptions())
	require.NoError(t, err)
	tree.ParseName = "" // what the template package sets for template.New("")

	blocks := tree.CoverBlocks()
	assert.Empty(t, blocks, "CoverBlocks() = %d blocks, want none", len(blocks))
}
