package template_test

import (
	htmltemplate "html/template"
	"io"
	"testing"

	"github.com/typelate/htt/template"
)

// TestErrorLocationsMatchHTMLTemplate checks that an escaping error begins
// with the same ParseName:line:column that html/template gives, for errors
// that are not on the first line.
func TestErrorLocationsMatchHTMLTemplate(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			"branch end after indentation on line 3",
			"<p>\n<p>\n    {{if .Cond}}<a{{end}}",
		},
		{
			"range loop re-entry on line 4",
			"<p>\n<p>\n<p>\n  {{range .Items}} x='<a{{end}}",
		},
		{
			"ambiguous URL action on line 3",
			"<p>\n<a href=\"{{if .F}}/foo?a={{else}}/bar/{{end}}\n  {{.H}}\">",
		},
		{
			"predefined escaper on line 3",
			"a\nb\n  Hello, {{. | urlquery | print}}!",
		},
		{
			"error inside a define that the page calls",
			"<p>\n{{define \"row\"}}\n  {{if .X}}<a{{end}}\n{{end}}{{template \"row\" .}}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := errorString(stdErrorRun(tt.input).err)
			got := errorString(httErrorRun(tt.input).err)
			if got != want {
				t.Errorf("error\n\t%q\nhtml/template\n\t%q", got, want)
			}
		})
	}
}

// TestErrorLocationFromParseFiles checks the location for a template read from
// a file, where the ParseName is the base name of the file.
func TestErrorLocationFromParseFiles(t *testing.T) {
	const file = "testdata/error_location.gohtml"

	stdTmpl, err := htmltemplate.ParseFiles(file)
	if err != nil {
		t.Fatal(err)
	}
	want := errorString(stdTmpl.Execute(io.Discard, nil))

	tmpl, err := template.ParseFiles(file)
	if err != nil {
		t.Fatal(err)
	}
	got := errorString(tmpl.Execute(io.Discard, nil))

	if got != want {
		t.Errorf("error\n\t%q\nhtml/template\n\t%q", got, want)
	}
}
