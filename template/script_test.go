package template_test

import (
	htmltemplate "html/template"
	"testing"

	"github.com/typelate/htt/template"
)

func TestScriptContextsMatchHTMLTemplate(t *testing.T) {
	rows := []string{
		`<script>var x = {{.}};</script>`,
		`<script>var s = "{{.}}";</script>`,
		`<script>var s = '{{.}}';</script>`,
		"<script>var s = `{{.}}`;</script>",
		`<script>var r = /{{.}}/;</script>`,
		`<script>f({{.}}, "{{.}}")</script>`,
		`<a onclick="f({{.}})">l</a>`,
		`<a onclick="f('{{.}}')">l</a>`,
		`<a data-on:click="@post('{{.}}')">l</a>`,
		`<button onclick='g("{{.}}")'>b</button>`,
		"<script>// c {{.}}\nvar y = {{.}};</script>",
		`<script type="application/json">{"k": {{.}}}</script>`,
		`<script>if (a < {{.}}) { b = {{.}} / 2 }</script>`,
	}
	// Each row runs with each value. Typed values name both packages' types,
	// since each package declares its own.
	values := []struct {
		name    string
		data    any
		httData any
	}{
		{name: "plain", data: `a'b/c&d=e</`},
		{name: "script end and comment", data: `</script><!--`},
		{name: "integer", data: 42},
		{name: "slice", data: []string{"a", "<b>"}},
		{name: "struct", data: struct{ A, B string }{A: "<x>", B: "y'z"}},
		{name: "nil", data: nil},
		{name: "typed JS", data: htmltemplate.JS("f(1)"), httData: template.JS("f(1)")},
		{name: "typed JSStr", data: htmltemplate.JSStr("it's"), httData: template.JSStr("it's")},
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
