package template_test

import (
	htmltemplate "html/template"
	"io"
	"testing"
	"text/template/parse"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
	"github.com/typelate/htt/template"
)

// An execution error names the template as it was executed or called, which
// is not always the name of the tree it runs: html/template reports the name
// a tree was added under, and the name the escaper gave a template called in
// a context other than HTML text.
func TestExecutionErrorNamesTheTemplateAsExecuted(t *testing.T) {
	const fails = `<p>{{.X}}</p>`
	tests := []struct {
		name string
		std  func(t *testing.T) error
		htt  func(t *testing.T) error
	}{
		{
			name: "a set's own tree added under another name",
			std: func(t *testing.T) error {
				set := htmltemplate.Must(htmltemplate.New("main").Parse(fails))
				_, err := set.AddParseTree("alias", set.Tree)
				require.NoError(t, err)
				return set.ExecuteTemplate(io.Discard, "alias", "s")
			},
			htt: func(t *testing.T) error {
				set := template.Must(template.New("main").Parse(fails))
				_, err := set.AddParseTree("alias", set.Tree)
				require.NoError(t, err)
				return set.ExecuteTemplate(io.Discard, "alias", "s")
			},
		},
		{
			name: "a text tree added under another name",
			std: func(t *testing.T) error {
				trees, err := parse.Parse("added", fails, "{{", "}}")
				require.NoError(t, err)
				set := htmltemplate.New("main")
				_, err = set.AddParseTree("alias", trees["added"])
				require.NoError(t, err)
				return set.ExecuteTemplate(io.Discard, "alias", "s")
			},
			htt: func(t *testing.T) error {
				tree, err := htt.ParseTree("added", "added", fails, nil, lex.Options{LeftDelim: "{{", RightDelim: "}}"})
				require.NoError(t, err)
				set := template.New("main")
				_, err = set.AddParseTree("alias", tree)
				require.NoError(t, err)
				return set.ExecuteTemplate(io.Discard, "alias", "s")
			},
		},
		{
			name: "a template called inside a script",
			std: func(t *testing.T) error {
				set := htmltemplate.Must(htmltemplate.New("main").Parse(`<script>{{template "t" .}}</script>{{define "t"}}{{.X}}{{end}}`))
				return set.Execute(io.Discard, "s")
			},
			htt: func(t *testing.T) error {
				set := template.Must(template.New("main").Parse(`<script>{{template "t" .}}</script>{{define "t"}}{{.X}}{{end}}`))
				return set.Execute(io.Discard, "s")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.std(t)
			require.Error(t, want)
			got := tt.htt(t)
			require.Error(t, got)
			assert.Equal(t, want.Error(), got.Error())
		})
	}
}
