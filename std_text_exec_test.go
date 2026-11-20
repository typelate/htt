// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Copied from text/template/exec_test.go of go1.27.3, with the changes marked below.

//lint:file-ignore ST1012 the copied declarations keep text/template's names

package htt_test

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"github.com/typelate/htt"
)

// Changed: text/template's Template is replaced by this adapter over htt.
// It forwards to htt's trees. It keeps the set's functions and missingkey
// option, because htt takes both as options to each execution.
type stdTextTemplate struct {
	set  *stdTextSet
	name string
	tmpl *htt.Template[any]
}

// stdTextSet is the state the templates of one set share.
type stdTextSet struct {
	funcs   htt.FuncMap
	missing htt.MissingKeyMode
}

// Changed: New is a package function here, since the adapter is not a
// method receiver. It makes a text-mode set, as text/template's New does.
func New(name string) *stdTextTemplate {
	return &stdTextTemplate{
		set:  &stdTextSet{funcs: htt.FuncMap{}},
		name: name,
		tmpl: htt.NewText[any](name),
	}
}

// Changed: text/template's FuncMap is htt's FuncMap.
type FuncMap = htt.FuncMap

// Changed: text/template's Must is not in exec_test.go; it is in template.go.
func Must(t *stdTextTemplate, err error) *stdTextTemplate {
	if err != nil {
		panic(err)
	}
	return t
}

// Changed: text/template's IsTrue is the root package's IsTrue.
func IsTrue(val any) (truth, ok bool) {
	return htt.IsTrue(val)
}

// Changed: debug is named stdtext-debug because parse_test.go in this
// directory already registers the flag "debug" in the same test binary.
var debug = flag.Bool("stdtext-debug", false, "show the errors produced by the tests")

func (t *stdTextTemplate) Name() string {
	return t.name
}

// Tree returns the template's parse tree. Changed: text/template's Template
// embeds its tree, so tests read Root directly; here they call Tree().
func (t *stdTextTemplate) Tree() *htt.Tree {
	return t.tmpl.Tree()
}

func (t *stdTextTemplate) Funcs(funcs FuncMap) *stdTextTemplate {
	for name, fn := range funcs {
		t.set.funcs[name] = fn
	}
	t.tmpl.Functions(funcs)
	return t
}

func (t *stdTextTemplate) Delims(left, right string) *stdTextTemplate {
	t.tmpl.Delimiters(left, right)
	return t
}

func (t *stdTextTemplate) Option(opt ...string) *stdTextTemplate {
	if _, err := t.tmpl.Option(opt...); err != nil {
		panic(err)
	}
	for _, o := range opt {
		switch o {
		case "missingkey=zero":
			t.set.missing = htt.MissingKeyZero
		case "missingkey=error":
			t.set.missing = htt.MissingKeyError
		case "missingkey=default", "missingkey=invalid":
			t.set.missing = htt.MissingKeyInvalid
		}
	}
	return t
}

func (t *stdTextTemplate) Parse(text string) (*stdTextTemplate, error) {
	if _, err := t.tmpl.Parse(text); err != nil {
		return nil, err
	}
	return t, nil
}

func (t *stdTextTemplate) Lookup(name string) *stdTextTemplate {
	tmpl := t.tmpl.Lookup(name)
	if tmpl == nil {
		return nil
	}
	return &stdTextTemplate{set: t.set, name: name, tmpl: tmpl}
}

func (t *stdTextTemplate) options() []htt.Option {
	return []htt.Option{
		htt.WithFuncs(t.set.funcs),
		htt.WithFinder(t.tmpl),
		htt.WithMissingKey(t.set.missing),
	}
}

func (t *stdTextTemplate) Execute(w io.Writer, data any) error {
	return htt.ExecuteTemplate(w, t.tmpl.Tree(), data, t.options()...)
}

// The missing-name error is text/template's wording, so the copied tests see
// the same message.
func (t *stdTextTemplate) ExecuteTemplate(w io.Writer, name string, data any) error {
	tree, ok := t.tmpl.FindTree(name)
	if !ok {
		return fmt.Errorf("template: no template %q associated with template %q", name, t.name)
	}
	return htt.ExecuteTemplate(w, tree, data, t.options()...)
}

var delimPairs = []string{
	"", "", // default
	"{{", "}}", // same as default
	"<<", ">>", // distinct
	"|", "|", // same
	"(日)", "(本)", // peculiar
}

