// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Copied from html/template/escape_test.go of go1.27.3, with the changes marked below.

package template

// Changed: left out, because their tests are already in another form here:
// TestEscape and TestErrors (compat_html_test.go), TestEscapeText
// (esc_escape_text_test.go), TestIssue81821 and TestIssue81823 (go_fixes_test.go).
// The badMarshaler and goodMarshaler types were used only by TestEscape, so they go too.
// Changed: left out, because this package has no such unexported state or function:
// TestEnsurePipelineContains (planPipeline edits a pipeline instead of rewriting its text) and
// TestRedundantFuncs (redundantFuncs is not here; equivEscapers is a different table).
// Changed: left out, because its GODEBUG (htmlmetacontenturlescape) is not read here:
// TestMetaContentEscapeGODEBUG. The same reason drops the internal/testenv import.
// Changed: text/template and text/template/parse are dropped with TestEscapeText and
// TestEnsurePipelineContains, the only tests that used them.
import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestEscapeMap(t *testing.T) {
	data := map[string]string{
		"html":     `<h1>Hi!</h1>`,
		"urlquery": `http://www.foo.com/index.html?title=main`,
	}
	for _, test := range [...]struct {
		desc, input, output string
	}{
		// covering issue 20323
		{
			"field with predefined escaper name 1",
			`{{.html | print}}`,
			`&lt;h1&gt;Hi!&lt;/h1&gt;`,
		},
		// covering issue 20323
		{
			"field with predefined escaper name 2",
			`{{.urlquery | print}}`,
			`http://www.foo.com/index.html?title=main`,
		},
	} {
		tmpl := Must(New("").Parse(test.input))
		b := new(strings.Builder)
		if err := tmpl.Execute(b, data); err != nil {
			t.Errorf("%s: template execution failed: %s", test.desc, err)
			continue
		}
		if w, g := test.output, b.String(); w != g {
			t.Errorf("%s: escaped output: want\n\t%q\ngot\n\t%q", test.desc, w, g)
			continue
		}
	}
}

