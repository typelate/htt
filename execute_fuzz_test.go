package htt_test

import (
	"io"
	"testing"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

// FuzzExecuteHTMLNoPanic runs the HTML engine on arbitrary templates and string
// data. Errors are fine; panics and hangs are not. The engine escapes less than
// html/template, so its output is not compared with html/template's.
func FuzzExecuteHTMLNoPanic(f *testing.F) {
	seeds := []struct {
		tmpl string
		data string
	}{
		// static
		{"<p>hello</p>", ""},
		{"<br>", ""},
		{"<div><span>x</span></div>", ""},
		{"<!-- comment -->", ""},

		// text context escaping
		{"<p>{{.}}</p>", "hello"},
		{"<p>{{.}}</p>", "<script>alert(1)</script>"},
		{"<p>{{.}}</p>", "a&b"},
		{"<p>{{.}}</p>", `he said "hi"`},
		{"<p>{{.}}</p>", "it's"},
		{"<p>{{.}}</p>", ""},
		{"<p>{{.}}</p>", "a\u2028b\u2029c"},

		// attribute escaping
		{`<div class="{{.}}">x</div>`, "my-class"},
		{`<div class="{{.}}">x</div>`, `"><script>alert(1)</script>`},
		{`<div title="{{.}}">x</div>`, `a "quoted" & <value>`},
		{`<div id="{{.}}">x</div>`, "abc"},

		// URL attributes
		{`<a href="{{.}}">x</a>`, "https://example.com"},
		{`<a href="{{.}}">x</a>`, "http://example.com"},
		{`<a href="{{.}}">x</a>`, "mailto:user@example.com"},
		{`<a href="{{.}}">x</a>`, "javascript:alert(1)"},
		{`<a href="{{.}}">x</a>`, "data:text/html,<script>"},
		{`<a href="{{.}}">x</a>`, "vbscript:MsgBox"},
		{`<a href="{{.}}">x</a>`, "/relative/path"},
		{`<a href="{{.}}">x</a>`, "?query=1&b=2"},
		{`<a href="{{.}}">x</a>`, "#fragment"},
		{`<img src="{{.}}">`, "javascript:alert(1)"},
		{`<a href="{{.}}">x</a>`, "https://example.com?a=1&b=2"},

		// event handlers
		{`<button onclick="{{.}}">x</button>`, "doStuff()"},
		{`<button onclick="{{.}}">x</button>`, ""},

		// comments stripped
		{`<!-- {{.}} --><p>hi</p>`, "secret"},

		// various elements
		{"<p>{{.}}</p>", "normal text"},
		{"<textarea>{{.}}</textarea>", "<script>alert(1)</script>"},
		{"<title>{{.}}</title>", "<script>alert(1)</script>"},
	}
	for _, s := range seeds {
		f.Add(s.tmpl, s.data)
	}
	f.Fuzz(func(t *testing.T, tmpl, data string) {
		tree, err := htt.ParseTree("test", "test", tmpl, nil, lex.Options{
			LeftDelim:  "{{",
			RightDelim: "}}",
			ParseHTML:  true,
			BreakOK:    true,
			ContinueOK: true,
		})
		if err != nil {
			return
		}
		htt.AnnotateHTMLContexts(tree.Root)
		// Execution errors are fine, panics are not.
		htt.ExecuteTemplate(io.Discard, tree, data)
	})
}

// FuzzExecuteTextNoPanic fuzzes the text-mode execution engine to ensure
// no panics occur on arbitrary templates with nil data.
func FuzzExecuteTextNoPanic(f *testing.F) {
	seeds := []string{
		"hello",
		"{{.}}",
		`{{"hello"}}`,
		"{{42}}",
		"{{true}}",
		`{{print "hi"}}`,
		`{{printf "%d" 42}}`,
		"{{if true}}yes{{end}}",
		"{{if false}}yes{{else}}no{{end}}",
		`{{$x := "val"}}{{$x}}`,
		"{{$x := 1}}{{$x = 2}}{{$x}}",
		"{{eq 1 1}}",
		"{{lt 1 2}}",
		"{{and 1 0}}",
		"{{or 0 1}}",
		"{{not true}}",
		`{{len "hello"}}`,
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, tmpl string) {
		tree, err := htt.ParseTree("fuzz", "fuzz", tmpl, nil, lex.Options{
			LeftDelim:  "{{",
			RightDelim: "}}",
			BreakOK:    true,
			ContinueOK: true,
		})
		if err != nil {
			return
		}
		// Execute with nil data — errors are fine, panics are not.
		htt.ExecuteTemplate(io.Discard, tree, nil)
	})
}
