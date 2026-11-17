package htt_test

import (
	"bytes"
	"testing"
	texttemplate "text/template"

	"github.com/stretchr/testify/require"
)

func TestURLQueryBuiltinMatchesTextTemplate(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "a b", want: "a+b"},
		{input: "a+b", want: "a%2Bb"},
		{input: "a&b=c", want: "a%26b%3Dc"},
		{input: "http://example.com/x?y=1", want: "http%3A%2F%2Fexample.com%2Fx%3Fy%3D1"},
		{input: "café", want: "caf%C3%A9"},
		{input: "", want: ""},
	}
	const tmpl = `{{urlquery .}}`
	textTmpl := texttemplate.Must(texttemplate.New("urlquery").Parse(tmpl))
	for _, tc := range tests {
		var buf bytes.Buffer
		require.NoError(t, textTmpl.Execute(&buf, tc.input))
		require.Equalf(t, tc.want, buf.String(), "text/template urlquery(%q)", tc.input)

		got := execText(t, "urlquery", tmpl, tc.input)
		require.Equalf(t, tc.want, got, "urlquery(%q)", tc.input)
	}
}