func TestEscapeSet(t *testing.T) {
	type dataItem struct {
		Children []*dataItem
		X        string
	}

	data := dataItem{
		Children: []*dataItem{
			{X: "foo"},
			{X: "<bar>"},
			{
				Children: []*dataItem{
					{X: "baz"},
				},
			},
		},
	}

	tests := []struct {
		inputs map[string]string
		want   string
	}{
		// The trivial set.
		{
			map[string]string{
				"main": ``,
			},
			``,
		},
		// A template called in the start context.
		{
			map[string]string{
				"main": `Hello, {{template "helper"}}!`,
				// Not a valid top level HTML template.
				// "<b" is not a full tag.
				"helper": `{{"<World>"}}`,
			},
			`Hello, &lt;World&gt;!`,
		},
		// A template called in a context other than the start.
		{
			map[string]string{
				"main": `<a onclick='a = {{template "helper"}};'>`,
				// Not a valid top level HTML template.
				// "<b" is not a full tag.
				"helper": `{{"<a>"}}<b`,
			},
			`<a onclick='a = &#34;\u003ca\u003e&#34;<b;'>`,
		},
		// A recursive template that ends in its start context.
		{
			map[string]string{
				"main": `{{range .Children}}{{template "main" .}}{{else}}{{.X}} {{end}}`,
			},
			`foo &lt;bar&gt; baz `,
		},
		// A recursive helper template that ends in its start context.
		{
			map[string]string{
				"main":   `{{template "helper" .}}`,
				"helper": `{{if .Children}}<ul>{{range .Children}}<li>{{template "main" .}}</li>{{end}}</ul>{{else}}{{.X}}{{end}}`,
			},
			`<ul><li>foo</li><li>&lt;bar&gt;</li><li><ul><li>baz</li></ul></li></ul>`,
		},
		// Co-recursive templates that end in its start context.
		{
			map[string]string{
				"main":   `<blockquote>{{range .Children}}{{template "helper" .}}{{end}}</blockquote>`,
				"helper": `{{if .Children}}{{template "main" .}}{{else}}{{.X}}<br>{{end}}`,
			},
			`<blockquote>foo<br>&lt;bar&gt;<br><blockquote>baz<br></blockquote></blockquote>`,
		},
		// A template that is called in two different contexts.
		{
			map[string]string{
				"main":   `<button onclick="title='{{template "helper"}}'; ...">{{template "helper"}}</button>`,
				"helper": `{{11}} of {{"<100>"}}`,
			},
			`<button onclick="title='11 of \u003c100\u003e'; ...">11 of &lt;100&gt;</button>`,
		},
		// A non-recursive template that ends in a different context.
		// helper starts in jsCtxRegexp and ends in jsCtxDivOp.
		{
			map[string]string{
				"main":   `<script>var x={{template "helper"}}/{{"42"}};</script>`,
				"helper": "{{126}}",
			},
			`<script>var x= 126 /"42";</script>`,
		},
		// A recursive template that ends in a similar context.
		{
			map[string]string{
				"main":      `<script>var x=[{{template "countdown" 4}}];</script>`,
				"countdown": `{{.}}{{if .}},{{template "countdown" . | pred}}{{end}}`,
			},
			`<script>var x=[ 4 , 3 , 2 , 1 , 0 ];</script>`,
		},
		// A recursive template that ends in a different context.
		/*
			{
				map[string]string{
					"main":   `<a href="/foo{{template "helper" .}}">`,
					"helper": `{{if .Children}}{{range .Children}}{{template "helper" .}}{{end}}{{else}}?x={{.X}}{{end}}`,
				},
				`<a href="/foo?x=foo?x=%3cbar%3e?x=baz">`,
			},
		*/
	}

	// pred is a template function that returns the predecessor of a
	// natural number for testing recursive templates.
	fns := FuncMap{"pred": func(a ...any) (any, error) {
		if len(a) == 1 {
			if i, _ := a[0].(int); i > 0 {
				return i - 1, nil
			}
		}
		return nil, fmt.Errorf("undefined pred(%v)", a)
	}}

	for _, test := range tests {
		source := ""
		for name, body := range test.inputs {
			source += fmt.Sprintf("{{define %q}}%s{{end}} ", name, body)
		}
		tmpl, err := New("root").Funcs(fns).Parse(source)
		if err != nil {
			t.Errorf("error parsing %q: %v", source, err)
			continue
		}
		var b strings.Builder

		if err := tmpl.ExecuteTemplate(&b, "main", data); err != nil {
			t.Errorf("%q executing %v", err.Error(), tmpl.Lookup("main"))
			continue
		}
		if got := b.String(); test.want != got {
			t.Errorf("want\n\t%q\ngot\n\t%q", test.want, got)
		}
	}
}

func TestEscapeMalformedPipelines(t *testing.T) {
	tests := []string{
		"{{ 0 | $ }}",
		"{{ 0 | $ | urlquery }}",
		"{{ 0 | (nil) }}",
		"{{ 0 | (nil) | html }}",
	}
	for _, test := range tests {
		var b bytes.Buffer
		tmpl, err := New("test").Parse(test)
		if err != nil {
			t.Errorf("failed to parse set: %q", err)
		}
		err = tmpl.Execute(&b, nil)
		if err == nil {
			t.Errorf("Expected error for %q", test)
		}
	}
}

func TestEscapeErrorsNotIgnorable(t *testing.T) {
	var b bytes.Buffer
	tmpl, _ := New("dangerous").Parse("<a")
	err := tmpl.Execute(&b, nil)
	if err == nil {
		t.Errorf("Expected error")
	} else if b.Len() != 0 {
		t.Errorf("Emitted output despite escaping failure")
	}
}

func TestEscapeSetErrorsNotIgnorable(t *testing.T) {
	var b bytes.Buffer
	tmpl, err := New("root").Parse(`{{define "t"}}<a{{end}}`)
	if err != nil {
		t.Errorf("failed to parse set: %q", err)
	}
	err = tmpl.ExecuteTemplate(&b, "t", nil)
	if err == nil {
		t.Errorf("Expected error")
	} else if b.Len() != 0 {
		t.Errorf("Emitted output despite escaping failure")
	}
}

