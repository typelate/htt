package template

import (
	"reflect"

	"github.com/typelate/htt"
)

// contentTrust is the Trust of each safe-content type, by name. The names
// are the same in this package and in the standard library's
// html/template.
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

// stdlibPkgPath is the standard library's html/template. Code running
// this package in process may pass its content types, which are distinct
// from this package's; they are recognized by name, since importing
// html/template here would make it import itself where muxt lays this
// package over it.
const stdlibPkgPath = "html/template"

// trustOf classifies a value's Trust kind for the htt engine. It recognizes
// this package's safe-content types and the standard library's
// html/template ones so they bypass the matching contextual escaper,
// restoring the trust-type semantics promised by the html/template API.
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
