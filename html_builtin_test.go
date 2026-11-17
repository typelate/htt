package htt_test

import (
	"bytes"
	"testing"
	texttemplate "text/template"

	"github.com/stretchr/testify/require"
)

func TestHTMLBuiltinMatchesTextTemplate(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "a\x00b", want: "a�b"},
		{input: `<a href="x">`, want: "&lt;a href=&#34;x&#34;&gt;"},
		{input: "it's", want: "it&#39;s"},
		{input: "&amp;", want: "&amp;amp;"},
		{input: "a+b", want: "a+b"},
		{input: "", want: ""},
	}
	const tmpl = `{{html .}}`
	textTmpl := texttemplate.Must(texttemplate.New("html").Parse(tmpl))
	for _, tc := range tests {
		var buf bytes.Buffer
		require.NoError(t, textTmpl.Execute(&buf, tc.input))
		require.Equalf(t, tc.want, buf.String(), "text/template html(%q)", tc.input)

		got := execText(t, "html", tmpl, tc.input)
		require.Equalf(t, tc.want, got, "html(%q)", tc.input)
	}
}
