package template_test

import (
	"bytes"
	htmltemplate "html/template"
	"io"
	"testing"
	"testing/fstest"
	"text/template/parse"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
	"github.com/typelate/htt/template"
)

const afterExecuteText = `<p>{{.}}</p>{{define "d"}}<i>{{.}}</i>{{end}}`

const afterExecuteFailingText = `<a href="x{{if .}}"{{end}}>x</a>`

const afterExecuteValue = "<script>alert(1)</script>"

// frozenFirstRun is the first execution of a set, on both packages.
type frozenFirstRun struct {
	name    string
	text    string
	html    func(*htmltemplate.Template) error
	ours    func(*template.Template) error
	wantErr bool
}

var frozenFirstRuns = []frozenFirstRun{
	{
		name: "Execute",
		text: afterExecuteText,
		html: func(s *htmltemplate.Template) error { return s.Execute(io.Discard, afterExecuteValue) },
		ours: func(s *template.Template) error { return s.Execute(io.Discard, afterExecuteValue) },
	},
	{
		name: "ExecuteTemplate",
		text: afterExecuteText,
		html: func(s *htmltemplate.Template) error { return s.ExecuteTemplate(io.Discard, "d", afterExecuteValue) },
		ours: func(s *template.Template) error { return s.ExecuteTemplate(io.Discard, "d", afterExecuteValue) },
	},
	{
		name:    "Execute that fails in the escaper",
		text:    afterExecuteFailingText,
		html:    func(s *htmltemplate.Template) error { return s.Execute(io.Discard, afterExecuteValue) },
		ours:    func(s *template.Template) error { return s.Execute(io.Discard, afterExecuteValue) },
		wantErr: true,
	},
}

// frozenOperation is a call that html/template refuses after an execution.
type frozenOperation struct {
	name string
	html func(*htmltemplate.Template) error
	ours func(*template.Template) error
}

func frozenOperations() []frozenOperation {
	const addText = `<b>{{.}}</b>`
	return []frozenOperation{
		{
			name: "Parse",
			html: func(s *htmltemplate.Template) error { _, err := s.Parse(addText); return err },
			ours: func(s *template.Template) error { _, err := s.Parse(addText); return err },
		},
		{
			name: "New then Parse",
			html: func(s *htmltemplate.Template) error { _, err := s.New("other").Parse(addText); return err },
			ours: func(s *template.Template) error { _, err := s.New("other").Parse(addText); return err },
		},
		{
			name: "Lookup then Parse",
			html: func(s *htmltemplate.Template) error { _, err := s.Lookup("page").Parse(addText); return err },
			ours: func(s *template.Template) error { _, err := s.Lookup("page").Parse(addText); return err },
		},
		{
			name: "AddParseTree",
			html: func(s *htmltemplate.Template) error {
				trees, err := parse.Parse("added", addText, "{{", "}}")
				if err != nil {
					return err
				}
				_, err = s.AddParseTree("added", trees["added"])
				return err
			},
			ours: func(s *template.Template) error {
				tree, err := htt.ParseTree("added", "added", addText, nil, lex.Options{
					LeftDelim:  "{{",
					RightDelim: "}}",
				})
				if err != nil {
					return err
				}
				_, err = s.AddParseTree("added", tree)
				return err
			},
		},
		{
			name: "ParseFiles",
			html: func(s *htmltemplate.Template) error {
				_, err := s.ParseFiles("testdata/error_location.gohtml")
				return err
			},
			ours: func(s *template.Template) error {
				_, err := s.ParseFiles("testdata/error_location.gohtml")
				return err
			},
		},
		{
			name: "ParseGlob",
			html: func(s *htmltemplate.Template) error {
				_, err := s.ParseGlob("testdata/error_location.gohtml")
				return err
			},
			ours: func(s *template.Template) error {
				_, err := s.ParseGlob("testdata/error_location.gohtml")
				return err
			},
		},
		{
			name: "ParseFS",
			html: func(s *htmltemplate.Template) error {
				_, err := s.ParseFS(frozenFS(), "*.gohtml")
				return err
			},
			ours: func(s *template.Template) error {
				_, err := s.ParseFS(frozenFS(), "*.gohtml")
				return err
			},
		},
		{
			name: "Clone",
			html: func(s *htmltemplate.Template) error { _, err := s.Clone(); return err },
			ours: func(s *template.Template) error { _, err := s.Clone(); return err },
		},
	}
}

