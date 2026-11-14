package template

import (
	stdtemplate "html/template"
	"reflect"
	"strings"
	"testing"

	"github.com/typelate/htt"
)

// TestTrustOfStdlibContent states that the standard library's
// html/template content types are trusted like this package's own: code
// that runs this engine in process may hand it either.
func TestTrustOfStdlibContent(t *testing.T) {
	stdHTML := stdtemplate.HTML("<b>")
	for _, tt := range []struct {
		name  string
		value any
		want  htt.Trust
	}{
		{name: "html/template.HTML", value: stdtemplate.HTML("<b>"), want: htt.TrustHTML},
		{name: "html/template.URL", value: stdtemplate.URL("javascript:x"), want: htt.TrustURL},
		{name: "html/template.JS", value: stdtemplate.JS("x()"), want: htt.TrustJS},
		{name: "html/template.JSStr", value: stdtemplate.JSStr(`a\"b`), want: htt.TrustJSStr},
		{name: "html/template.CSS", value: stdtemplate.CSS("color: red"), want: htt.TrustCSS},
		{name: "html/template.HTMLAttr", value: stdtemplate.HTMLAttr(`a="b"`), want: htt.TrustHTMLAttr},
		{name: "html/template.Srcset", value: stdtemplate.Srcset("a.png 2x"), want: htt.TrustSrcset},
		{name: "*html/template.HTML", value: &stdHTML, want: htt.TrustHTML},
		{name: "HTML", value: HTML("<b>"), want: htt.TrustHTML},
		{name: "URL", value: URL("javascript:x"), want: htt.TrustURL},
		{name: "JS", value: JS("x()"), want: htt.TrustJS},
		{name: "JSStr", value: JSStr(`a\"b`), want: htt.TrustJSStr},
		{name: "CSS", value: CSS("color: red"), want: htt.TrustCSS},
		{name: "HTMLAttr", value: HTMLAttr(`a="b"`), want: htt.TrustHTMLAttr},
		{name: "Srcset", value: Srcset("a.png 2x"), want: htt.TrustSrcset},
		{name: "string", value: "<b>", want: htt.TrustNone},
		{name: "html/template.ErrorCode", value: stdtemplate.ErrBadHTML, want: htt.TrustNone},
		{name: "nil", value: nil, want: htt.TrustNone},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := trustOf(reflect.ValueOf(tt.value)); got != tt.want {
				t.Errorf("trustOf(%T(%v)) = %v, want %v", tt.value, tt.value, got, tt.want)
			}
		})
	}
}

// TestExecuteTrustsStdlibHTML states the same through Execute.
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
