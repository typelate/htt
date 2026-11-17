package template_test

import (
	"bytes"
	"errors"
	stdhtml "html/template"
	"strings"
	"testing"

	"github.com/typelate/htt/template"
)

// Adversarial inputs for script contexts. Each case first checks that this
// package and html/template agree, then asserts that no data value inside a
// <script> element becomes executable JavaScript.

const bt = "`"

// stepError records which step failed: "Parse" or "Execute".
type stepError struct {
	step string
	err  error
}

func (e *stepError) Error() string { return e.step + ": " + e.err.Error() }

func (e *stepError) Unwrap() error { return e.err }

// rendHTT renders tmpl through the htt shim. It returns the output written and
// the error from Parse or Execute.
func rendHTT(tmpl string, data any) (string, error) {
	tt, err := template.New("t").Parse(tmpl)
	if err != nil {
		return "", &stepError{step: "Parse", err: err}
	}
	var buf bytes.Buffer
	if err := tt.Execute(&buf, data); err != nil {
		return buf.String(), &stepError{step: "Execute", err: err}
	}
	return buf.String(), nil
}

// rendStd renders tmpl through stdlib html/template. stdlib is the reference.
func rendStd(tmpl string, data any) (string, error) {
	st, err := stdhtml.New("t").Parse(tmpl)
	if err != nil {
		return "", &stepError{step: "Parse", err: err}
	}
	var buf bytes.Buffer
	if err := st.Execute(&buf, data); err != nil {
		return buf.String(), &stepError{step: "Execute", err: err}
	}
	return buf.String(), nil
}

// sameRefusal reports whether both engines failed at the same step, and for
// an Execute failure with the same ErrorCode when html/template gives one.
func sameRefusal(stdErr, httErr error) bool {
	var se, he *stepError
	if !errors.As(stdErr, &se) || !errors.As(httErr, &he) {
		return false
	}
	if se.step != he.step {
		return false
	}
	if se.step != "Execute" {
		return true
	}
	var sc *stdhtml.Error
	if !errors.As(se.err, &sc) {
		return true
	}
	var hc *template.Error
	return errors.As(he.err, &hc) && int(hc.ErrorCode) == int(sc.ErrorCode)
}

// agree is the differential assertion. Both engines must refuse tmpl at the
// same step, or both must render the same bytes. It returns the htt output and
// whether html/template refused, in which case the caller skips its output
// assertions.
func agree(t *testing.T, tmpl string, stdData, httData any) (got string, refused bool) {
	t.Helper()
	want, stdErr := rendStd(tmpl, stdData)
	got, httErr := rendHTT(tmpl, httData)
	if stdErr != nil {
		if !sameRefusal(stdErr, httErr) {
			t.Errorf("refusal differs\n  tmpl: %s\n  data: %#v\n  htt:  %q, err %v\n  std:  %q, err %v",
				tmpl, httData, got, httErr, want, stdErr)
			return "", true
		}
		t.Logf("html/template and htt both refuse: %v", stdErr)
		return "", true
	}
	if httErr != nil {
		t.Errorf("htt refuses what html/template renders\n  tmpl: %s\n  data: %#v\n  htt:  err %v\n  std:  %q",
			tmpl, httData, httErr, want)
		return "", false
	}
	if got != want {
		t.Errorf("htt and html/template write different bytes\n  tmpl: %s\n  data: %#v\n  htt:  %q\n  std:  %q",
			tmpl, httData, got, want)
	}
	return got, false
}

// mustNotContain fails when got contains the payload, meaning the data value
// reached the browser as code. When html/template's own output also contains
// the payload, the case is skipped as an html/template parity finding.
func mustNotContain(t *testing.T, tmpl string, data any, got, payload string) {
	t.Helper()
	if !strings.Contains(got, payload) {
		return
	}
	std, _ := rendStd(tmpl, data)
	if strings.Contains(std, payload) {
		t.Skipf("html/template parity: html/template also lets %q through\n  tmpl: %s\n  data: %#v\n  htt:  %s\n  std:  %s",
			payload, tmpl, data, got, std)
	}
	t.Errorf("script injection escaped its context\n  tmpl: %s\n  data: %#v\n  htt:  %s\n  std:  %s\n  htt output contains executable %q; want it neutralized",
		tmpl, data, got, std, payload)
}

type condVal struct {
	C bool
	X string
}

// Hypothesis: a {{if}} body that closes a template literal leaves the next
// action annotated as template-literal text.

