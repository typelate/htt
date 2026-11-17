package template_test

import (
	htmltemplate "html/template"
	"strings"
	"testing"

	"github.com/typelate/htt/template"
)

// callRun parses src under a root named "root", so that the rows can define "t".
func callRunHTMLTemplate(t *testing.T, src string, data any) string {
	t.Helper()
	tmpl, err := htmltemplate.New("root").Parse(src)
	if err != nil {
		t.Fatalf("html/template.Parse(%q) = %v", src, err)
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, data); err != nil {
		t.Fatalf("html/template.Execute(%q) = %v", src, err)
	}
	return b.String()
}

func callRunHTT(t *testing.T, src string, data any) string {
	t.Helper()
	tmpl, err := template.New("root").Parse(src)
	if err != nil {
		t.Fatalf("template.Parse(%q) = %v", src, err)
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, data); err != nil {
		t.Fatalf("template.Execute(%q) = %v", src, err)
	}
	return b.String()
}

func TestTemplateCallsMatchHTMLTemplate(t *testing.T) {
	const defT = `{{define "t"}}{{.}}{{end}}`
	rows := []string{
		defT + `<a title="{{template "t" .}}">l</a>`,
		defT + `<a href="/x?q={{template "t" .}}">l</a>`,
		defT + `<script>var x = {{template "t" .}};</script>`,
		defT + `<a onclick="f('{{template "t" .}}')">l</a>`,
		defT + `<style>p { color: {{template "t" .}} }</style>`,
		defT + `<p>{{template "t" .}}</p><a href="{{template "t" .}}">l</a><script>var y = "{{template "t" .}}";</script>`,
		`<a title="{{block "b" .}}[{{.}}]{{end}}">l</a>`,
		`{{define "u"}}({{template "t" .}}){{end}}` + defT + `<a title="{{template "u" .}}">l</a>`,
		defT + `<input {{template "t" .}} hidden>`,
		`{{define "r"}}{{if .}}{{.}}{{template "r" ""}}{{end}}{{end}}<a title="{{template "r" .}}">l</a>`,
		defT + `<textarea>{{template "t" .}}</textarea>`,
	}
	values := []any{
		`a'b/c&d=e</`,
		`<b>+"x"`,
	}
	for _, src := range rows {
		for _, data := range values {
			t.Run(src, func(t *testing.T) {
				want := callRunHTMLTemplate(t, src, data)
				got := callRunHTT(t, src, data)
				if got != want {
					t.Errorf("htt/template.Execute(%q, %#v) = %q, want %q", src, data, got, want)
				}
			})
		}
	}
}
