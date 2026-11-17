package htt

import (
	_ "embed"
	"encoding/json"
	"reflect"
	"strings"
	"unicode/utf8"
)

// Trust says which contextual escaping a value skips in HTML mode. The
// kinds match html/template's safe-content types.
type Trust int

const (
	// TrustNone escapes the value for its context.
	TrustNone Trust = iota
	// TrustHTML is an HTML fragment, written as it is in element content.
	TrustHTML
	// TrustURL is a URL, not filtered by scheme.
	TrustURL
	// TrustJS is a JavaScript expression.
	TrustJS
	// TrustJSStr is text for the inside of a JavaScript string literal.
	TrustJSStr
	// TrustCSS is CSS, written as it is in a style element.
	TrustCSS
	// TrustHTMLAttr is an attribute value that skips every attribute
	// filter, or a name where an attribute is written.
	TrustHTMLAttr
	// TrustSrcset is a srcset value, not filtered by scheme.
	TrustSrcset
)

// TrustFunc reports the Trust of a value. A nil TrustFunc trusts nothing.
type TrustFunc func(reflect.Value) Trust

// escapeAction escapes s for the context ctx with the given trust. It
// reports whether the result is already escaped and must be written as it
// is.
func escapeAction(s string, ctx *HTMLContext, trust Trust) (string, bool) {
	if ctx == nil {
		return s, false
	}
	switch ctx.State {
	case "attr":
		return escapeAttrValue(s, ctx, trust), false
	case "comment":
		return "", false
	default: // "text"
		switch ctx.Element {
		case "script":
			return escapeScript(s, ctx, trust), true
		case "style":
			if trust == TrustCSS {
				return s, true
			}
			return escapeCSSText(s), true
		default:
			if trust == TrustHTML {
				return s, true
			}
			// html.Render handles escaping for text nodes.
			return s, false
		}
	}
}

// escapeScript escapes a value in script content: as the content of a
// string or template literal the action is in, and as a quoted string
// where it is in an expression.
func escapeScript(s string, ctx *HTMLContext, trust Trust) string {
	if trust == TrustJS {
		return s
	}
	switch ctx.JSContext {
	case "tpl":
		// TrustJSStr vouches for a value between quotes, not for one
		// that could open a ${...}.
		return escapeJSTmplLit(s)
	case "str":
		if trust == TrustJSStr {
			return s
		}
		return escapeJS(s)
	default:
		// In an expression the value is always a string literal.
		if trust == TrustJSStr {
			return "\"" + s + "\""
		}
		return jsValEscape(s)
	}
}

// escapeAttrValue filters s for the attribute ctx names. Each trust kind
// covers its own class of attribute; TrustHTMLAttr covers all of them.
func escapeAttrValue(s string, ctx *HTMLContext, trust Trust) string {
	if trust == TrustHTMLAttr {
		return s
	}
	switch {
	case isSrcsetAttr(ctx.Attr):
		if trust == TrustSrcset {
			return s
		}
		return filterSrcset(s)
	case isURLAttr(ctx.Attr):
		if trust == TrustURL {
			return s
		}
		return filterURL(s)
	case isJSEventAttr(ctx.Attr):
		if trust == TrustJS || trust == TrustJSStr {
			return "\"" + s + "\""
		}
		return jsValEscape(s)
	default:
		return s
	}
}

// escapeCSSText escapes s for style content, so that it cannot leave a
// declaration, rule, comment or string.
func escapeCSSText(s string) string {
	return jsReplace(s, cssReplacementTable)
}

// escapeJS escapes s for a JavaScript string, as html/template's
// jsStrEscaper does.
func escapeJS(s string) string {
	return jsReplace(s, jsStrReplacementTable)
}

// escapeJSTmplLit escapes s for a backtick template literal, as
// html/template's jsTmplLitEscaper does.
func escapeJSTmplLit(s string) string {
	return jsReplace(s, jsTmplLitReplacementTable)
}

// jsValEscape returns s as a double-quoted JavaScript string.
func jsValEscape(s string) string {
	return "\"" + escapeJS(s) + "\""
}

