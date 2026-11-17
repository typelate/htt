package template_test

import (
	htmltemplate "html/template"
	"testing"

	"github.com/typelate/htt/template"
)

func TestStyleSrcsetAndMetaMatchHTMLTemplate(t *testing.T) {
	rows := []string{
		`<p style="color: {{.}}">t</p>`,
		`<p style="{{.}}">t</p>`,
		`<style>p { color: {{.}} }</style>`,
		`<style>p { background: url("{{.}}") }</style>`,
		`<p style="background: url('{{.}}')">t</p>`,
		`<style>p::after { content: "{{.}}" }</style>`,
		`<img srcset="{{.}}">`,
		`<img srcset="/a.png 1x, {{.}} 2x">`,
		`<meta http-equiv="refresh" content="0; url={{.}}">`,
		`<meta name="x" content="{{.}}">`,
		`<p><!-- {{.}} -->t</p>`,
	}
	// Each row runs with each value. Typed values name both packages' types,
	// since each package declares its own.
	values := []struct {
		name    string
		data    any
		httData any
	}{
		{name: "plain", data: `a'b/c&d=e</`},
		{name: "keyword", data: `red`},
		{name: "expression", data: `expression(alert(1))`},
		{name: "javascript URL", data: `javascript:alert(1)`},
		{name: "srcset", data: `/img.png 1x, /img2.png 2x`},
		{name: "typed CSS", data: htmltemplate.CSS("color: red"), httData: template.CSS("color: red")},
		{name: "typed URL", data: htmltemplate.URL("javascript:alert(1)"), httData: template.URL("javascript:alert(1)")},
		{name: "typed Srcset", data: htmltemplate.Srcset("/a.png 2x"), httData: template.Srcset("/a.png 2x")},
	}
	for _, src := range rows {
		for _, v := range values {
			t.Run(src+" with "+v.name, func(t *testing.T) {
				want := runHTMLTemplate(t, src, v.data)
				data := v.data
				if v.httData != nil {
					data = v.httData
				}
				got := runHTT(t, src, data)
				if got != want {
					t.Errorf("htt/template.Execute(%q, %#v) = %q, want %q", src, data, got, want)
				}
			})
		}
	}
}
