package template

import (
	"reflect"

	"github.com/typelate/htt"
)

// contentTrust is the Trust of each safe-content type, by name. The names
// are html/template's.
var contentTrust = map[string]htt.Trust{
	"HTML":     htt.TrustHTML,
	"URL":      htt.TrustURL,
	"JS":       htt.TrustJS,
	"JSStr":    htt.TrustJSStr,
	"CSS":      htt.TrustCSS,
	"HTMLAttr": htt.TrustHTMLAttr,
	"Srcset":   htt.TrustSrcset,
}

// pkgPath is this package's import path, as its types report it.
var pkgPath = reflect.TypeFor[HTML]().PkgPath()

// stdlibPkgPath is html/template's import path. Its content types are
// recognized by name, since this package cannot import it.
const stdlibPkgPath = "html/template"

// trustOf returns the Trust of v: that of a safe-content type of this
// package or of html/template, and TrustNone otherwise.
func trustOf(v reflect.Value) htt.Trust {
	if !v.IsValid() {
		return htt.TrustNone
	}
	for v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return htt.TrustNone
		}
		v = v.Elem()
	}
	t := v.Type()
	if t.Kind() != reflect.String || (t.PkgPath() != pkgPath && t.PkgPath() != stdlibPkgPath) {
		return htt.TrustNone
	}
	return contentTrust[t.Name()] // TrustNone for any other name
}
