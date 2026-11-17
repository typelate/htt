package template_test

import (
	"bytes"
	htmltemplate "html/template"
	"testing"
	"text/template/parse"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
	"github.com/typelate/htt/template"
)

func TestAddParseTreeMatchesHTMLTemplate(t *testing.T) {
	const value = `<b>'+"x"/&`
	tests := []struct {
		name string
		text string
	}{
		{"element text", `<p>{{.}}</p>`},
		{"url attribute", `<a href="/x?q={{.}}">l</a>`},
		{"script", `<script>var x = {{.}};</script>`},
		{"attribute and event handler", `<a title="{{.}}" onclick="f('{{.}}')">l</a>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := htmlTemplateAdded(t, tt.text, value)

			tree, err := htt.ParseTree("added", "added", tt.text, nil, lex.Options{
				LeftDelim:  "{{",
				RightDelim: "}}",
			})
			require.NoError(t, err)
			tmpl := template.New("main")
			_, err = tmpl.AddParseTree("added", tree)
			require.NoError(t, err)

			var buf bytes.Buffer
			require.NoError(t, tmpl.ExecuteTemplate(&buf, "added", value))
			assert.Equal(t, want, buf.String())
		})
	}
}

func TestAddParseTreeCalledFromParsedTemplate(t *testing.T) {
	const value = `<b>'+"x"/&`
	const mainText = `<div title="{{template "added" .}}">{{template "added" .}}</div>`
	const addedText = `[{{.}}]`

	htmlSet := htmltemplate.New("main")
	addedHTML := parseTextTree(t, "added", addedText)
	_, err := htmlSet.AddParseTree("added", addedHTML)
	require.NoError(t, err)
	_, err = htmlSet.Parse(mainText)
	require.NoError(t, err)
	var want bytes.Buffer
	require.NoError(t, htmlSet.ExecuteTemplate(&want, "main", value))

	tmpl := template.New("main")
	added, err := htt.ParseTree("added", "added", addedText, nil, lex.Options{LeftDelim: "{{", RightDelim: "}}"})
	require.NoError(t, err)
	_, err = tmpl.AddParseTree("added", added)
	require.NoError(t, err)
	_, err = tmpl.Parse(mainText)
	require.NoError(t, err)
	var buf bytes.Buffer
	require.NoError(t, tmpl.ExecuteTemplate(&buf, "main", value))

	assert.Equal(t, want.String(), buf.String())
}

func TestAddParseTreeRefusesAnHTMLTree(t *testing.T) {
	tree, err := htt.ParseTree("page", "page", `<p>{{.}}</p>`, nil, lex.Options{
		LeftDelim:  "{{",
		RightDelim: "}}",
		ParseHTML:  true,
	})
	require.NoError(t, err)

	tmpl := template.New("main")
	_, err = tmpl.AddParseTree("page", tree)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsed without HTML structure")
	assert.Nil(t, tmpl.Lookup("page"), "the set is unchanged")
}

// htmlTemplateAdded returns html/template's output for the text added as
// the template "added" with AddParseTree, executed with value.
func htmlTemplateAdded(t *testing.T, text string, value any) string {
	t.Helper()
	trees, err := parse.Parse("added", text, "{{", "}}")
	require.NoError(t, err)
	set := htmltemplate.New("main")
	_, err = set.AddParseTree("added", trees["added"])
	require.NoError(t, err)
	var buf bytes.Buffer
	require.NoError(t, set.ExecuteTemplate(&buf, "added", value))
	return buf.String()
}

// parseTextTree returns the text/template tree of text, named name.
func parseTextTree(t *testing.T, name, text string) *parse.Tree {
	t.Helper()
	trees, err := parse.Parse(name, text, "{{", "}}")
	require.NoError(t, err)
	return trees[name]
}
