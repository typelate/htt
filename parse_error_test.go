package htt_test

import (
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

func TestParseErrorEndPosition(t *testing.T) {
	for _, tt := range []struct {
		name       string
		text       string
		start, end token.Position
	}{
		{
			name:  "same line",
			text:  "line one\n  {{ .X | nope }}",
			start: token.Position{Filename: "e.gohtml", Offset: 19, Line: 2, Column: 11},
			end:   token.Position{Filename: "e.gohtml", Offset: 23, Line: 2, Column: 15},
		},
		{
			name:  "first line",
			text:  "{{nope}}",
			start: token.Position{Filename: "e.gohtml", Offset: 2, Line: 1, Column: 3},
			end:   token.Position{Filename: "e.gohtml", Offset: 6, Line: 1, Column: 7},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := htt.ParseTree("e.gohtml", "e.gohtml", tt.text, nil, lex.Options{Functions: map[string]bool{}})

			var parseErr *htt.ParseError
			require.ErrorAs(t, err, &parseErr)
			assert.Equal(t, tt.start, parseErr.Position(), "Position()")
			assert.Equal(t, tt.end, parseErr.EndPosition(), "EndPosition()")
		})
	}
}
