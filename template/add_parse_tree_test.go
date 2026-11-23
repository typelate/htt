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

func TestAddParseTreeKeepsAnHTMLTreeForInspection(t *testing.T) {
	set := htt.New[any]("index")
	_, err := set.Parse(`<h1>x</h1>{{define "row"}}<td>{{.}}</td>{{end}}`)
	require.NoError(t, err)

	out := template.New("index")
	for _, name := range set.Names() {
		tree, ok := set.FindTree(name)
		require.True(t, ok, name)
		_, err := out.AddParseTree(name, tree)
		require.NoError(t, err)
	}

	row := out.Lookup("row")
	require.NotNil(t, row)
	var names []string
	for _, tmpl := range out.Templates() {
		names = append(names, tmpl.Name())
	}
	assert.ElementsMatch(t, []string{"index", "row"}, names)

	rowTree, ok := set.FindTree("row")
	require.True(t, ok)
	assert.Same(t, rowTree, row.Tree)
	assert.True(t, row.Tree.HTMLMode)
	var annotated bool
	htt.Walk(row.Tree, func(_ *htt.Tree, n htt.Node) {
		if action, ok := n.(*htt.ActionNode); ok && action.HTMLContext != nil {
			annotated = true
		}
	})
	assert.True(t, annotated, "an action of the tree has an HTML context")
}

func TestExecuteOfAnHTMLTreeParsedByTheRootPackage(t *testing.T) {
	set := htt.New[any]("index")
	_, err := set.Parse(`<h1>x</h1>{{define "row"}}<td title="{{.}}">{{.}}</td>{{end}}`)
	require.NoError(t, err)
	rowTree, ok := set.FindTree("row")
	require.True(t, ok)

	tmpl := template.New("index")
	_, err = tmpl.AddParseTree("row", rowTree)
	require.NoError(t, err)
	// Parsed before the first execution: a set that has executed refuses Parse.
	_, err = tmpl.Parse(`<tr>{{template "row" .}}</tr>`)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, tmpl.ExecuteTemplate(&buf, "row", `<b>"x"`))
	assert.Equal(t, `<td title="&lt;b&gt;&#34;x&#34;">&lt;b&gt;&#34;x&#34;</td>`, buf.String())

	buf.Reset()
	require.NoError(t, tmpl.Execute(&buf, `<b>"x"`))
	assert.Equal(t, `<tr><td title="&lt;b&gt;&#34;x&#34;">&lt;b&gt;&#34;x&#34;</td></tr>`, buf.String())
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

// foreignTreeCase adds the tree of the template named lookup, parsed from
// text into a set named "other", to a new set named "main" as "added", and
// executes the template named execute with value.
type foreignTreeCase struct {
	name        string
	left, right string // delimiters of the other set; empty for the default
	text        string // parsed into the other set
	lookup      string // template of the other set whose tree is added
	mainText    string // parsed into the receiving set after the tree is added, when not empty
	execute     string
	value       any
}

func TestAddParseTreeOfATreeFromAnotherSetMatchesHTMLTemplate(t *testing.T) {
	const value = `<b>'+"x"/&`
	for _, tt := range []foreignTreeCase{
		{
			name: "top-level template", text: `<a href="/x?q={{.}}" onclick="f('{{.}}')">{{.}}</a>`,
			lookup: "other", execute: "added", value: value,
		},
		{
			name: "defined template", text: `<h1>x</h1>{{define "row"}}<td title="{{.}}">{{.}}</td>{{end}}`,
			lookup: "row", execute: "added", value: value,
		},
		{
			name: "called from a template of the receiving set", text: `{{define "row"}}<td>{{.}}</td>{{end}}`,
			lookup: "row", mainText: `<tr title="{{template "added" .}}">{{template "added" .}}</tr>`, execute: "main", value: value,
		},
		{
			name: "other delimiters", left: "<<", right: ">>", text: `<p title="<<.>>"><<.>></p>{{.}}`,
			lookup: "other", execute: "added", value: value,
		},
		{
			name: "execution error", text: "<p>\n{{.X}}</p>",
			lookup: "other", execute: "added", value: value,
		},
		{
			name: "calls a template the receiving set lacks", text: `<p>{{template "missing" .}}</p>`,
			lookup: "other", execute: "added", value: value,
		},
		{
			name: "empty top-level template", text: `{{define "one"}}1{{end}}`,
			lookup: "other", execute: "added", value: value,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			want, wantErr := htmlTemplateForeignTree(t, tt)
			got, gotErr := httTemplateForeignTree(t, tt)
			if wantErr != nil {
				require.Error(t, gotErr)
				assert.Equal(t, wantErr.Error(), gotErr.Error())
			} else {
				require.NoError(t, gotErr)
			}
			assert.Equal(t, want, got)
		})
	}
}

func TestExecuteOfAnAddedHTMLTreeWithoutATextFormSaysWhy(t *testing.T) {
	set := htt.New[any]("index")
	_, err := set.Parse(`{{define "row"}}<td>{{.}}</td>{{end}}`)
	require.NoError(t, err)
	rowTree, ok := set.FindTree("row")
	require.True(t, ok)
	// The source the tree was parsed from defines no template of this name.
	rowTree.Name = "renamed"

	tmpl := template.New("index")
	_, err = tmpl.AddParseTree("row", rowTree)
	require.NoError(t, err)
	_, err = tmpl.Parse(`<p>{{template "row" .}}</p>`)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "row", "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"row"`)
	assert.Contains(t, err.Error(), "no text form")
	assert.Empty(t, buf.String())

	buf.Reset()
	err = tmpl.Execute(&buf, "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"row"`)
	assert.Contains(t, err.Error(), "no text form")
	assert.Empty(t, buf.String())
}

// htmlTemplateForeignTree runs tt with html/template.
func htmlTemplateForeignTree(t *testing.T, tt foreignTreeCase) (string, error) {
	t.Helper()
	other := htmltemplate.New("other")
	if tt.left != "" {
		other.Delims(tt.left, tt.right)
	}
	_, err := other.Parse(tt.text)
	require.NoError(t, err)
	set := htmltemplate.New("main")
	_, err = set.AddParseTree("added", other.Lookup(tt.lookup).Tree)
	require.NoError(t, err)
	if tt.mainText != "" {
		_, err = set.Parse(tt.mainText)
		require.NoError(t, err)
	}
	var buf bytes.Buffer
	err = set.ExecuteTemplate(&buf, tt.execute, tt.value)
	return buf.String(), err
}

// httTemplateForeignTree runs tt with htt/template.
func httTemplateForeignTree(t *testing.T, tt foreignTreeCase) (string, error) {
	t.Helper()
	other := template.New("other")
	if tt.left != "" {
		other.Delims(tt.left, tt.right)
	}
	_, err := other.Parse(tt.text)
	require.NoError(t, err)
	set := template.New("main")
	_, err = set.AddParseTree("added", other.Lookup(tt.lookup).Tree)
	require.NoError(t, err)
	if tt.mainText != "" {
		_, err = set.Parse(tt.mainText)
		require.NoError(t, err)
	}
	var buf bytes.Buffer
	err = set.ExecuteTemplate(&buf, tt.execute, tt.value)
	return buf.String(), err
}
