package htt

import (
	"fmt"
	"slices"

	"github.com/typelate/htt/lex"
)

// Severity ranks a Diagnostic.
type Severity int

// The severities.
const (
	SeverityWarning Severity = iota
	SeverityError
)

// String returns "warning" or "error".
func (s Severity) String() string {
	switch s {
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	default:
		return fmt.Sprintf("Severity(%d)", int(s))
	}
}

// DiagnosticCode names a class of finding. Codes are stable.
type DiagnosticCode string

// The codes for HTML structure. The parser reports DiagOrphanEndTag,
// DiagMisnestedTag and DiagUnescapedAmpersand; the others are for analyses
// built on the tree. An analysis of anything else declares its own codes.
const (
	DiagUnclosedElement    DiagnosticCode = "unclosed-element"
	DiagOrphanEndTag       DiagnosticCode = "orphan-end-tag"
	DiagMisnestedTag       DiagnosticCode = "misnested-tag"    // adoption agency (WHATWG §13.2.6.4.7)
	DiagFosteredContent    DiagnosticCode = "fostered-content" // foster parenting (§13.2.6.4.9)
	DiagNestedForm         DiagnosticCode = "nested-form"      // §13.2.6.4.7 form pointer
	DiagAutoClosed         DiagnosticCode = "auto-closed"      // implied end tags (§13.2.6.3)
	DiagNestedLink         DiagnosticCode = "nested-link"
	DiagNestedButton       DiagnosticCode = "nested-button"
	DiagUnescapedAmpersand DiagnosticCode = "unescaped-ampersand"
)

// Diagnostic is a finding that does not stop the parse: a place where a
// browser would build a tree other than the one the source spells.
type Diagnostic struct {
	Pos      lex.Pos
	End      lex.Pos
	Severity Severity
	Code     DiagnosticCode
	Message  string
}

// String formats d with its severity and byte offset.
func (d Diagnostic) String() string {
	return fmt.Sprintf("%s at pos %d: %s", d.Severity, d.Pos, d.Message)
}

// Diagnostics is a list of findings.
type Diagnostics []Diagnostic

// SortByPos sorts ds by position, keeping the order of equals.
func (ds Diagnostics) SortByPos() {
	slices.SortStableFunc(ds, func(a, b Diagnostic) int { return int(a.Pos - b.Pos) })
}

// diag records a diagnostic on the tree.
func (t *Tree) diag(pos, end lex.Pos, sev Severity, code DiagnosticCode, format string, args ...any) {
	t.Diagnostics = append(t.Diagnostics, Diagnostic{
		Pos: pos, End: end, Severity: sev, Code: code,
		Message: fmt.Sprintf(format, args...),
	})
}

// Message formats.
const (
	msgOrphanEndTag       = `unmatched </%s> is ignored by browsers`
	msgMisnestedClose     = `</%s> also closes <%s>; browsers reopen a copy of <%s> after it`
	msgUnescapedAmpersand = `& before an action starts a character reference its value can complete; write &amp;`
)
