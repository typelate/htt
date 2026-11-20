package htt_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	texttemplate "text/template"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

type execErrInner struct{ F int }

type execErrData struct {
	X       int
	S       string
	Slice   []int
	P       *execErrInner
	M       map[string]int
	Struct  execErrInner
	NotFunc int
	I       any
}

func (execErrData) Method(a int) string { return "m" }
func (execErrData) Fail() (string, error) {
	return "", errors.New("method failed")
}
func (execErrData) PanicString() string { panic("method panic") }

var execErrFuncs = texttemplate.FuncMap{
	"fail":       func() (string, error) { return "", errors.New("function failed") },
	"panicStr":   func() string { panic("custom panic string") },
	"panicErr":   func() string { panic(errors.New("custom panic error")) },
	"takesInt":   func(i int) int { return i },
	"takesTwo":   func(a, b int) int { return a + b },
	"identity":   func(v any) any { return v },
	"stringsToX": func(s string) string { return s },
}

func execErrFuncNames() map[string]bool {
	names := make(map[string]bool, len(execErrFuncs))
	for name := range execErrFuncs {
		names[name] = true
	}
	return names
}

type execErrRow struct {
	name       string
	text       string
	data       any
	missingErr bool
}

const execErrIndented = `line one
  {{if .X}}a{{end}}
    {{.Missing}}
`

