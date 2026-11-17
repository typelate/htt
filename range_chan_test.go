package htt_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRangeChanBreakOnFirstElementSkipsElse(t *testing.T) {
	const text = `{{range .}}{{break}}{{else}}EMPTY{{end}}`
	for _, tt := range []struct {
		name string
		exec func(t *testing.T, data any) string
	}{
		{name: "text", exec: func(t *testing.T, data any) string { return execText(t, "t", text, data) }},
		{name: "html", exec: func(t *testing.T, data any) string { return execHTML(t, "t", text, data) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ch := make(chan int, 2)
			ch <- 1
			ch <- 2
			close(ch)

			got := tt.exec(t, ch)

			assert.Empty(t, got)
		})
	}
}