func TestAttack_BranchClosesTemplateLiteral(t *testing.T) {
	tmpl := "<script>var a = " + bt + "{{if .C}}" + bt + "{{end}}{{.X}};</script>"
	data := condVal{C: true, X: ";alert(1)"}
	got, refused := agree(t, tmpl, data, data)
	if refused {
		return
	}
	mustNotContain(t, tmpl, data, got, ";alert(1);")
}

// Hypothesis: a {{with}} body that closes a template literal leaves the next
// action annotated as template-literal text.

func TestAttack_WithBranchClosesTemplateLiteral(t *testing.T) {
	tmpl := "<script>var a = " + bt + "{{with .X}}" + bt + "{{end}}{{.X}};</script>"
	data := condVal{C: true, X: ";alert(1)"}
	got, refused := agree(t, tmpl, data, data)
	if refused {
		return
	}
	mustNotContain(t, tmpl, data, got, ";alert(1);")
}

// Hypothesis: a {{if}} body that opens a template literal leaves the next
// action annotated with the expression escaper, which keeps ${ live.

func TestAttack_BranchOpensTemplateLiteral(t *testing.T) {
	tmpl := "<script>{{if .C}}var a = " + bt + "{{end}}{{.X}}" + bt + ";</script>"
	data := condVal{C: true, X: "${alert(1)}"}
	got, refused := agree(t, tmpl, data, data)
	if refused {
		return
	}
	mustNotContain(t, tmpl, data, got, "${alert(1)}")
}

// Hypothesis: branches that end in different script contexts are refused or
// annotated for the path taken.

func TestAttack_ElseBranchEndsInDifferentContext(t *testing.T) {
	tmpl := "<script>var a = {{if .C}}" + bt + `{{else}}"{{end}}{{.X}};</script>`
	data := condVal{C: true, X: "${alert(1)}"}
	got, refused := agree(t, tmpl, data, data)
	if refused {
		return
	}
	mustNotContain(t, tmpl, data, got, "${alert(1)}")
}

// Hypothesis: a {{if}} body that opens a ${ interpolation must not leave the
// following action in template-literal text position.

func TestAttack_BranchOpensInterpolation(t *testing.T) {
	tmpl := "<script>var a = " + bt + "{{if .C}}${ {{end}}{{.X}} }" + bt + ";</script>"
	data := condVal{C: true, X: "alert(1)"}
	got, refused := agree(t, tmpl, data, data)
	if refused {
		return
	}
	mustNotContain(t, tmpl, data, got, "${ alert(1) }")
}

// Hypothesis: a backtick inside a regex literal must not open a template
// literal for later actions.

func TestAttack_RegexLiteralContainingBacktick(t *testing.T) {
	tmpl := "<script>var re = /" + bt + "/; alert({{.}});</script>"
	data := "1);alert(2"
	got, refused := agree(t, tmpl, data, data)
	if refused {
		return
	}
	mustNotContain(t, tmpl, data, got, "alert(1);alert(2)")
}

// Hypothesis: a quote inside a regex literal must not open a string for later
// actions.

func TestAttack_RegexLiteralContainingQuote(t *testing.T) {
	tmpl := `<script>var re = /["]/; alert({{.}});</script>`
	data := "1);alert(2"
	got, refused := agree(t, tmpl, data, data)
	if refused {
		return
	}
	mustNotContain(t, tmpl, data, got, "alert(1);alert(2)")
}

// Hypothesis: a legacy <!-- opener inside script must not hide a backtick from
// the scanner.

func TestAttack_HTMLCommentInScriptContainingBacktick(t *testing.T) {
	tmpl := "<script><!-- " + bt + " --> alert({{.}});</script>"
	data := "1);alert(2"
	got, refused := agree(t, tmpl, data, data)
	if refused {
		return
	}
	mustNotContain(t, tmpl, data, got, "alert(1);alert(2)")
}

// Hypothesis: actions in a {{block}} body inside script are escaped as script
// text, so a value cannot close the element.

func TestAttack_BlockInsideScriptIsUnescaped(t *testing.T) {
	tmpl := `<script>{{block "b" .}}var a = {{.}};{{end}}</script>`
	data := `</script><img src=x onerror=alert(1)>`
	got, refused := agree(t, tmpl, data, data)
	if refused {
		return
	}
	mustNotContain(t, tmpl, data, got, `</script><img src=x onerror=alert(1)>`)
}

// Hypothesis: actions in a {{define}} body inside script are escaped as script
// text.

