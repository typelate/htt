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
