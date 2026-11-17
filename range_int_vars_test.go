package htt_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRangeIntegerRejectsTwoVariables(t *testing.T) {
	for _, tt := range []struct {
		name string
		exec func(t *testing.T, data any) error
		data any
	}{
		{name: "text int", exec: func(t *testing.T, data any) error {
			return execTextErr(t, "t", `{{range $i, $v := .}}{{$v}}{{end}}`, data)
		}, data: 2},
		{name: "text uint", exec: func(t *testing.T, data any) error {
			return execTextErr(t, "t", `{{range $i, $v := .}}{{$v}}{{end}}`, data)
		}, data: uint(2)},
		{name: "html int", exec: func(t *testing.T, data any) error {
			return execHTMLErr(t, "t", `<p>{{range $i, $v := .}}{{$v}}{{end}}</p>`, data)
		}, data: 2},
		{name: "html uint", exec: func(t *testing.T, data any) error {
			return execHTMLErr(t, "t", `<p>{{range $i, $v := .}}{{$v}}{{end}}</p>`, data)
		}, data: uint(2)},
		{name: "attribute int", exec: func(t *testing.T, data any) error {
			return execHTMLErr(t, "t", `<p {{range $i, $v := .}}hidden{{end}}></p>`, data)
		}, data: 2},
		{name: "attribute uint", exec: func(t *testing.T, data any) error {
			return execHTMLErr(t, "t", `<p {{range $i, $v := .}}hidden{{end}}></p>`, data)
		}, data: uint(2)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.exec(t, tt.data)

			require.ErrorContains(t, err, "can't use 2 to iterate over more than one variable")
		})
	}
}