func TestAttack_DefineInsideScriptIsUnescaped(t *testing.T) {
	tmpl := `<script>{{define "b"}}alert({{.}});{{end}}{{template "b" .}}</script>`
	data := "1);alert(2"
	got, refused := agree(t, tmpl, data, data)
	if refused {
		return
	}
	mustNotContain(t, tmpl, data, got, "alert(1);alert(2)")
}

// Hypothesis: a {{template}} call from inside script carries the script context
// into the callee.

func TestAttack_TemplateCallFromScriptIntoOuterDefine(t *testing.T) {
	tmpl := `<script>{{template "b" .}}</script>{{define "b"}}alert({{.}});{{end}}`
	data := "1);alert(2"
	got, refused := agree(t, tmpl, data, data)
	if refused {
		return
	}
	mustNotContain(t, tmpl, data, got, "alert(1);alert(2)")
}

// Hypothesis: a trailing backslash in a double-quoted string must not escape
// the next action's delimiter.

func TestAttack_TrailingBackslashSwallowsStringDelimiter(t *testing.T) {
	tmpl := `<script>var a = "\{{.A}}"; alert({{.B}});</script>`
	data := struct{ A, B string }{A: "", B: ",alert(1),"}
	got, refused := agree(t, tmpl, data, data)
	if refused {
		return
	}
	mustNotContain(t, tmpl, data, got, `,alert(1),`)
}

// Hypothesis: a trailing backslash in a template literal must not escape the
// next action's closing backtick.

func TestAttack_TrailingBackslashSwallowsBacktickDelimiter(t *testing.T) {
	tmpl := "<script>var a = " + bt + `\{{.A}}` + bt + "; alert({{.B}});</script>"
	data := struct{ A, B string }{A: "", B: ",alert(1),"}
	got, refused := agree(t, tmpl, data, data)
	if refused {
		return
	}
	mustNotContain(t, tmpl, data, got, `,alert(1),`)
}

// Hypothesis: an inline event handler that builds a template literal gets the
// template-literal escaper, not the string quoting one.

func TestAttack_TemplateLiteralInJSEventAttribute(t *testing.T) {
	tmpl := `<a onclick="var a = ` + bt + `{{.}}` + bt + `">x</a>`
	data := "${alert(1)}"
	got, refused := agree(t, tmpl, data, data)
	if refused {
		return
	}
	mustNotContain(t, tmpl, data, got, "${alert(1)}")
}

// Hypothesis: sigils and separators inside template literal text are neutralized
// by the template-literal escaper.

func TestClean_TemplateLiteralNeutralizesInterpolation(t *testing.T) {
	cases := []struct {
		name, tmpl string
		data       any
		want       string
	}{
		{
			"interpolation sigils hex-escaped",
			"<script>var a = " + bt + "{{.}}" + bt + ";</script>",
			"${alert(1)}",
			"<script>var a = `\\u0024\\u007balert(1)\\u007d`;</script>",
		},
		{
			"dollar in text immediately before action",
			"<script>var a = " + bt + "${{.}}" + bt + ";</script>",
			"{alert(1)}",
			"<script>var a = `$\\u007balert(1)\\u007d`;</script>",
		},
		{
			"unicode escape for dollar in text before action",
			"<script>var a = " + bt + "\\u0024{{.}}" + bt + ";</script>",
			"{alert(1)}",
			"<script>var a = `\\u0024\\u007balert(1)\\u007d`;</script>",
		},
		{
			"U+2028 and U+2029 escaped",
			"<script>var a = " + bt + "{{.}}" + bt + ";</script>",
			"a b c",
			"<script>var a = `a\\u2028b\\u2029c`;</script>",
		},
		{
			"apostrophe in template literal does not open a string",
			"<script>var a = " + bt + "don't {{.}}" + bt + "; alert({{.}});</script>",
			"1);alert(2",
			"<script>var a = `don't 1);alert(2`; alert(\"1);alert(2\");</script>",
		},
		{
			"backtick inside a quoted string does not open a literal",
			`<script>var a = "` + bt + `{{.}}"; alert({{.}});</script>`,
			"x",
			"<script>var a = \"`x\"; alert(\"x\");</script>",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, refused := agree(t, tc.tmpl, tc.data, tc.data)
			if refused {
				return
			}
			if got != tc.want {
				t.Errorf("render(%q, %#v) = %q, want %q", tc.tmpl, tc.data, got, tc.want)
			}
		})
	}
}

// Hypothesis: actions inside ${ ... } are in expression position and get the
// string escaper, at any nesting depth.

