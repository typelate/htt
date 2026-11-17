package template

import (
	stdtemplate "html/template"
	"strings"
	"testing"
)

// TestExecuteTrustsStdlibHTML states that the standard library's
// html/template content types are trusted like this package's own: code
// that runs this engine in process may hand it either.
func TestExecuteTrustsStdlibHTML(t *testing.T) {
	ts := Must(New("page").Parse(`<p>{{.}}</p>`))
	var out strings.Builder
	if err := ts.Execute(&out, stdtemplate.HTML("<b>bold</b>")); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), `<p><b>bold</b></p>`; got != want {
		t.Errorf("Execute wrote %s, want %s", got, want)
	}
}