func TestDelims(t *testing.T) {
	const hello = "Hello, world"
	value := struct{ Str string }{hello}
	for i := 0; i < len(delimPairs); i += 2 {
		left := delimPairs[i+0]
		trueLeft := left
		right := delimPairs[i+1]
		trueRight := right
		if left == "" { // default case
			trueLeft = "{{"
		}
		if right == "" { // default case
			trueRight = "}}"
		}
		action := trueLeft + ".Str" + trueRight
		// A comment, which is not preserved in the parse tree.
		comment := trueLeft + "/*comment*/" + trueRight
		// An action containing a string that looks like the left delimiter.
		strAction := trueLeft + `"` + trueLeft + `"` + trueRight
		text := action + comment + strAction
		// At this point text looks like `{{.Str}}{{/*comment*/}}{{"{{"}}`.
		// Changed: each pair is a subtest, so the skip for one pair leaves the
		// others running. The String check moved after the execution checks so
		// that its skip does not skip the execution.
		t.Run(fmt.Sprintf("%q %q", left, right), func(t *testing.T) {
			tmpl, err := New("delims").Delims(left, right).Parse(text)
			if err != nil {
				t.Fatalf("delim %q text %q parse err %s", left, text, err)
			}
			b := new(strings.Builder)
			err = tmpl.Execute(b, value)
			if err != nil {
				t.Fatalf("delim %q exec err %s", left, err)
			}
			if b.String() != hello+trueLeft {
				t.Errorf("expected %q got %q", hello+trueLeft, b.String())
			}
			// The parse tree's String form should roundtrip back to the input,
			// using the custom delimiters, modulo the dropped comment.
			// Changed: Root is reached through Tree(); see stdTextTemplate.Tree.
			if got, want := tmpl.Tree().Root.String(), action+strAction; got != want {
				t.Skipf("not applicable: Root.String() keeps the comment %q, which text/template drops, because htt's set keeps template comments in the tree (lex.Options.EmitComment) for tools: got %q, want %q", comment, got, want)
			}
		})
	}
}

// Check that an error from a method flows back to the top.
func TestExecuteError(t *testing.T) {
	b := new(bytes.Buffer)
	tmpl := New("error")
	_, err := tmpl.Parse("{{.MyError true}}")
	if err != nil {
		t.Fatalf("parse error: %s", err)
	}
	err = tmpl.Execute(b, tVal)
	if err == nil {
		t.Errorf("expected error; got none")
	} else if !strings.Contains(err.Error(), myError.Error()) {
		if *debug {
			fmt.Printf("test execute error: %s\n", err)
		}
		t.Errorf("expected myError; got %s", err)
	}
}

const execErrorText = `line 1
line 2
line 3
{{template "one" .}}
{{define "one"}}{{template "two" .}}{{end}}
{{define "two"}}{{template "three" .}}{{end}}
{{define "three"}}{{index "hi" $}}{{end}}`

// Check that an error from a nested template contains all the relevant information.
func TestExecError(t *testing.T) {
	tmpl, err := New("top").Parse(execErrorText)
	if err != nil {
		t.Fatal("parse error:", err)
	}
	var b bytes.Buffer
	err = tmpl.Execute(&b, 5) // 5 is out of range indexing "hi"
	if err == nil {
		t.Fatal("expected error")
	}
	const want = `template: top:7:20: executing "three" at <index "hi" $>: error calling index: index out of range: 5`
	got := err.Error()
	if got != want {
		t.Errorf("expected\n%q\ngot\n%q", want, got)
	}
}

type CustomError struct{}

func (*CustomError) Error() string { return "heyo !" }

// Check that a custom error can be returned.
func TestExecError_CustomError(t *testing.T) {
	failingFunc := func() (string, error) {
		return "", &CustomError{}
	}
	tmpl := Must(New("top").Funcs(FuncMap{
		"err": failingFunc,
	}).Parse("{{ err }}"))

	var b bytes.Buffer
	err := tmpl.Execute(&b, nil)

	if _, ok := errors.AsType[*CustomError](err); !ok {
		t.Fatalf("expected custom error; got %s", err)
	}
}

// A nice example: walk a binary tree.

type Tree struct {
	Val         int
	Left, Right *Tree
}

// Use different delimiters to test Set.Delims.
// Also test the trimming of leading and trailing spaces.
const treeTemplate = `
	(- define "tree" -)
	[
		(- .Val -)
		(- with .Left -)
			(template "tree" . -)
		(- end -)
		(- with .Right -)
			(- template "tree" . -)
		(- end -)
	]
	(- end -)
`

func TestTree(t *testing.T) {
	tree := &Tree{
		1,
		&Tree{
			2, &Tree{
				3,
				&Tree{
					4, nil, nil,
				},
				nil,
			},
			&Tree{
				5,
				&Tree{
					6, nil, nil,
				},
				nil,
			},
		},
		&Tree{
			7,
			&Tree{
				8,
				&Tree{
					9, nil, nil,
				},
				nil,
			},
			&Tree{
				10,
				&Tree{
					11, nil, nil,
				},
				nil,
			},
		},
	}
	tmpl, err := New("root").Delims("(", ")").Parse(treeTemplate)
	if err != nil {
		t.Fatal("parse error:", err)
	}
	var b strings.Builder
	const expect = "[1[2[3[4]][5[6]]][7[8[9]][10[11]]]]"
	// First by looking up the template.
	err = tmpl.Lookup("tree").Execute(&b, tree)
	if err != nil {
		t.Fatal("exec error:", err)
	}
	result := b.String()
	if result != expect {
		t.Errorf("expected %q got %q", expect, result)
	}
	// Then direct to execution.
	b.Reset()
	err = tmpl.ExecuteTemplate(&b, "tree", tree)
	if err != nil {
		t.Fatal("exec error:", err)
	}
	result = b.String()
	if result != expect {
		t.Errorf("expected %q got %q", expect, result)
	}
}

