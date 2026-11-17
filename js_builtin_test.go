package htt_test

import (
	"bytes"
	"testing"
	texttemplate "text/template"

	"github.com/stretchr/testify/require"
)

func TestJSBuiltinMatchesTextTemplate(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "It'd be nice.", want: `It\'d be nice.`},
		{input: "<html>", want: "\\u003Chtml\\u003E"},
		{input: "no = in attributes", want: "no \\u003D in attributes"},
		{input: "&#x27; does not become HTML entity", want: "\\u0026#x27; does not become HTML entity"},
		{input: "a\nb\tc", want: `a\u000Ab\u0009c`},
		{input: `say "hi"`, want: `say \"hi\"`},
		{input: "a\x00b", want: `a\u0000b`},
	}
	const tmpl = `{{js .}}`
	textTmpl := texttemplate.Must(texttemplate.New("js").Parse(tmpl))
	for _, tc := range tests {
		var buf bytes.Buffer
		require.NoError(t, textTmpl.Execute(&buf, tc.input))
		require.Equalf(t, tc.want, buf.String(), "text/template js(%q)", tc.input)

		got := execText(t, "js", tmpl, tc.input)
		require.Equalf(t, tc.want, got, "js(%q)", tc.input)
	}
}