// jsExprJSONMarshal returns v as JSON for a script expression, as
// html/template's jsValEscaper does, so that a struct becomes an object
// literal. json.Marshal escapes '<', '>' and '&', so the result cannot
// close the script element. It reports false for nil and strings, which
// jsValEscape handles, and for values JSON cannot hold.
func jsExprJSONMarshal(v any) (string, bool) {
	if v == nil {
		return "", false
	}
	switch v.(type) {
	case string, []byte:
		return "", false
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.String:
		return "", false
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return "", false
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", false
	}
	return string(b), true
}

// jsReplace replaces each rune r of s with replacementTable[r], provided that
// r < len(replacementTable). If replacementTable[r] is the empty string then
// no replacement is made. It also replaces runes U+2028 and U+2029.
func jsReplace(s string, replacementTable []string) string {
	var b strings.Builder
	r, w, written := rune(0), 0, 0
	for i := 0; i < len(s); i += w {
		r, w = utf8.DecodeRuneInString(s[i:])
		var repl string
		switch {
		case int(r) < len(lowUnicodeReplacementTable) && lowUnicodeReplacementTable[r] != "":
			repl = lowUnicodeReplacementTable[r]
		case int(r) < len(replacementTable) && replacementTable[r] != "":
			repl = replacementTable[r]
		case r == ' ':
			repl = "\\u2028"
		case r == ' ':
			repl = "\\u2029"
		default:
			continue
		}
		if written == 0 {
			b.Grow(len(s))
		}
		b.WriteString(s[written:i])
		b.WriteString(repl)
		written = i + w
	}
	if written == 0 {
		return s
	}
	b.WriteString(s[written:])
	return b.String()
}

// lowUnicodeReplacementTable replaces the control characters U+0000 through
// U+001F, as html/template's table of the same name does.
var lowUnicodeReplacementTable = []string{
	0:    "\\u0000",
	0x01: "\\u0001",
	0x02: "\\u0002",
	0x03: "\\u0003",
	0x04: "\\u0004",
	0x05: "\\u0005",
	0x06: "\\u0006",
	'\a': "\\u0007",
	'\b': "\\u0008",
	'\t': "\\t",
	'\n': "\\n",
	'\v': "\\u000b", // "\v" == "v" on IE 6.
	'\f': "\\f",
	'\r': "\\r",
	0x0e: "\\u000e",
	0x0f: "\\u000f",
	0x10: "\\u0010",
	0x11: "\\u0011",
	0x12: "\\u0012",
	0x13: "\\u0013",
	0x14: "\\u0014",
	0x15: "\\u0015",
	0x16: "\\u0016",
	0x17: "\\u0017",
	0x18: "\\u0018",
	0x19: "\\u0019",
	0x1a: "\\u001a",
	0x1b: "\\u001b",
	0x1c: "\\u001c",
	0x1d: "\\u001d",
	0x1e: "\\u001e",
	0x1f: "\\u001f",
}

// jsStrReplacementTable is html/template's table of the same name.
var jsStrReplacementTable = []string{
	0:    "\\u0000",
	'\t': "\\t",
	'\n': "\\n",
	'\v': "\\u000b",
	'\f': "\\f",
	'\r': "\\r",
	// Encode HTML specials as hex so the output can be embedded
	// in HTML attributes without further encoding.
	'"':  "\\u0022",
	'`':  "\\u0060",
	'&':  "\\u0026",
	'\'': "\\u0027",
	'+':  "\\u002b",
	'/':  "\\/",
	'<':  "\\u003c",
	'>':  "\\u003e",
	'\\': "\\\\",
}

// jsTmplLitReplacementTable is jsStrReplacementTable with '$', '{' and
// '}', which open and close an interpolation in a template literal. It is
// html/template's jsBqStrReplacementTable.
var jsTmplLitReplacementTable = []string{
	0:    "\\u0000",
	'\t': "\\t",
	'\n': "\\n",
	'\v': "\\u000b",
	'\f': "\\f",
	'\r': "\\r",
	// Encode HTML specials as hex so the output can be embedded
	// in HTML attributes without further encoding.
	'"':  "\\u0022",
	'`':  "\\u0060",
	'&':  "\\u0026",
	'\'': "\\u0027",
	'+':  "\\u002b",
	'/':  "\\/",
	'<':  "\\u003c",
	'>':  "\\u003e",
	'\\': "\\\\",
	'$':  "\\u0024",
	'{':  "\\u007b",
	'}':  "\\u007d",
}