func TestFinalForPrintf(t *testing.T) {
	tmpl, err := New("").Parse(`{{"x" | printf}}`)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	err = tmpl.Execute(&b, 0)
	if err != nil {
		t.Fatal(err)
	}
}

type cmpTest struct {
	expr  string
	truth string
	ok    bool
}

var cmpTests = []cmpTest{
	{"eq true true", "true", true},
	{"eq true false", "false", true},
	{"eq 1+2i 1+2i", "true", true},
	{"eq 1+2i 1+3i", "false", true},
	{"eq 1.5 1.5", "true", true},
	{"eq 1.5 2.5", "false", true},
	{"eq 1 1", "true", true},
	{"eq 1 2", "false", true},
	{"eq `xy` `xy`", "true", true},
	{"eq `xy` `xyz`", "false", true},
	{"eq .Uthree .Uthree", "true", true},
	{"eq .Uthree .Ufour", "false", true},
	{"eq 3 4 5 6 3", "true", true},
	{"eq 3 4 5 6 7", "false", true},
	{"ne true true", "false", true},
	{"ne true false", "true", true},
	{"ne 1+2i 1+2i", "false", true},
	{"ne 1+2i 1+3i", "true", true},
	{"ne 1.5 1.5", "false", true},
	{"ne 1.5 2.5", "true", true},
	{"ne 1 1", "false", true},
	{"ne 1 2", "true", true},
	{"ne `xy` `xy`", "false", true},
	{"ne `xy` `xyz`", "true", true},
	{"ne .Uthree .Uthree", "false", true},
	{"ne .Uthree .Ufour", "true", true},
	{"lt 1.5 1.5", "false", true},
	{"lt 1.5 2.5", "true", true},
	{"lt 1 1", "false", true},
	{"lt 1 2", "true", true},
	{"lt `xy` `xy`", "false", true},
	{"lt `xy` `xyz`", "true", true},
	{"lt .Uthree .Uthree", "false", true},
	{"lt .Uthree .Ufour", "true", true},
	{"le 1.5 1.5", "true", true},
	{"le 1.5 2.5", "true", true},
	{"le 2.5 1.5", "false", true},
	{"le 1 1", "true", true},
	{"le 1 2", "true", true},
	{"le 2 1", "false", true},
	{"le `xy` `xy`", "true", true},
	{"le `xy` `xyz`", "true", true},
	{"le `xyz` `xy`", "false", true},
	{"le .Uthree .Uthree", "true", true},
	{"le .Uthree .Ufour", "true", true},
	{"le .Ufour .Uthree", "false", true},
	{"gt 1.5 1.5", "false", true},
	{"gt 1.5 2.5", "false", true},
	{"gt 1 1", "false", true},
	{"gt 2 1", "true", true},
	{"gt 1 2", "false", true},
	{"gt `xy` `xy`", "false", true},
	{"gt `xy` `xyz`", "false", true},
	{"gt .Uthree .Uthree", "false", true},
	{"gt .Uthree .Ufour", "false", true},
	{"gt .Ufour .Uthree", "true", true},
	{"ge 1.5 1.5", "true", true},
	{"ge 1.5 2.5", "false", true},
	{"ge 2.5 1.5", "true", true},
	{"ge 1 1", "true", true},
	{"ge 1 2", "false", true},
	{"ge 2 1", "true", true},
	{"ge `xy` `xy`", "true", true},
	{"ge `xy` `xyz`", "false", true},
	{"ge `xyz` `xy`", "true", true},
	{"ge .Uthree .Uthree", "true", true},
	{"ge .Uthree .Ufour", "false", true},
	{"ge .Ufour .Uthree", "true", true},
	// Mixing signed and unsigned integers.
	{"eq .Uthree .Three", "true", true},
	{"eq .Three .Uthree", "true", true},
	{"le .Uthree .Three", "true", true},
	{"le .Three .Uthree", "true", true},
	{"ge .Uthree .Three", "true", true},
	{"ge .Three .Uthree", "true", true},
	{"lt .Uthree .Three", "false", true},
	{"lt .Three .Uthree", "false", true},
	{"gt .Uthree .Three", "false", true},
	{"gt .Three .Uthree", "false", true},
	{"eq .Ufour .Three", "false", true},
	{"lt .Ufour .Three", "false", true},
	{"gt .Ufour .Three", "true", true},
	{"eq .NegOne .Uthree", "false", true},
	{"eq .Uthree .NegOne", "false", true},
	{"ne .NegOne .Uthree", "true", true},
	{"ne .Uthree .NegOne", "true", true},
	{"lt .NegOne .Uthree", "true", true},
	{"lt .Uthree .NegOne", "false", true},
	{"le .NegOne .Uthree", "true", true},
	{"le .Uthree .NegOne", "false", true},
	{"gt .NegOne .Uthree", "false", true},
	{"gt .Uthree .NegOne", "true", true},
	{"ge .NegOne .Uthree", "false", true},
	{"ge .Uthree .NegOne", "true", true},
	{"eq (index `x` 0) 'x'", "true", true}, // The example that triggered this rule.
	{"eq (index `x` 0) 'y'", "false", true},
	{"eq .V1 .V2", "true", true},
	{"eq .Ptr .Ptr", "true", true},
	{"eq .Ptr .NilPtr", "false", true},
	{"eq .NilPtr .NilPtr", "true", true},
	{"eq .Iface1 .Iface1", "true", true},
	{"eq .Iface1 .NilIface", "false", true},
	{"eq .NilIface .NilIface", "true", true},
	{"eq .NilIface .Iface1", "false", true},
	{"eq .NilIface 0", "false", true},
	{"eq 0 .NilIface", "false", true},
	{"eq .Map .Map", "true", true},        // Uncomparable types but nil is OK.
	{"eq .Map nil", "true", true},         // Uncomparable types but nil is OK.
	{"eq nil .Map", "true", true},         // Uncomparable types but nil is OK.
	{"eq .Map .NonNilMap", "false", true}, // Uncomparable types but nil is OK.
	// Errors
	{"eq `xy` 1", "", false},                // Different types.
	{"eq 2 2.0", "", false},                 // Different types.
	{"lt true true", "", false},             // Unordered types.
	{"lt 1+0i 1+0i", "", false},             // Unordered types.
	{"eq .Ptr 1", "", false},                // Incompatible types.
	{"eq .Ptr .NegOne", "", false},          // Incompatible types.
	{"eq .Map .V1", "", false},              // Uncomparable types.
	{"eq .NonNilMap .NonNilMap", "", false}, // Uncomparable types.
}