func TestClean_InterpolationIsExpressionPosition(t *testing.T) {
	cases := []struct {
		name, tmpl string
		data       any
		want       string
	}{
		{
			"single interpolation quotes the value",
			"<script>var a = " + bt + "${ {{.}} }" + bt + ";</script>",
			"1);alert(2",
			"<script>var a = `${ \"1);alert(2\" }`;</script>",
		},
		{
			"nested literal inside interpolation",
			"<script>var a = " + bt + "${ " + bt + "${ {{.}} }" + bt + " }" + bt + ";</script>",
			"1);alert(2",
			"<script>var a = `${ `${ \"1);alert(2\" }` }`;</script>",
		},
		{
			"three levels of nesting",
			"<script>var a = " + bt + "${" + bt + "${" + bt + "${ {{.}} }" + bt + "}" + bt + "}" + bt + ";</script>",
			"x",
			"<script>var a = `${`${`${ \"x\" }`}`}`;</script>",
		},
		{
			"object literal braces inside interpolation",
			"<script>var a = " + bt + "${ {k: {{.}}} }" + bt + ";</script>",
			"1);alert(2",
			"<script>var a = `${ {k: \"1);alert(2\"} }`;</script>",
		},
		{
			"range body opening an interpolation each iteration",
			"<script>var a = " + bt + "{{range .}}${ {{.}} }{{end}}" + bt + ";</script>",
			[]string{"1", "2"},
			"<script>var a = `${ \"1\" }${ \"2\" }`;</script>",
		},
		{
			"unbalanced close brace at top level",
			"<script>} } } alert({{.}});</script>",
			"1);alert(2",
			"<script>} } } alert(\"1);alert(2\");</script>",
		},
		{
			"dollar-brace outside any template literal",
			"<script>var re = /a${1}/; alert({{.}});</script>",
			"1);alert(2",
			"<script>var re = /a${1}/; alert(\"1);alert(2\");</script>",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, refused := agree(t, tc.tmpl, tc.data, tc.data)
			if refused {
				return
			}
			if got != tc.want {
				t.Errorf("render(%q, %#v) = %q, want %q", tc.tmpl, tc.data, got, tc.want)
			}
		})
	}
}

// Hypothesis: a {{if}} body that decrements the brace depth must not change the
// depth seen by the action after {{end}}.

func TestClean_BraceDepthStackNotSharedAcrossBranches(t *testing.T) {
	tmpl := "<script>var a = " + bt + "${ { {{if .C}}}{{end}} } {{.X}} } }" + bt + ";</script>"
	for _, c := range []bool{true, false} {
		data := condVal{C: c, X: "alert(1)"}
		got, refused := agree(t, tmpl, data, data)
		if refused {
			continue
		}
		if !strings.Contains(got, `"alert(1)"`) {
			t.Errorf("C=%v: render = %q; want the action quoted (expression position), got a template-literal escape", c, got)
		}
	}
}

// Hypothesis: a backtick, quote or ${ inside a JS comment does not poison the
// scan of the code after it.

func TestClean_CommentsInsideScript(t *testing.T) {
	cases := []struct {
		name, tmpl string
		data       any
	}{
		{
			"backtick in line comment",
			"<script>// see " + bt + "\nvar a = {{.}};</script>",
			"1);alert(2",
		},
		{
			"backtick in block comment",
			"<script>/* " + bt + " */ var a = {{.}};</script>",
			"1);alert(2",
		},
		{
			"quote in line comment",
			"<script>// don't\nvar a = {{.}};</script>",
			"1);alert(2",
		},
		{
			"dollar-brace in line comment",
			"<script>// " + bt + "${ \nalert({{.}});</script>",
			"1);alert(2",
		},
		{
			"line comment inside an interpolation",
			"<script>var a = " + bt + "${ // c\n{{.}} }" + bt + ";</script>",
			"1);alert(2",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agree(t, tc.tmpl, tc.data, tc.data)
		})
	}
}

// Hypothesis: a trusted JSStr or JS value inside a template literal is escaped
// the way html/template escapes it.

func TestClean_TrustInTemplateLiteral(t *testing.T) {
	tmpl := "<script>var a = " + bt + "{{.}}" + bt + ";</script>"

	got, refused := agree(t, tmpl, stdhtml.JSStr("${alert(1)}"), template.JSStr("${alert(1)}"))
	if !refused {
		want := "<script>var a = `\\u0024\\u007balert(1)\\u007d`;</script>"
		if got != want {
			t.Errorf("TrustJSStr in tpl context = %q, want %q", got, want)
		}
	}

	agree(t, tmpl, stdhtml.JS("x + y"), template.JS("x + y"))
}