// cssReplacementTable hex-escapes the characters that are special in CSS.
// A hex escape runs to the first character that is not a hex digit, so
// each ends with a space.
var cssReplacementTable = []string{
	0:    "\\0 ",
	'\t': "\\9 ",
	'\n': "\\a ",
	'\v': "\\b ",
	'\f': "\\c ",
	'\r': "\\d ",
	'"':  "\\22 ",
	'\'': "\\27 ",
	'(':  "\\28 ",
	')':  "\\29 ",
	'*':  "\\2a ",
	'+':  "\\2b ",
	',':  "\\2c ",
	'/':  "\\2f ",
	':':  "\\3a ",
	';':  "\\3b ",
	'<':  "\\3c ",
	'=':  "\\3d ",
	'>':  "\\3e ",
	'@':  "\\40 ",
	'\\': "\\\\",
	'{':  "\\7b ",
	'}':  "\\7d ",
	'`':  "\\60 ",
	'&':  "\\26 ",
}

// filterURL returns s, or "#ZgotmplZ" when s has a scheme that is not
// safe, as html/template does.
func filterURL(s string) string {
	s = strings.TrimSpace(s)
	if isSafeURL(s) {
		return s
	}
	return "#ZgotmplZ"
}

// filterSrcset applies filterURL to the URL of each comma-separated entry
// of a srcset value.
func filterSrcset(s string) string {
	parts := strings.Split(s, ",")
	for i, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		urlPart, descriptor, hasDesc := strings.Cut(trimmed, " ")
		filtered := filterURL(urlPart)
		if hasDesc {
			parts[i] = filtered + " " + strings.TrimSpace(descriptor)
		} else {
			parts[i] = filtered
		}
	}
	return strings.Join(parts, ", ")
}

// isSafeURL reports whether s is relative or has the scheme http, https or
// mailto, the ones html/template allows.
func isSafeURL(s string) bool {
	if protocol, _, ok := strings.Cut(s, ":"); ok && !strings.Contains(protocol, "/") {
		if !strings.EqualFold(protocol, "http") && !strings.EqualFold(protocol, "https") && !strings.EqualFold(protocol, "mailto") {
			return false
		}
	}
	return true
}

// isURLAttr reports whether attr holds a URL.
func isURLAttr(attr string) bool {
	switch strings.ToLower(attr) {
	case "href", "src", "action", "formaction", "cite", "data", "poster",
		"codebase", "background", "longdesc", "usemap", "manifest", "icon",
		"ping", "xlink:href":
		return true
	}
	return false
}

// isSrcsetAttr reports whether attr holds a comma-separated list of URLs.
func isSrcsetAttr(attr string) bool {
	switch strings.ToLower(attr) {
	case "srcset", "imagesrcset":
		return true
	}
	return false
}

// isJSEventAttr reports whether attr is an event handler listed in
// js_event_attrs.json. A name that only starts with "on" is not one.
func isJSEventAttr(attr string) bool {
	_, ok := jsEventAttrs[strings.ToLower(attr)]
	return ok
}

//go:embed js_event_attrs.json
var jsEventAttrsJSON []byte

var jsEventAttrs = loadJSEventAttrs()

func loadJSEventAttrs() map[string]struct{} {
	var names []string
	if err := json.Unmarshal(jsEventAttrsJSON, &names); err != nil {
		panic("template: js_event_attrs.json is malformed: " + err.Error())
	}
	out := make(map[string]struct{}, len(names))
	for _, n := range names {
		out[strings.ToLower(n)] = struct{}{}
	}
	return out
}

// EscapeJS escapes s for the inside of a JavaScript string literal.
func EscapeJS(s string) string {
	return escapeJS(s)
}

// EscapeCSS escapes s for style content.
func EscapeCSS(s string) string {
	return escapeCSSText(s)
}

// FilterURL returns s, or "#ZgotmplZ" when s has a scheme other than
// http, https or mailto.
func FilterURL(s string) string {
	return filterURL(s)
}

// IsURLAttr reports whether the escaper filters values of the named
// attribute as URLs.
func IsURLAttr(attr string) bool {
	return isURLAttr(attr)
}

// IsJSEventAttr reports whether the escaper escapes values of the named
// attribute as JavaScript.
func IsJSEventAttr(attr string) bool {
	return isJSEventAttr(attr)
}

// FilterSrcset applies FilterURL to each URL of a srcset value.
func FilterSrcset(s string) string {
	return filterSrcset(s)
}
