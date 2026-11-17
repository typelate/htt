package htt_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestRangeSendOnlyChanIsAnError(t *testing.T) {
	for _, tt := range []struct {
		name string
		exec func(t *testing.T, data any) error
	}{
		{name: "text", exec: func(t *testing.T, data any) error { return execTextErr(t, "t", `{{range .}}x{{end}}`, data) }},
		{name: "html", exec: func(t *testing.T, data any) error { return execHTMLErr(t, "t", `{{range .}}x{{end}}`, data) }},
		{name: "attribute", exec: func(t *testing.T, data any) error {
			return execHTMLErr(t, "t", `<p {{range .}}hidden{{end}}></p>`, data)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.exec(t, make(chan<- int))

			require.ErrorContains(t, err, "range over send-only channel")
		})
	}
}