var execErrRows = []execErrRow{
	{name: "field of int", text: `hello, {{.X.Y}}`, data: 0},
	{name: "missing field", text: `{{.Missing}}`, data: execErrData{}},
	{name: "missing key", text: `{{.nope}}`, data: map[string]int{}, missingErr: true},
	{name: "nested missing key", text: `a {{.M.nope}}`, data: execErrData{M: map[string]int{}}, missingErr: true},
	{name: "nil data missing key", text: `{{.nope}}`, data: nil, missingErr: true},
	{name: "index string out of range", text: `{{index "hi" 5}}`},
	{name: "index slice out of range", text: `{{index .Slice 99}}`, data: execErrData{Slice: []int{1}}},
	{name: "len of int", text: `{{len 3}}`},
	{name: "wrong argument count", text: `{{.Method 1 2}}`, data: execErrData{}},
	{name: "method returns error", text: `x{{.Fail}}`, data: execErrData{}},
	{name: "method panics", text: `{{.PanicString}}`, data: execErrData{}},
	{name: "function returns error", text: `{{fail}}`},
	{name: "function panics with string", text: `{{panicStr}}`},
	{name: "function panics with error", text: `{{panicErr}}`},
	{name: "nil pointer field", text: `{{.P.F}}`, data: execErrData{}},
	{name: "range over struct", text: `{{range .Struct}}x{{end}}`, data: execErrData{}},
	{name: "range over float", text: `{{range 1.5}}x{{end}}`},
	{name: "variable field", text: `{{with $x := 3}}{{$x.Y}}{{end}}`},
	{name: "if condition", text: `{{if .X.Y}}a{{end}}`, data: 0},
	{name: "with condition", text: `{{with .X.Y}}a{{end}}`, data: 0},
	{name: "else if condition", text: `{{if .X}}a{{else if .Missing}}b{{end}}`, data: execErrData{}},
	{name: "later command in pipeline", text: `{{.X | printf "%d" | .S}}`, data: execErrData{}},
	{name: "function with failing pipeline", text: `{{printf "%d" .Missing}}`, data: execErrData{}},
	{name: "call of a nested template", text: "{{define \"inner\"}}a\nb\n  {{.Missing}}\n{{end}}{{template \"inner\" .}}", data: execErrData{}},
	{name: "pipeline of a template call", text: "x\n  {{template \"inner\" .Missing}}{{define \"inner\"}}{{end}}", data: execErrData{}},
	{name: "undefined template", text: `a{{template "nope"}}`},
	{name: "three levels of templates", text: "line 1\nline 2\nline 3\n{{template \"one\" .}}\n{{define \"one\"}}{{template \"two\" .}}{{end}}\n{{define \"two\"}}{{template \"three\" .}}{{end}}\n{{define \"three\"}}{{index \"hi\" $}}{{end}}", data: 5},
	{name: "assign to undeclared variable", text: `{{$x = 1}}`},
	{name: "read of undefined variable", text: `{{$x := $x}}`},
	{name: "call of a non-function", text: `{{call .NotFunc}}`, data: execErrData{}},
	{name: "incompatible comparison", text: `{{eq 2 2.0}}`},
	{name: "slice out of range", text: `{{slice "abc" 5}}`},
	{name: "wrong argument type", text: `{{takesInt "a"}}`},
	{name: "number argument for string", text: `{{stringsToX 3}}`},
	{name: "too many arguments", text: `{{takesTwo 1}}`},
	{name: "argument is an error", text: `{{takesInt .Missing}}`, data: execErrData{}},
	{name: "can't print", text: `{{.Fn}}`, data: struct{ Fn func() }{}},
	{name: "map key with arguments", text: `{{.k 1}}`, data: map[string]int{"k": 1}},
	{name: "field with arguments", text: `{{.S 1}}`, data: execErrData{}},
	{name: "unknown field with arguments", text: `{{.Missing 1}}`, data: execErrData{}},
	{name: "nil pointer with arguments", text: `{{.P.F 1}}`, data: execErrData{}},
	{name: "chain of three fields", text: `{{.Struct.F.Y.Z}}`, data: execErrData{}},
	{name: "chain after a parenthesis", text: `{{(.Struct).F.Y}}`, data: execErrData{}},
	{name: "variable chain", text: `{{$x := .Struct}}{{$x.F.Y}}`, data: execErrData{}},
	{name: "spaces inside the action", text: `{{   .X.Y   }}`, data: 0},
	{name: "trim marker on an action", text: "a\n{{- .X.Y -}}\nb", data: 0},
	{name: "nested parenthesized call", text: `{{index (index .Slice 9) 1}}`, data: execErrData{Slice: []int{1}}},
	{name: "parenthesized call", text: `{{(index .Slice 9)}}`, data: execErrData{Slice: []int{1}}},
	{name: "parenthesized argument", text: `{{printf "%d" (.X.Y)}}`, data: 0},
	{name: "range pipeline", text: `{{range .Missing}}x{{end}}`, data: execErrData{}},
	{name: "with declaration", text: `{{with $x := .Missing}}x{{end}}`, data: execErrData{}},
	{name: "range declaration", text: `{{range $i, $e := .Missing}}x{{end}}`, data: execErrData{}},
	{name: "call with a string for an int", text: `{{.Method "a"}}`, data: execErrData{}},
	{name: "call with a float for an int", text: `{{.Method 1.5}}`, data: execErrData{}},
	{name: "nil argument for an int", text: `{{takesInt nil}}`},
	{name: "error method in a pipeline", text: `{{.Fail | printf "%s"}}`, data: execErrData{}},
	{name: "nested call error", text: `{{printf "%s" (fail)}}`},
	{name: "template pipeline in parentheses", text: "{{define \"i\"}}{{end}}{{template \"i\" (.X.Y)}}", data: 0},
	{name: "else with", text: `{{with .X}}a{{else with .Missing}}b{{end}}`, data: execErrData{}},
	{name: "undefined template with spaces and trim markers", text: "a {{-   template   \"nope\"   .X   -}} b", data: execErrData{}},
	{name: "undefined template in a branch", text: `{{if true}}  {{template "nope"}}{{end}}`},
	{name: "undefined template in a range", text: "{{range .Slice}}\n{{template \"nope\" .}}{{end}}", data: execErrData{Slice: []int{1}}},
	{name: "function of a variable", text: `{{$f := .X}}{{$f.Y 1}}`, data: execErrData{}},
	{name: "call of a parenthesized non-function", text: `{{call (len "test")}}`},
	{name: "extra spaces between arguments", text: `{{index  "hi"   5}}`},
	{name: "extra spaces around pipe", text: `{{ index "hi" 5  |   printf "%d" }}`},
	{name: "extra spaces in nested call", text: `{{printf "%s"  ( index  "hi"  5 )}}`},
	{name: "newline in action", text: "{{index\n  \"hi\"\n  5}}"},
	{name: "extra spaces around declaration", text: `{{$x   :=   index "hi" 5}}`},
	{name: "extra spaces with arguments to a non-function", text: `{{1   2}}`},
	{name: "method call spaces", text: `{{.Method   1   2}}`, data: execErrData{}},
	{name: "multiline", text: execErrIndented, data: execErrData{}},
	{name: "action on a later line", text: "a\nb\n   {{.X.Y}}", data: 0},
	{name: "range body", text: "{{range .Slice}}\n  {{.X}}\n{{end}}", data: execErrData{Slice: []int{1}}},
	{name: "range else", text: "{{range .Slice}}a{{else}}{{.Missing}}{{end}}", data: execErrData{}},
	{name: "break in range", text: "{{range .Slice}}{{if .X}}{{break}}{{end}}{{end}}", data: execErrData{Slice: []int{1}}},
	{name: "parenthesized pipeline", text: `{{(.X.Y)}}`, data: 0},
	{name: "chain after parenthesis", text: `{{(.Struct).X}}`, data: execErrData{}},
	{name: "arguments to a non-function", text: `{{1 2}}`},
	{name: "string argument to non-function", text: `{{"a" | .X}}`, data: execErrData{}},
	{name: "variable with arguments", text: `{{$x := 1}}{{$x 2}}`},
	{name: "nil is not a command", text: `{{nil}}`},
	{name: "and short circuit error", text: `{{and .X .Missing}}`, data: execErrData{X: 1}},
	{name: "not an interface method", text: `{{.I.Missing}}`, data: execErrData{}},
	{name: "unexported field", text: `{{.x}}`, data: struct{ x int }{}},
	{name: "number overflow", text: `{{9223372036854775808}}`},
	{name: "block", text: "{{block \"b\" .}}\n {{.Missing}}{{end}}", data: execErrData{}},
	{name: "dollar field", text: `{{$.Missing}}`, data: execErrData{}},
	{name: "unicode before action", text: "héllo {{.Missing}}", data: execErrData{}},
	{name: "trim markers", text: "a  {{- .Missing -}}  b", data: execErrData{}},
	{name: "comment before action", text: "{{/* c */}}{{.Missing}}", data: execErrData{}},
}