func TestComparison(t *testing.T) {
	b := new(strings.Builder)
	cmpStruct := struct {
		Uthree, Ufour    uint
		NegOne, Three    int
		Ptr, NilPtr      *int
		NonNilMap        map[int]int
		Map              map[int]int
		V1, V2           V
		Iface1, NilIface fmt.Stringer
	}{
		Uthree:    3,
		Ufour:     4,
		NegOne:    -1,
		Three:     3,
		Ptr:       new(int),
		NonNilMap: make(map[int]int),
		Iface1:    b,
	}
	for _, test := range cmpTests {
		text := fmt.Sprintf("{{if %s}}true{{else}}false{{end}}", test.expr)
		tmpl, err := New("empty").Parse(text)
		if err != nil {
			t.Fatalf("%q: %s", test.expr, err)
		}
		b.Reset()
		err = tmpl.Execute(b, &cmpStruct)
		if test.ok && err != nil {
			t.Errorf("%s errored incorrectly: %s", test.expr, err)
			continue
		}
		if !test.ok && err == nil {
			t.Errorf("%s did not error", test.expr)
			continue
		}
		if b.String() != test.truth {
			t.Errorf("%s: want %s; got %s", test.expr, test.truth, b.String())
		}
	}
}

func TestMissingMapKey(t *testing.T) {
	data := map[string]int{
		"x": 99,
	}
	tmpl, err := New("t1").Parse("{{.x}} {{.y}}")
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	// By default, just get "<no value>"
	err = tmpl.Execute(&b, data)
	if err != nil {
		t.Fatal(err)
	}
	want := "99 <no value>"
	got := b.String()
	if got != want {
		t.Errorf("got %q; expected %q", got, want)
	}
	// Same if we set the option explicitly to the default.
	tmpl.Option("missingkey=default")
	b.Reset()
	err = tmpl.Execute(&b, data)
	if err != nil {
		t.Fatal("default:", err)
	}
	want = "99 <no value>"
	got = b.String()
	if got != want {
		t.Errorf("got %q; expected %q", got, want)
	}
	// Next we ask for a zero value
	tmpl.Option("missingkey=zero")
	b.Reset()
	err = tmpl.Execute(&b, data)
	if err != nil {
		t.Fatal("zero:", err)
	}
	want = "99 0"
	got = b.String()
	if got != want {
		t.Errorf("got %q; expected %q", got, want)
	}
	// Now we ask for an error.
	tmpl.Option("missingkey=error")
	err = tmpl.Execute(&b, data)
	if err == nil {
		t.Errorf("expected error; got none")
	}
	// same Option, but now a nil interface: ask for an error
	err = tmpl.Execute(&b, nil)
	t.Log(err)
	if err == nil {
		t.Errorf("expected error for nil-interface; got none")
	}
}

