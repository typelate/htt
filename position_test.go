package htt_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/typelate/htt/lex"
)

func TestTreePositionAt(t *testing.T) {
	const text = "ab\ncd\n\nef"
	tree := parseText(t, "p.txt", text)
	for _, tt := range []struct {
		name         string
		pos          lex.Pos
		line, column int
	}{
		{name: "start", pos: 0, line: 1, column: 1},
		{name: "in the first line", pos: 1, line: 1, column: 2},
		{name: "the first newline", pos: 2, line: 1, column: 3},
		{name: "start of the second line", pos: 3, line: 2, column: 1},
		{name: "an empty line", pos: 6, line: 3, column: 1},
		{name: "the last byte", pos: 8, line: 4, column: 2},
		{name: "the end", pos: lex.Pos(len(text)), line: 4, column: 3},
		{name: "past the end", pos: lex.Pos(len(text) + 5), line: 4, column: 3},
		{name: "before the start", pos: -1, line: 1, column: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := tree.PositionAt(tt.pos)
			assert.Equal(t, tt.line, got.Line, "PositionAt(%d).Line", tt.pos)
			assert.Equal(t, tt.column, got.Column, "PositionAt(%d).Column", tt.pos)
			assert.Equal(t, "p.txt", got.ParseName)
		})
	}
}