// runExecErrStd executes text with text/template and returns its error.
func runExecErrStd(t *testing.T, row execErrRow, text string) error {
	t.Helper()
	tmpl := texttemplate.New("t").Funcs(execErrFuncs)
	if row.missingErr {
		tmpl = tmpl.Option("missingkey=error")
	}
	tmpl, err := tmpl.Parse(text)
	if err != nil {
		t.Fatalf("text/template parse %q: %v", text, err)
	}
	var buf bytes.Buffer
	return tmpl.Execute(&buf, row.data)
}

// runExecErrHTT executes text with htt, in HTML mode or in text mode, and
// returns its error.
func runExecErrHTT(t *testing.T, row execErrRow, text string, html bool) error {
	t.Helper()
	opts := lex.Options{
		LeftDelim:  "{{",
		RightDelim: "}}",
		ParseHTML:  html,
		BreakOK:    true,
		ContinueOK: true,
		Functions:  execErrFuncNames(),
	}
	set := make(map[string]*htt.Tree)
	tree, err := htt.ParseTree("t", "t", text, set, opts)
	if err != nil {
		t.Fatalf("htt parse %q: %v", text, err)
	}
	if html {
		htt.AnnotateHTMLContexts(tree.Root)
	}
	finder := htt.FindTreeFunc(func(name string) (*htt.Tree, bool) {
		tr, ok := set[name]
		return tr, ok
	})
	execOpts := []htt.Option{htt.WithFinder(finder), htt.WithFuncs(htt.FuncMap(execErrFuncs))}
	if row.missingErr {
		execOpts = append(execOpts, htt.WithMissingKey(htt.MissingKeyError))
	}
	var buf bytes.Buffer
	return htt.ExecuteTemplate(&buf, tree, row.data, execOpts...)
}

// execErrorDivergences lists the rows, by template text, whose htt error
// differs from text/template's, with the cause. The test fails when a
// listed row starts to match.
var execErrorDivergences = map[string]string{
	`{{.Fn}}`: causePrintFunc,
}