func TestIndirectPrint(t *testing.T) {
	a := 3
	ap := &a
	b := "hello"
	bp := &b
	bpp := &bp
	tmpl := Must(New("t").Parse(`{{.}}`))
	var buf strings.Builder
	err := tmpl.Execute(&buf, ap)
	if err != nil {
		t.Errorf("Unexpected error: %s", err)
	} else if buf.String() != "3" {
		t.Errorf(`Expected "3"; got %q`, buf.String())
	}
	buf.Reset()
	err = tmpl.Execute(&buf, bpp)
	if err != nil {
		t.Errorf("Unexpected error: %s", err)
	} else if buf.String() != "hello" {
		t.Errorf(`Expected "hello"; got %q`, buf.String())
	}
}

// This is a test for issue 3272.
func TestEmptyTemplateHTML(t *testing.T) {
	page := Must(New("page").ParseFiles(os.DevNull))
	if err := page.ExecuteTemplate(os.Stdout, "page", "nothing"); err == nil {
		t.Fatal("expected error")
	}
}

type Issue7379 int

func (Issue7379) SomeMethod(x int) string {
	return fmt.Sprintf("<%d>", x)
}

// This is a test for issue 7379: type assertion error caused panic, and then
// the code to handle the panic breaks escaping. It's hard to see the second
// problem once the first is fixed, but its fix is trivial so we let that go. See
// the discussion for issue 7379.
func TestPipeToMethodIsEscaped(t *testing.T) {
	tmpl := Must(New("x").Parse("<html>{{0 | .SomeMethod}}</html>\n"))
	tryExec := func() string {
		defer func() {
			panicValue := recover()
			if panicValue != nil {
				t.Errorf("panicked: %v\n", panicValue)
			}
		}()
		var b strings.Builder
		tmpl.Execute(&b, Issue7379(0))
		return b.String()
	}
	for i := 0; i < 3; i++ {
		str := tryExec()
		const expect = "<html>&lt;0&gt;</html>\n"
		if str != expect {
			t.Errorf("expected %q got %q", expect, str)
		}
	}
}

// Unlike text/template, html/template crashed if given an incomplete
// template, that is, a template that had been named but not given any content.
// This is issue #10204.
func TestErrorOnUndefined(t *testing.T) {
	tmpl := New("undefined")

	err := tmpl.Execute(nil, nil)
	if err == nil {
		t.Error("expected error")
	} else if !strings.Contains(err.Error(), "incomplete") {
		t.Errorf("expected error about incomplete template; got %s", err)
	}
}

// This covers issue #20842.
func TestIdempotentExecute(t *testing.T) {
	tmpl := Must(New("").
		Parse(`{{define "main"}}<body>{{template "hello"}}</body>{{end}}`))
	Must(tmpl.
		Parse(`{{define "hello"}}Hello, {{"Ladies & Gentlemen!"}}{{end}}`))
	got := new(strings.Builder)
	var err error
	// Ensure that "hello" produces the same output when executed twice.
	want := "Hello, Ladies &amp; Gentlemen!"
	for i := 0; i < 2; i++ {
		err = tmpl.ExecuteTemplate(got, "hello", nil)
		if err != nil {
			t.Errorf("unexpected error: %s", err)
		}
		if got.String() != want {
			t.Errorf("after executing template \"hello\", got:\n\t%q\nwant:\n\t%q\n", got.String(), want)
		}
		got.Reset()
	}
	// Ensure that the implicit re-execution of "hello" during the execution of
	// "main" does not cause the output of "hello" to change.
	err = tmpl.ExecuteTemplate(got, "main", nil)
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}
	// If the HTML escaper is added again to the action {{"Ladies & Gentlemen!"}},
	// we would expected to see the ampersand overescaped to "&amp;amp;".
	want = "<body>Hello, Ladies &amp; Gentlemen!</body>"
	if got.String() != want {
		t.Errorf("after executing template \"main\", got:\n\t%q\nwant:\n\t%q\n", got.String(), want)
	}
}

