package template_test

import (
	htmltemplate "html/template"
	"strings"
	"testing"

	"github.com/typelate/htt/template"
)

type noValueData struct {
	E  any
	M  map[string]any
	MI map[string]int
}

// A value with nothing in it is written as html/template writes it, in each
// context: nothing in text and attributes, null in a script.
func TestNoValueMatchesHTMLTemplate(t *testing.T) {
	data := noValueData{M: map[string]any{}, MI: map[string]int{}}
	for _, text := range []string{
		`<p>{{.E}}</p>`,
		`<p>{{.M.missing}}</p>`,
		`<p>{{.MI.missing}}</p>`,
		`<a title="{{.E}}" href="/x?q={{.M.missing}}">x</a>`,
		`<a href="{{.M.missing}}">x</a>`,
		`<script>var x = {{.E}}; var y = {{.M.missing}}; var z = "{{.M.missing}}";</script>`,
		`<p style="color: {{.M.missing}}">x</p>`,
		`<p>{{.E | print}}</p>`,
		`<p>{{.M.missing | print}}</p>`,
		`<p>{{.M.missing | printf "%v"}}</p>`,
		`<p>{{with .M.missing}}{{.}}{{else}}none{{end}}</p>`,
	} {
		t.Run(text, func(t *testing.T) {
			var want, got strings.Builder
			wantErr := htmltemplate.Must(htmltemplate.New("t").Parse(text)).Execute(&want, data)
			gotErr := template.Must(template.New("t").Parse(text)).Execute(&got, data)
			if (wantErr == nil) != (gotErr == nil) {
				t.Fatalf("htt error %v, html/template error %v", gotErr, wantErr)
			}
			if wantErr != nil && wantErr.Error() != gotErr.Error() {
				t.Errorf("htt error %q, html/template error %q", gotErr, wantErr)
			}
			if got.String() != want.String() {
				t.Errorf("htt = %q, html/template = %q", got.String(), want.String())
			}
		})
	}
}