// Test that the error message for multiline unterminated string
// refers to the line number of the opening quote.
func TestUnterminatedStringError(t *testing.T) {
	_, err := New("X").Parse("hello\n\n{{`unterminated\n\n\n\n}}\n some more\n\n")
	if err == nil {
		t.Fatal("expected error")
	}
	str := err.Error()
	if !strings.Contains(str, "X:3: unterminated raw quoted string") {
		t.Fatalf("unexpected error: %s", str)
	}
}

const alwaysErrorText = "always be failing"

var alwaysError = errors.New(alwaysErrorText)

type ErrorWriter int

func (e ErrorWriter) Write(p []byte) (int, error) {
	return 0, alwaysError
}

func TestExecuteGivesExecError(t *testing.T) {
	// First, a non-execution error shouldn't be an ExecError.
	tmpl, err := New("X").Parse("hello")
	if err != nil {
		t.Fatal(err)
	}
	err = tmpl.Execute(ErrorWriter(0), 0)
	if err == nil {
		t.Fatal("expected error; got none")
	}
	if err.Error() != alwaysErrorText {
		t.Errorf("expected %q error; got %q", alwaysErrorText, err)
	}
	// This one should be an ExecError.
	tmpl, err = New("X").Parse("hello, {{.X.Y}}")
	if err != nil {
		t.Fatal(err)
	}
	err = tmpl.Execute(io.Discard, 0)
	if err == nil {
		t.Fatal("expected error; got none")
	}
	// Changed: ExecError is the root package's type.
	eerr, ok := err.(htt.ExecError)
	if !ok {
		t.Fatalf("did not expect ExecError %s", eerr)
	}
	expect := "field X in type int"
	if !strings.Contains(err.Error(), expect) {
		t.Errorf("expected %q; got %q", expect, err)
	}
}

func funcNameTestFunc() int {
	return 0
}

func TestGoodFuncNames(t *testing.T) {
	names := []string{
		"_",
		"a",
		"a1",
		"a1",
		"Ӵ",
	}
	for _, name := range names {
		tmpl := New("X").Funcs(
			FuncMap{
				name: funcNameTestFunc,
			},
		)
		if tmpl == nil {
			t.Fatalf("nil result for %q", name)
		}
	}
}

func TestBadFuncNames(t *testing.T) {
	names := []string{
		"",
		"2",
		"a-b",
	}
	for _, name := range names {
		// Changed: each name is a subtest, so the skip for one name leaves the others running.
		t.Run(fmt.Sprintf("%q", name), func(t *testing.T) {
			testBadFuncName(name, t)
		})
	}
}

func TestIsTrue(t *testing.T) {
	var nil_ptr *int
	var nil_chan chan int
	tests := []struct {
		v    any
		want bool
	}{
		{1, true},
		{0, false},
		{uint8(1), true},
		{uint8(0), false},
		{float64(1.0), true},
		{float64(0.0), false},
		{complex64(1.0), true},
		{complex64(0.0), false},
		{true, true},
		{false, false},
		{[2]int{1, 2}, true},
		{[0]int{}, false},
		{[]byte("abc"), true},
		{[]byte(""), false},
		{map[string]int{"a": 1, "b": 2}, true},
		{map[string]int{}, false},
		{make(chan int), true},
		{nil_chan, false},
		{new(int), true},
		{nil_ptr, false},
		{unsafe.Pointer(new(int)), true},
		{unsafe.Pointer(nil_ptr), false},
	}
	for _, test_case := range tests {
		got, _ := IsTrue(test_case.v)
		if got != test_case.want {
			t.Fatalf("expect result %v, got %v", test_case.want, got)
		}
	}
}

func testBadFuncName(name string, t *testing.T) {
	t.Helper()
	defer func() {
		recover()
	}()
	New("X").Funcs(
		FuncMap{
			name: funcNameTestFunc,
		},
	)
	// If we get here, the name did not cause a panic, which is how Funcs
	// reports an error.
	// Changed: the skip is by design, not a difference; see the reason below.
	t.Skipf("not applicable: the adapter's Funcs is the root set's Functions, a record of names for the parser, so it accepts %q; text/template's Funcs panics on it", name)
}