func frozenFS() fstest.MapFS {
	return fstest.MapFS{"extra.gohtml": {Data: []byte(`<i>{{.}}</i>`)}}
}

func TestSetIsFrozenAfterExecute(t *testing.T) {
	for _, first := range frozenFirstRuns {
		for _, op := range frozenOperations() {
			t.Run(first.name+"/"+op.name, func(t *testing.T) {
				htmlSet := htmltemplate.Must(htmltemplate.New("page").Parse(first.text))
				htmlRunErr := first.html(htmlSet)
				require.Equal(t, first.wantErr, htmlRunErr != nil, "html/template first run error: %v", htmlRunErr)
				wantErr := op.html(htmlSet)
				require.Error(t, wantErr, "html/template must refuse %s", op.name)

				ourSet := template.Must(template.New("page").Parse(first.text))
				ourRunErr := first.ours(ourSet)
				require.Equal(t, first.wantErr, ourRunErr != nil, "first run error: %v", ourRunErr)
				gotErr := op.ours(ourSet)
				require.Error(t, gotErr, "%s after %s must fail", op.name, first.name)

				assert.Equal(t, wantErr.Error(), gotErr.Error())
			})
		}
	}
}

func TestExecuteAfterRefusedParseStillEscapes(t *testing.T) {
	const text = `<p>{{.}}</p>`
	const addText = `<div>{{.}}</div>`

	htmlSet := htmltemplate.Must(htmltemplate.New("page").Parse(text))
	var want bytes.Buffer
	require.NoError(t, htmlSet.Execute(io.Discard, afterExecuteValue))
	_, err := htmlSet.Parse(addText)
	require.Error(t, err)
	require.NoError(t, htmlSet.Execute(&want, afterExecuteValue))

	ourSet := template.Must(template.New("page").Parse(text))
	require.NoError(t, ourSet.Execute(io.Discard, afterExecuteValue))
	_, err = ourSet.Parse(addText)
	require.Error(t, err)
	var got bytes.Buffer
	require.NoError(t, ourSet.Execute(&got, afterExecuteValue))

	assert.Equal(t, want.String(), got.String())
	assert.NotContains(t, got.String(), "<script>")
}

func TestFuncsAfterExecuteStillWorks(t *testing.T) {
	const text = `<p>{{.}}</p>`

	htmlSet := htmltemplate.Must(htmltemplate.New("page").Parse(text))
	require.NoError(t, htmlSet.Execute(io.Discard, afterExecuteValue))
	assert.NotNil(t, htmlSet.Funcs(htmltemplate.FuncMap{"f": func() string { return "f" }}))
	assert.NotNil(t, htmlSet.Delims("", ""))
	assert.NotNil(t, htmlSet.Option("missingkey=zero"))
	assert.NotNil(t, htmlSet.Lookup("page"))
	assert.NotNil(t, htmlSet.New("x"))
	var want bytes.Buffer
	require.NoError(t, htmlSet.Execute(&want, afterExecuteValue))

	ourSet := template.Must(template.New("page").Parse(text))
	require.NoError(t, ourSet.Execute(io.Discard, afterExecuteValue))
	assert.NotNil(t, ourSet.Funcs(template.FuncMap{"f": func() string { return "f" }}))
	assert.NotNil(t, ourSet.Delims("", ""))
	assert.NotNil(t, ourSet.Option("missingkey=zero"))
	assert.NotNil(t, ourSet.Lookup("page"))
	assert.NotNil(t, ourSet.New("x"))
	var got bytes.Buffer
	require.NoError(t, ourSet.Execute(&got, afterExecuteValue))

	assert.Equal(t, want.String(), got.String())
}