// causePrintFunc: text/template refuses to print a func or chan value and
// words the error with the action ("can't print {{.Fn}} of type func()");
// htt prints it as fmt.Sprint does and succeeds. Printing is not part of
// this leaf.
const causePrintFunc = "htt prints a func value where text/template fails with can't print"

func TestExecutionErrorsMatchTextTemplate(t *testing.T) {
	for _, row := range execErrRows {
		t.Run(row.name, func(t *testing.T) {
			stdErr := runExecErrStd(t, row, row.text)
			if stdErr == nil {
				t.Fatalf("text/template executed %q without an error", row.text)
			}
			var stdExec texttemplate.ExecError
			if !errors.As(stdErr, &stdExec) {
				t.Fatalf("text/template error %q is not an ExecError", stdErr)
			}
			err := runExecErrHTT(t, row, row.text, false)
			cause, diverges := execErrorDivergences[row.text]
			if err == nil {
				if !diverges {
					t.Fatalf("htt executed %q without an error; text/template: %q", row.text, stdErr)
				}
				return
			}
			matches := err.Error() == stdErr.Error()
			var gotExec texttemplate.ExecError
			if !errors.As(err, &gotExec) {
				matches = false
			} else if gotExec.Name != stdExec.Name {
				matches = false
			}
			switch {
			case diverges && matches:
				t.Errorf("%q now matches text/template; remove it from execErrorDivergences (%s)", row.text, cause)
			case !diverges && !matches:
				t.Errorf("Execute(%q)\n got %T %q\nwant %T %q", row.text, err, err, stdErr, stdErr)
			}
		})
	}
}

// execErrHTMLWrappers are rows wrapped in markup. {{TEMPLATE}} marks where the
// row's text goes.
var execErrHTMLWrappers = []struct {
	name string
	row  string // name of a row in execErrRows
	wrap string
}{
	{"paragraph", "field of int", "<p>\n  {{TEMPLATE}}\n</p>"},
	{"attribute value", "missing field", `<a title="x {{TEMPLATE}} y">z</a>`},
	{"branch around an element", "if condition", "<ul>{{if true}}<li>{{TEMPLATE}}</li>{{end}}</ul>"},
	{"method error in a span", "method returns error", "<div>\n<span>{{TEMPLATE}}</span></div>"},
	{"function error in a list", "function returns error", "<ul><li>a</li>\n<li>{{TEMPLATE}}</li></ul>"},
	{"spaced call in a heading", "extra spaces between arguments", "<h1>\n  {{TEMPLATE}}</h1>"},
	{"undefined template in a div", "undefined template", "<div class=\"a\">{{TEMPLATE}}</div>"},
	{"range in a table", "range body", "<table>{{TEMPLATE}}</table>"},
}

func TestHTMLExecutionErrorsMatchTextTemplate(t *testing.T) {
	rows := make(map[string]execErrRow, len(execErrRows))
	for _, row := range execErrRows {
		rows[row.name] = row
	}
	for _, w := range execErrHTMLWrappers {
		t.Run(w.name, func(t *testing.T) {
			row, ok := rows[w.row]
			if !ok {
				t.Fatalf("no row named %q", w.row)
			}
			text := strings.Replace(w.wrap, "{{TEMPLATE}}", row.text, 1)
			stdErr := runExecErrStd(t, row, text)
			if stdErr == nil {
				t.Fatalf("text/template executed %q without an error", text)
			}
			want := stdErr.Error()
			cut := strings.Index(want, " at <")
			msgAt := strings.LastIndex(want, ">: ")
			if cut < 0 || msgAt < 0 {
				t.Fatalf("text/template error %q has no location prefix", want)
			}
			prefix, message := want[:cut+len(" at <")], want[msgAt:]
			err := runExecErrHTT(t, row, text, true)
			if err == nil {
				t.Fatalf("htt executed %q without an error; text/template: %q", text, want)
			}
			got := err.Error()
			if !strings.HasPrefix(got, prefix) || !strings.HasSuffix(got, message) {
				t.Errorf("Execute(%q) in HTML mode\n got %q\nwant prefix %q and suffix %q", text, got, prefix, message)
			}
		})
	}
}