func BenchmarkEscapedExecute(b *testing.B) {
	tmpl := Must(New("t").Parse(`<a onclick="alert('{{.}}')">{{.}}</a>`))
	var buf bytes.Buffer
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tmpl.Execute(&buf, "foo & 'bar' & baz")
		buf.Reset()
	}
}

// Covers issue 22780.
func TestOrphanedTemplate(t *testing.T) {
	t1 := Must(New("foo").Parse(`<a href="{{.}}">link1</a>`))
	t2 := Must(t1.New("foo").Parse(`bar`))

	var b strings.Builder
	const wantError = `template: "foo" is an incomplete or empty template`
	t.Run("t1 is incomplete", func(t *testing.T) {
		if err := t1.Execute(&b, "javascript:alert(1)"); err == nil {
			t.Fatal("expected error executing t1")
		} else if gotError := err.Error(); gotError != wantError {
			t.Fatalf("got t1 execution error:\n\t%s\nwant:\n\t%s", gotError, wantError)
		}
	})
	b.Reset()
	if err := t2.Execute(&b, nil); err != nil {
		t.Fatalf("error executing t2: %s", err)
	}
	const want = "bar"
	if got := b.String(); got != want {
		t.Fatalf("t2 rendered %q, want %q", got, want)
	}
}

// Covers issue 21844.
func TestAliasedParseTreeDoesNotOverescape(t *testing.T) {
	const (
		tmplText = `{{.}}`
		data     = `<baz>`
		want     = `&lt;baz&gt;`
	)
	// Templates "foo" and "bar" both alias the same underlying parse tree.
	tpl := Must(New("foo").Parse(tmplText))
	if _, err := tpl.AddParseTree("bar", tpl.Tree); err != nil {
		t.Fatalf("AddParseTree error: %v", err)
	}
	var b1, b2 strings.Builder
	if err := tpl.ExecuteTemplate(&b1, "foo", data); err != nil {
		t.Fatalf(`ExecuteTemplate failed for "foo": %v`, err)
	}
	if err := tpl.ExecuteTemplate(&b2, "bar", data); err != nil {
		t.Fatalf(`ExecuteTemplate failed for "foo": %v`, err)
	}
	got1, got2 := b1.String(), b2.String()
	if got1 != want {
		t.Fatalf(`Template "foo" rendered %q, want %q`, got1, want)
	}
	if got1 != got2 {
		t.Fatalf(`Template "foo" and "bar" rendered %q and %q respectively, expected equal values`, got1, got2)
	}
}

func TestCVE202656858(t *testing.T) {
	tests := []struct {
		name  string
		tmpl  string
		input string
		want  string
	}{
		{
			name:  "regexp after open brace in if block",
			tmpl:  `<script>if(true){/{{.}}/g.test("x")}</script>`,
			input: "a.b",
			want:  `<script>if(true){/a\.b/g.test("x")}</script>`,
		},
		{
			name:  "regexp after close brace",
			tmpl:  `<script>if(true){x=1}/{{.}}/g.test("x")</script>`,
			input: "a.b",
			want:  `<script>if(true){x=1}/a\.b/g.test("x")</script>`,
		},
		{
			name:  "regexp pathological attacker input",
			tmpl:  `<script>if(true){/{{.}}/g.test("x")}</script>`,
			input: `./;alert(1);var q=/.`,
			want:  `<script>if(true){/\.\/;alert\(1\);var q=\/\./g.test("x")}</script>`,
		},
		{
			name:  "regexp after open brace in template literal",
			tmpl:  "<script>`${ (function(){/{{.}}/g.test(x)}) }`</script>",
			input: "a.b",
			want:  "<script>`${ (function(){/a\\.b/g.test(x)}) }`</script>",
		},
		{
			name:  "regexp after close brace in template literal",
			tmpl:  "<script>`${ (function(){}/{{.}}/g.test(x)) }`</script>",
			input: "a.b",
			want:  "<script>`${ (function(){}/a\\.b/g.test(x)) }`</script>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl := Must(New("test").Parse(tt.tmpl))
			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, tt.input); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if got := buf.String(); got != tt.want {
				t.Errorf("got:  %s\nwant: %s", got, tt.want)
			}
		})
	}
}
