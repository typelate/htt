package template_test

import (
	htmltemplate "html/template"
	"testing"

	"github.com/typelate/htt/template"
)

func TestAttributesAndURLsMatchHTMLTemplate(t *testing.T) {
	rows := []string{
		`<a href="/x?q={{.}}">l</a>`,
		`<a href="{{.}}">l</a>`,
		`<a href='{{.}}'>l</a>`,
		`<a href={{.}}>l</a>`,
		`<a href="/p/{{.}}/q?x={{.}}#{{.}}">l</a>`,
		`<img src="{{.}}">`,
		`<a title="{{.}}">l</a>`,
		`<a title={{.}}>l</a>`,
		`<input value="{{.}}" {{.}}>`,
		`<a {{.}}="v">l</a>`,
		`<textarea>{{.}}</textarea>`,
		`<title>{{.}}</title>`,
		`<a href="{{. | urlquery}}">l</a>`,
		`<a title="{{. | html}}">l</a>`,
		`<a title="{{html .}}">l</a>`,
		`<p>{{html}}</p>`,
		`<form action="{{.}}"><button formaction="{{.}}">b</button></form>`,
		`<a href="{{if .}}/a?b={{.}}{{else}}/c{{end}}">l</a>`,
	}
	// Each row runs with each value. Typed values name both packages' types,
	// since each package declares its own.
	values := []struct {
		name    string
		data    any
		httData any
	}{
		{name: "plain", data: `a'b/c&d=e</`, httData: `a'b/c&d=e</`},
		{name: "javascript URL", data: "javascript:alert(1)", httData: "javascript:alert(1)"},
		{name: "typed URL", data: htmltemplate.URL("javascript:alert(1)"), httData: template.URL("javascript:alert(1)")},
		{name: "typed HTML", data: htmltemplate.HTML("<b>x</b>"), httData: template.HTML("<b>x</b>")},
	}
	for _, src := range rows {
		for _, v := range values {
			t.Run(src+" with "+v.name, func(t *testing.T) {
				want := runHTMLTemplate(t, src, v.data)
				got := runHTT(t, src, v.httData)
				if got != want {
					t.Errorf("htt/template.Execute(%q, %#v) = %q, want %q", src, v.httData, got, want)
				}
			})
		}
	}
}

func TestURLQueryMatchesHTMLTemplate(t *testing.T) {
	const src = `<a href="/x?q={{.}}">x</a>`
	const data = `a b+&`
	want := runHTMLTemplate(t, src, data)
	if got := runHTT(t, src, data); got != want {
		t.Errorf("htt/template.Execute(%q, %#v) = %q, want %q", src, data, got, want)
	}
}
