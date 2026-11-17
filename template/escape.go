package template

import (
	"io"
	texttemplate "text/template"

	"github.com/typelate/htt"
)

// HTMLEscape writes to w the escaped HTML equivalent of the plain text data b.
func HTMLEscape(w io.Writer, b []byte) {
	texttemplate.HTMLEscape(w, b)
}

// HTMLEscapeString returns the escaped HTML equivalent of the plain text data s.
func HTMLEscapeString(s string) string {
	return texttemplate.HTMLEscapeString(s)
}

// HTMLEscaper returns the escaped HTML equivalent of the textual
// representation of its arguments.
func HTMLEscaper(args ...any) string {
	return texttemplate.HTMLEscaper(args...)
}

// JSEscape writes to w the escaped JavaScript equivalent of the plain text data b.
func JSEscape(w io.Writer, b []byte) {
	texttemplate.JSEscape(w, b)
}

// JSEscapeString returns the escaped JavaScript equivalent of the plain text data s.
func JSEscapeString(s string) string {
	return texttemplate.JSEscapeString(s)
}

// JSEscaper returns the escaped JavaScript equivalent of the textual
// representation of its arguments.
func JSEscaper(args ...any) string {
	return texttemplate.JSEscaper(args...)
}

// URLQueryEscaper returns the escaped value of the textual representation
// of its arguments in a form suitable for embedding in a URL query.
func URLQueryEscaper(args ...any) string {
	return texttemplate.URLQueryEscaper(args...)
}

// IsTrue reports whether the value is 'true', in the sense of not the zero
// of its type, and whether the value is valid. This is the definition of
// truth used by if and other such actions.
func IsTrue(val any) (truth, ok bool) {
	return htt.IsTrue(val)
}
