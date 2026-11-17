package template_test

import (
	htmltemplate "html/template"
	"strings"
	"testing"

	"github.com/typelate/htt/template"
)

func TestElementTextMatchesHTMLTemplate(t *testing.T) {
	for _, tt := range []struct {
		name string
		src  string
		data any
		// httData is the data for htt/template when it differs from data,
		// as for the safe-content types, which each package declares.
		httData any
	}{
		{name: "plus in text", src: `{{.}}`, data: "+"},
		{name: "special characters in an element", src: `<p>{{.}}</p>`, data: `<b>&"'`},
		{name: "NUL in an element", src: `<p>{{.}}</p>`, data: "a\x00b"},
		{name: "html/template's HTML is trusted", src: `<p>{{.}}</p>`, data: htmltemplate.HTML("<b>x</b>")},
		{name: "own HTML is trusted", src: `<p>{{.}}</p>`, data: htmltemplate.HTML("<b>x</b>"), httData: template.HTML("<b>x</b>")},
		{name: "integer", src: `<p>{{.}}</p>`, data: 42},
		{name: "nil", src: `<p>{{.}}</p>`, data: nil},
		{name: "missing map key", src: `<p>{{.k}}</p>`, data: map[string]string{}},
		{
			name: "if and else",
			src:  `{{if .}}<i>{{.}}</i>{{else}}none{{end}}`,
			data: "a+b",
		},
		{
			name: "else branch",
			src:  `{{if .}}<i>{{.}}</i>{{else}}none{{end}}`,
			data: "",
		},
		{name: "range", src: `{{range .}}<li>{{.}}</li>{{end}}`, data: []string{"a+", "b<"}},
		{
			name: "define and template",
			src:  `{{define "u"}}<b>{{.}}</b>{{end}}<p>{{template "u" .}}</p>`,
			data: "x+",
		},
		{name: "block", src: `<p>{{block "b" .}}<i>{{.}}</i>{{end}}</p>`, data: "x+"},
		{name: "trim markers", src: "<p> {{- . -}} </p>", data: "x+"},
		{
			name: "trim markers on a template call and a block",
			src:  "{{define \"u\"}}<b>{{.}}</b>{{end}}<p> {{- template \"u\" . -}} </p> {{- block \"b\" . -}} <i>{{.}}</i> {{- end}} ",
			data: "x+",
		},
		{
			name: "trim markers on branches",
			src:  "{{if . -}} <i> {{.}} </i> {{- else -}} none {{- end}} {{range . -}} <li>{{.}}</li> {{- else -}} empty {{- end}}",
			data: []string{"a+", "b"},
		},

		{name: "comment in an element", src: `<p>a<!-- note -->b{{.}}</p>`, data: "+"},
		{name: "comment before an element", src: `<!-- lead --><p>{{.}}</p>`, data: "+"},
		{name: "multi-line comment", src: "<!--\nline one\nline two\n--><p>{{.}}</p>", data: "+"},
		{name: "JS comment in a script", src: `<script>/* note */ var x = 1;</script><p>{{.}}</p>`, data: "+"},
		{name: "CSS comment in a style", src: `<style>/* note */ p { color: red }</style><p>{{.}}</p>`, data: "+"},
		{
			name: "trim markers on a define",
			src:  "{{define \"u\" -}} <b>{{.}}</b> {{- end}}<p>{{template \"u\" .}}</p>",
			data: "+",
		},
		{name: "pipeline ends with html", src: `<p>{{. | html}}</p>`, data: `<a&b+c d>`},
		{name: "html command", src: `<p>{{html .}}</p>`, data: `<a&b+c d>`},
		{name: "pipeline ends with urlquery", src: `<p>{{. | urlquery}}</p>`, data: `<a&b+c d>`},
		{name: "urlquery command", src: `<p>{{urlquery .}}</p>`, data: `<a&b+c d>`},
		{name: "html command with two arguments", src: `<p>{{html . .}}</p>`, data: `<a&b+c d>`},
		{name: "html after a print", src: `<p>{{. | print | html}}</p>`, data: `<a&b+c d>`},
		{name: "html after a printf", src: `<p>{{printf "%s" . | html}}</p>`, data: `<a&b+c d>`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			want := runHTMLTemplate(t, tt.src, tt.data)
			data := tt.data
			if tt.httData != nil {
				data = tt.httData
			}
			got := runHTT(t, tt.src, data)
			if got != want {
				t.Errorf("htt/template.Execute(%q, %#v) = %q, want %q", tt.src, data, got, want)
			}
		})
	}
}

func TestOldEngineStillRunsWhereTheEscaperIsNotUsed(t *testing.T) {
	for _, tt := range []struct {
		name string
		src  string
		data any
		want string
	}{
		{name: "action in a URL query", src: `<a href="/x?q={{.}}">x</a>`, data: `a b+&`, want: `<a href="/x?q=a b&#43;&amp;">x</a>`},
		{name: "action in a script", src: `<script>var x = {{.}};</script>`, data: `a<+`, want: "<script>var x = \"a\\u003c\\u002b\";</script>"},
		{name: "html in a pipeline", src: `<p>{{. | html}}</p>`, data: `<+`, want: `<p>&lt;+</p>`},
		{name: "call from an attribute", src: `{{define "u"}}{{.}}{{end}}<a title="{{template "u" .}}">`, data: `+`, want: `<a title="">`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := runHTT(t, tt.src, tt.data); got != tt.want {
				t.Errorf("htt/template.Execute(%q, %#v) = %q, want %q", tt.src, tt.data, got, tt.want)
			}
		})
	}
}

func runHTMLTemplate(t *testing.T, src string, data any) string {
	t.Helper()
	tmpl, err := htmltemplate.New("t").Parse(src)
	if err != nil {
		t.Fatalf("html/template.Parse(%q) = %v", src, err)
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, data); err != nil {
		t.Fatalf("html/template.Execute(%q) = %v", src, err)
	}
	return b.String()
}

func runHTT(t *testing.T, src string, data any) string {
	t.Helper()
	tmpl, err := template.New("t").Parse(src)
	if err != nil {
		t.Fatalf("template.Parse(%q) = %v", src, err)
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, data); err != nil {
		t.Fatalf("template.Execute(%q) = %v", src, err)
	}
	return b.String()
}
