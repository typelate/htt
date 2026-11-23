package template_test

import (
	htmltemplate "html/template"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/htt/template"
)

// The error for an action in an ambiguous part of a URL names the action as
// html/template writes it, whatever spacing or trim markers the source has.
func TestAmbiguousURLErrorNamesTheActionAsHTMLTemplateDoes(t *testing.T) {
	for _, text := range []string{
		`<a href="{{if .C}}/path/{{else}}/search?q={{end}}{{.X}}">`,
		`<a href="{{if .C}}/path/{{else}}/search?q={{end}}{{ .X }}">`,
		`<a href="{{if .C}}/path/{{else}}/search?q={{end}}{{-   .X  |  print   -}}">`,
	} {
		t.Run(text, func(t *testing.T) {
			std := htmltemplate.Must(htmltemplate.New("z").Parse(text))
			want := std.Execute(io.Discard, nil)
			require.Error(t, want)

			tmpl := template.Must(template.New("z").Parse(text))
			got := tmpl.Execute(io.Discard, nil)
			require.Error(t, got)
			assert.Equal(t, want.Error(), got.Error())
		})
	}
}
