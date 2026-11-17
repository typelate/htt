package htt_test

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

func TestCoverProfileKeepsFilesApart(t *testing.T) {
	cp := new(htt.CoverProfile)
	for _, name := range []string{"a.gohtml", "b.gohtml"} {
		tree, err := htt.ParseTree(name, name, "<p>hi</p>", nil, lex.NewOptions())
		require.NoError(t, err)
		require.NoError(t, htt.ExecuteTemplate(io.Discard, tree, nil, htt.WithCoverProfile(cp)))
	}

	var profile strings.Builder
	_, err := cp.WriteTo(&profile)

	require.NoError(t, err)
	assert.Equal(t, "mode: set\na.gohtml:1.1,1.10 4 1\nb.gohtml:1.1,1.10 4 1\n", profile.String())
}

func TestCoverProfileUnclosedElementHasNoEndTagBlock(t *testing.T) {
	cp := new(htt.CoverProfile)
	tree, err := htt.ParseTree("u.gohtml", "u.gohtml", "text before\n<main>\n  body\n", nil, lex.NewOptions())
	require.NoError(t, err)
	require.NoError(t, htt.ExecuteTemplate(io.Discard, tree, nil, htt.WithCoverProfile(cp)))

	var profile strings.Builder
	_, err = cp.WriteNewlyCovered(&profile, map[string]struct{}{})

	require.NoError(t, err)
	assert.Equal(t, strings.Join([]string{
		"u.gohtml:1.1,2.1 1 1", // text before
		"u.gohtml:2.1,2.6 1 1", // <main
		"u.gohtml:2.6,2.7 1 1", // >
		"u.gohtml:2.7,4.1 1 1", // body
		"",
	}, "\n"), profile.String())
}
