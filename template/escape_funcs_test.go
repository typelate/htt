package template_test

import (
	"bytes"
	htmltemplate "html/template"
	"testing"

	"github.com/typelate/htt/template"
)

type stringerValue struct{}

func (stringerValue) String() string { return "<s & 'q'>" }

// TestExportedEscapersMatchHTMLTemplate checks that each exported escaper
// returns what the html/template function of the same name returns.
func TestExportedEscapersMatchHTMLTemplate(t *testing.T) {
	inputs := []string{"\x00", `<a href="x">'&+`, "a\nb c", "é ü", ""}
	argLists := [][]any{
		{1, "<", nil},
		{"<b>"},
		{&[]string{"<p>"}[0]},
		{stringerValue{}},
	}
	for _, in := range inputs {
		t.Run("string "+in, func(t *testing.T) {
			if got, want := template.HTMLEscapeString(in), htmltemplate.HTMLEscapeString(in); got != want {
				t.Errorf("HTMLEscapeString(%q) = %q, want %q", in, got, want)
			}
			if got, want := template.JSEscapeString(in), htmltemplate.JSEscapeString(in); got != want {
				t.Errorf("JSEscapeString(%q) = %q, want %q", in, got, want)
			}
			var got, want bytes.Buffer
			template.HTMLEscape(&got, []byte(in))
			htmltemplate.HTMLEscape(&want, []byte(in))
			if got.String() != want.String() {
				t.Errorf("HTMLEscape(%q) = %q, want %q", in, got.String(), want.String())
			}
			got.Reset()
			want.Reset()
			template.JSEscape(&got, []byte(in))
			htmltemplate.JSEscape(&want, []byte(in))
			if got.String() != want.String() {
				t.Errorf("JSEscape(%q) = %q, want %q", in, got.String(), want.String())
			}
		})
		t.Run("variadic string "+in, func(t *testing.T) {
			args := []any{in}
			if got, want := template.HTMLEscaper(args...), htmltemplate.HTMLEscaper(args...); got != want {
				t.Errorf("HTMLEscaper(%q) = %q, want %q", in, got, want)
			}
			if got, want := template.JSEscaper(args...), htmltemplate.JSEscaper(args...); got != want {
				t.Errorf("JSEscaper(%q) = %q, want %q", in, got, want)
			}
			if got, want := template.URLQueryEscaper(args...), htmltemplate.URLQueryEscaper(args...); got != want {
				t.Errorf("URLQueryEscaper(%q) = %q, want %q", in, got, want)
			}
		})
	}
	for _, args := range argLists {
		t.Run("variadic", func(t *testing.T) {
			if got, want := template.HTMLEscaper(args...), htmltemplate.HTMLEscaper(args...); got != want {
				t.Errorf("HTMLEscaper(%#v) = %q, want %q", args, got, want)
			}
			if got, want := template.JSEscaper(args...), htmltemplate.JSEscaper(args...); got != want {
				t.Errorf("JSEscaper(%#v) = %q, want %q", args, got, want)
			}
			if got, want := template.URLQueryEscaper(args...), htmltemplate.URLQueryEscaper(args...); got != want {
				t.Errorf("URLQueryEscaper(%#v) = %q, want %q", args, got, want)
			}
		})
	}
}
