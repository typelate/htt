package htt_test

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

// scriptHeavyTree parses testdata/script_heavy.gohtml followed by extra
// lines of markup, and returns the tree with the length of its source.
func scriptHeavyTree(b *testing.B, extraLines int) (*htt.Tree, int) {
	b.Helper()
	text, err := os.ReadFile("testdata/script_heavy.gohtml")
	require.NoError(b, err)
	source := string(text) + strings.Repeat("<p>{{.}}</p>\n", extraLines)
	tree, err := htt.ParseTree("script_heavy.gohtml", "script_heavy.gohtml", source, nil, lex.NewOptions())
	require.NoError(b, err)
	return tree, len(source)
}

var benchmarkExtraLines = []int{0, 10_000}

func BenchmarkTreePositionAt(b *testing.B) {
	for _, extra := range benchmarkExtraLines {
		b.Run(fmt.Sprintf("extra_lines=%d", extra), func(b *testing.B) {
			tree, size := scriptHeavyTree(b, extra)
			b.ResetTimer()
			for i := 0; b.Loop(); i++ {
				tree.PositionAt(lex.Pos(i * 7919 % size))
			}
		})
	}
}

func BenchmarkCoverProfileWriteTo(b *testing.B) {
	for _, extra := range benchmarkExtraLines {
		b.Run(fmt.Sprintf("extra_lines=%d", extra), func(b *testing.B) {
			tree, _ := scriptHeavyTree(b, extra)
			var profile htt.CoverProfile
			profile.AddTree(tree)
			b.ResetTimer()
			for b.Loop() {
				_, err := profile.WriteTo(io.Discard)
				require.NoError(b, err)
			}
		})
	}
}