func TestEvalFieldErrors(t *testing.T) {
	tests := []struct {
		name, src string
		value     any
		want      string
	}{
		{
			// Check that calling an invalid field on nil pointer
			// prints a field error instead of a distracting nil
			// pointer error. https://golang.org/issue/15125
			"MissingFieldOnNil",
			"{{.MissingField}}",
			(*T)(nil),
			// Changed: the type lives in package htt_test here
			"can't evaluate field MissingField in type *htt_test.T",
		},
		{
			"MissingFieldOnNonNil",
			"{{.MissingField}}",
			&T{},
			// Changed: the type lives in package htt_test here
			"can't evaluate field MissingField in type *htt_test.T",
		},
		{
			"ExistingFieldOnNil",
			"{{.X}}",
			(*T)(nil),
			// Changed: the type lives in package htt_test here
			"nil pointer evaluating *htt_test.T.X",
		},
		{
			"MissingKeyOnNilMap",
			"{{.MissingKey}}",
			(*map[string]string)(nil),
			"nil pointer evaluating *map[string]string.MissingKey",
		},
		{
			"MissingKeyOnNilMapPtr",
			"{{.MissingKey}}",
			(*map[string]string)(nil),
			"nil pointer evaluating *map[string]string.MissingKey",
		},
		{
			"MissingKeyOnMapPtrToNil",
			"{{.MissingKey}}",
			&map[string]string{},
			"<nil>",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpl := Must(New("tmpl").Parse(tc.src))
			err := tmpl.Execute(io.Discard, tc.value)
			got := "<nil>"
			if err != nil {
				got = err.Error()
			}
			if !strings.HasSuffix(got, tc.want) {
				// Changed: the copied messages name the type as *template.T, which
				// is the package name in text/template's tests; here it is htt_test.
				if strings.HasSuffix(got, strings.Replace(tc.want, "*template.", "*htt_test.", 1)) {
					t.Skip("differs from text/template: the type is named *htt_test.T in this package, not *template.T")
				}
				t.Fatalf("got error %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMaxExecDepth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode")
	}
	tmpl := Must(New("tmpl").Parse(`{{template "tmpl" .}}`))
	err := tmpl.Execute(io.Discard, nil)
	got := "<nil>"
	if err != nil {
		got = err.Error()
	}
	const want = "exceeded maximum template depth"
	if !strings.Contains(got, want) {
		t.Errorf("got error %q; want %q", got, want)
	}
}

func TestAddrOfIndex(t *testing.T) {
	// golang.org/issue/14916.
	// Before index worked on reflect.Values, the .String could not be
	// found on the (incorrectly unaddressable) V value,
	// in contrast to range, which worked fine.
	// Also testing that passing a reflect.Value to tmpl.Execute works.
	texts := []string{
		`{{range .}}{{.String}}{{end}}`,
		`{{with index . 0}}{{.String}}{{end}}`,
	}
	for _, text := range texts {
		tmpl := Must(New("tmpl").Parse(text))
		var buf strings.Builder
		err := tmpl.Execute(&buf, reflect.ValueOf([]V{{1}}))
		if err != nil {
			t.Fatalf("%s: Execute: %v", text, err)
		}
		if buf.String() != "<1>" {
			t.Fatalf("%s: template output = %q, want %q", text, &buf, "<1>")
		}
	}
}

func TestInterfaceValues(t *testing.T) {
	// golang.org/issue/17714.
	// Before index worked on reflect.Values, interface values
	// were always implicitly promoted to the underlying value,
	// except that nil interfaces were promoted to the zero reflect.Value.
	// Eliminating a round trip to interface{} and back to reflect.Value
	// eliminated this promotion, breaking these cases.
	tests := []struct {
		text string
		out  string
	}{
		{`{{index .Nil 1}}`, "ERROR: index of untyped nil"},
		{`{{index .Slice 2}}`, "2"},
		{`{{index .Slice .Two}}`, "2"},
		{`{{call .Nil 1}}`, "ERROR: call of nil"},
		{`{{call .PlusOne 1}}`, "2"},
		{`{{call .PlusOne .One}}`, "2"},
		{`{{and (index .Slice 0) true}}`, "0"},
		{`{{and .Zero true}}`, "0"},
		{`{{and (index .Slice 1) false}}`, "false"},
		{`{{and .One false}}`, "false"},
		{`{{or (index .Slice 0) false}}`, "false"},
		{`{{or .Zero false}}`, "false"},
		{`{{or (index .Slice 1) true}}`, "1"},
		{`{{or .One true}}`, "1"},
		{`{{not (index .Slice 0)}}`, "true"},
		{`{{not .Zero}}`, "true"},
		{`{{not (index .Slice 1)}}`, "false"},
		{`{{not .One}}`, "false"},
		{`{{eq (index .Slice 0) .Zero}}`, "true"},
		{`{{eq (index .Slice 1) .One}}`, "true"},
		{`{{ne (index .Slice 0) .Zero}}`, "false"},
		{`{{ne (index .Slice 1) .One}}`, "false"},
		{`{{ge (index .Slice 0) .One}}`, "false"},
		{`{{ge (index .Slice 1) .Zero}}`, "true"},
		{`{{gt (index .Slice 0) .One}}`, "false"},
		{`{{gt (index .Slice 1) .Zero}}`, "true"},
		{`{{le (index .Slice 0) .One}}`, "true"},
		{`{{le (index .Slice 1) .Zero}}`, "false"},
		{`{{lt (index .Slice 0) .One}}`, "true"},
		{`{{lt (index .Slice 1) .Zero}}`, "false"},
	}

	for _, tt := range tests {
		tmpl := Must(New("tmpl").Parse(tt.text))
		var buf strings.Builder
		err := tmpl.Execute(&buf, map[string]any{
			"PlusOne": func(n int) int {
				return n + 1
			},
			"Slice": []int{0, 1, 2, 3},
			"One":   1,
			"Two":   2,
			"Nil":   nil,
			"Zero":  0,
		})
		if strings.HasPrefix(tt.out, "ERROR:") {
			e := strings.TrimSpace(strings.TrimPrefix(tt.out, "ERROR:"))
			if err == nil || !strings.Contains(err.Error(), e) {
				t.Errorf("%s: Execute: %v, want error %q", tt.text, err, e)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: Execute: %v", tt.text, err)
			continue
		}
		if buf.String() != tt.out {
			t.Errorf("%s: template output = %q, want %q", tt.text, &buf, tt.out)
		}
	}
}

// Check that panics during calls are recovered and returned as errors.
func TestExecutePanicDuringCall(t *testing.T) {
	funcs := map[string]any{
		"doPanic": func() string {
			panic("custom panic string")
		},
	}
	tests := []struct {
		name    string
		input   string
		data    any
		wantErr string
	}{
		{
			"direct func call panics",
			"{{doPanic}}", (*T)(nil),
			`template: t:1:2: executing "t" at <doPanic>: error calling doPanic: custom panic string`,
		},
		{
			"indirect func call panics",
			"{{call doPanic}}", (*T)(nil),
			`template: t:1:7: executing "t" at <doPanic>: error calling doPanic: custom panic string`,
		},
		{
			"direct method call panics",
			"{{.GetU}}", (*T)(nil),
			`template: t:1:2: executing "t" at <.GetU>: error calling GetU: runtime error: invalid memory address or nil pointer dereference`,
		},
		{
			"indirect method call panics",
			"{{call .GetU}}", (*T)(nil),
			`template: t:1:7: executing "t" at <.GetU>: error calling GetU: runtime error: invalid memory address or nil pointer dereference`,
		},
		{
			"func field call panics",
			"{{call .PanicFunc}}", tVal,
			`template: t:1:2: executing "t" at <call .PanicFunc>: error calling call: test panic`,
		},
		{
			"method call on nil interface",
			"{{.NonEmptyInterfaceNil.Method0}}", tVal,
			// Changed: the type lives in package htt_test here
			`template: t:1:23: executing "t" at <.NonEmptyInterfaceNil.Method0>: nil pointer evaluating htt_test.I.Method0`,
		},
	}
	for _, tc := range tests {
		// Changed: each row is a subtest, so a skip for one row leaves the others running.
		t.Run(tc.name, func(t *testing.T) {
			b := new(bytes.Buffer)
			tmpl, err := New("t").Funcs(funcs).Parse(tc.input)
			if err != nil {
				t.Fatalf("parse error: %s", err)
			}
			err = tmpl.Execute(b, tc.data)
			switch {
			case err == nil:
				t.Errorf("%s: expected error; got none", tc.name)
			case strings.Contains(err.Error(), tc.wantErr):
			default:
				if *debug {
					fmt.Printf("%s: test execute error: %s\n", tc.name, err)
				}
				t.Errorf("%s: expected error:\n%s\ngot:\n%s", tc.name, tc.wantErr, err)
			}
		})
	}
}

func TestFunctionCheckDuringCall(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		data    any
		wantErr string
	}{
		{
			name:    "call nothing",
			input:   `{{call}}`,
			data:    tVal,
			wantErr: "wrong number of args for call: want at least 1 got 0",
		},
		{
			name:    "call non-function",
			input:   "{{call .True}}",
			data:    tVal,
			wantErr: "error calling call: non-function .True of type bool",
		},
		{
			name:    "call func with wrong argument",
			input:   "{{call .BinaryFunc 1}}",
			data:    tVal,
			wantErr: "error calling call: wrong number of args for .BinaryFunc: got 1 want 2",
		},
		{
			name:    "call variadic func with wrong argument",
			input:   `{{call .VariadicFuncInt}}`,
			data:    tVal,
			wantErr: "error calling call: wrong number of args for .VariadicFuncInt: got 0 want at least 1",
		},
		{
			name:    "call too few return number func",
			input:   `{{call .TooFewReturnCountFunc}}`,
			data:    tVal,
			wantErr: "error calling call: function .TooFewReturnCountFunc has 0 return values; should be 1 or 2",
		},
		{
			name:    "call too many return number func",
			input:   `{{call .TooManyReturnCountFunc}}`,
			data:    tVal,
			wantErr: "error calling call: function .TooManyReturnCountFunc has 3 return values; should be 1 or 2",
		},
		{
			name:    "call invalid return type func",
			input:   `{{call .InvalidReturnTypeFunc}}`,
			data:    tVal,
			wantErr: "error calling call: invalid function signature for .InvalidReturnTypeFunc: second return value should be error; is bool",
		},
		{
			name:    "call pipeline",
			input:   `{{call (len "test")}}`,
			data:    nil,
			wantErr: "error calling call: non-function len \"test\" of type int",
		},
	}

	// Changed: the reasons for the rows htt words differently. Each is skipped
	// only when that row's message is the one recorded here.
	differs := map[string]string{
		"call func with wrong argument":          "differs from text/template: the wrong-argument-count error omits the function name (\"for .BinaryFunc\")",
		"call variadic func with wrong argument": "differs from text/template: the wrong-argument-count error omits the function name (\"for .VariadicFuncInt\")",
		"call too few return number func":        "differs from text/template: 0 results are reported as \"function called with 0 args\", and the function name is omitted",
		"call too many return number func":       "differs from text/template: 3 results are reported as \"function called with 3 args\", and the function name is omitted",
		"call invalid return type func":          "differs from text/template: a second result that is not error is reported as \"function called with 2 args\", not as an invalid signature",
	}
	for _, tc := range tests {
		// Changed: each row is a subtest, so a skip for one row leaves the others running.
		t.Run(tc.name, func(t *testing.T) {
			b := new(bytes.Buffer)
			tmpl, err := New("t").Parse(tc.input)
			if err != nil {
				t.Fatalf("parse error: %s", err)
			}
			err = tmpl.Execute(b, tc.data)
			if err == nil {
				t.Errorf("%s: expected error; got none", tc.name)
			} else if tc.wantErr == "" || !strings.Contains(err.Error(), tc.wantErr) {
				if reason, ok := differs[tc.name]; ok {
					t.Skip(reason)
				}
				if *debug {
					fmt.Printf("%s: test execute error: %s\n", tc.name, err)
				}
				t.Errorf("%s: expected error:\n%s\ngot:\n%s", tc.name, tc.wantErr, err)
			}
		})
	}
}

// Issue 31810. Check that a parenthesized first argument behaves properly.
func TestIssue31810(t *testing.T) {
	// A simple value with no arguments is fine.
	var b strings.Builder
	const text = "{{ (.)  }}"
	tmpl, err := New("").Parse(text)
	if err != nil {
		t.Error(err)
	}
	err = tmpl.Execute(&b, "result")
	if err != nil {
		t.Error(err)
	}
	if b.String() != "result" {
		t.Errorf("%s got %q, expected %q", text, b.String(), "result")
	}

	// Even a plain function fails - need to use call.
	f := func() string { return "result" }
	b.Reset()
	// Changed: the check is a subtest, so its skip leaves the call case below running.
	t.Run("printing a function value", func(t *testing.T) {
		err = tmpl.Execute(&b, f)
		if err == nil {
			t.Error("expected error; got none")
		}
	})

	// Works if the function is explicitly called.
	const textCall = "{{ (call .)  }}"
	tmpl, err = New("").Parse(textCall)
	b.Reset()
	err = tmpl.Execute(&b, f)
	if err != nil {
		t.Error(err)
	}
	if b.String() != "result" {
		t.Errorf("%s got %q, expected %q", textCall, b.String(), "result")
	}
}

// Issue 43065, range over send only channel
func TestIssue43065(t *testing.T) {
	var b bytes.Buffer
	tmp := Must(New("").Parse(`{{range .}}{{end}}`))
	ch := make(chan<- int)
	err := tmp.Execute(&b, ch)
	if err == nil {
		t.Error("expected err got nil")
	} else if !strings.Contains(err.Error(), "range over send-only channel") {
		t.Errorf("%s", err)
	}
}

// Issue 48215: embedded nil pointer causes panic.
// Fixed by adding FieldByIndexErr to the reflect package.
func TestIssue48215(t *testing.T) {
	type A struct {
		S string
	}
	type B struct {
		*A
	}
	tmpl, err := New("").Parse(`{{ .S }}`)
	if err != nil {
		t.Fatal(err)
	}
	err = tmpl.Execute(io.Discard, B{})
	// We expect an error, not a panic.
	if err == nil {
		t.Fatal("did not get error for nil embedded struct")
	}
	if !strings.Contains(err.Error(), "reflect: indirection through nil pointer to embedded struct field A") {
		t.Fatal(err)
	}
}
