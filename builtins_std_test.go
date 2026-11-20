package htt_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	texttemplate "text/template"

	"github.com/typelate/htt"
)

// builtinRow is one template run through text/template and through htt. The
// row states what text/template does; the oracle run checks the row first.
type builtinRow struct {
	name  string
	input string
	data  any
	want  string // output, when err is empty
	stage string // "parse" or "execute", when err is set
	err   string // a phrase the error must contain; empty when the run succeeds
}

// builtinResult is what one engine did with a row.
type builtinResult struct {
	stage string
	out   string
	err   string
}

func runBuiltinStd(input string, data any) builtinResult {
	tmpl, err := texttemplate.New("t").Parse(input)
	if err != nil {
		return builtinResult{stage: "parse", err: err.Error()}
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, data); err != nil {
		return builtinResult{stage: "execute", out: b.String(), err: err.Error()}
	}
	return builtinResult{stage: "", out: b.String()}
}

func runBuiltinHTT(input string, data any) builtinResult {
	set, err := htt.NewText[any]("t").Parse(input)
	if err != nil {
		return builtinResult{stage: "parse", err: err.Error()}
	}
	tree, ok := set.FindTree("t")
	if !ok {
		return builtinResult{stage: "parse", err: errors.New("template t not found").Error()}
	}
	var b strings.Builder
	if err := htt.ExecuteTemplate(&b, tree, data, htt.WithFinder(set)); err != nil {
		return builtinResult{stage: "execute", out: b.String(), err: err.Error()}
	}
	return builtinResult{stage: "", out: b.String()}
}

// checkBuiltinRow compares one engine's result with the row.
func checkBuiltinRow(t *testing.T, engine string, row builtinRow, got builtinResult) {
	t.Helper()
	if row.err != "" {
		if got.err == "" || !strings.Contains(got.err, row.err) {
			t.Fatalf("%s(%q) error = %q, want error containing %q", engine, row.input, got.err, row.err)
		}
		if got.stage != row.stage {
			t.Fatalf("%s(%q) failed at %s, want %s", engine, row.input, got.stage, row.stage)
		}
		return
	}
	if got.err != "" {
		t.Fatalf("%s(%q) error = %q, want output %q", engine, row.input, got.err, row.want)
	}
	if got.out != row.want {
		t.Fatalf("%s(%q) = %q, want %q", engine, row.input, got.out, row.want)
	}
}

func runBuiltinRows(t *testing.T, rows []builtinRow) {
	t.Helper()
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			checkBuiltinRow(t, "text/template", row, runBuiltinStd(row.input, row.data))
			checkBuiltinRow(t, "htt", row, runBuiltinHTT(row.input, row.data))
		})
	}
}

// pipelineIfaceFuncs has echo, which returns its argument with static type any.
var pipelineIfaceFuncs = texttemplate.FuncMap{
	"echo": func(v any) any { return v },
}

// runFuncsStd runs input through text/template with funcs.
func runFuncsStd(input string, data any, funcs texttemplate.FuncMap) builtinResult {
	tmpl, err := texttemplate.New("t").Funcs(funcs).Parse(input)
	if err != nil {
		return builtinResult{stage: "parse", err: err.Error()}
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, data); err != nil {
		return builtinResult{stage: "execute", out: b.String(), err: err.Error()}
	}
	return builtinResult{stage: "", out: b.String()}
}

// runFuncsHTT runs input through htt with fns.
func runFuncsHTT(input string, data any, fns texttemplate.FuncMap) builtinResult {
	funcs := htt.FuncMap(fns)
	set, err := htt.NewText[any]("t").Functions(funcs).Parse(input)
	if err != nil {
		return builtinResult{stage: "parse", err: err.Error()}
	}
	tree, ok := set.FindTree("t")
	if !ok {
		return builtinResult{stage: "parse", err: errors.New("template t not found").Error()}
	}
	var b strings.Builder
	if err := htt.ExecuteTemplate(&b, tree, data, htt.WithFinder(set), htt.WithFuncs(funcs)); err != nil {
		return builtinResult{stage: "execute", out: b.String(), err: err.Error()}
	}
	return builtinResult{stage: "", out: b.String()}
}

// TestPipelineIfaceMatchesTextTemplate: after a command whose static type is
// the empty interface, a pipeline continues with the value inside it, as
// text/template's evalPipeline does.
func TestPipelineIfaceMatchesTextTemplate(t *testing.T) {
	data := diffTextValue("s")
	rows := []builtinRow{
		{name: "field of pipe through echo", input: `{{($ | echo).S}}`, data: data, want: "s"},
		{name: "missing field of pipe through echo", input: `{{($ | echo).X}}`, data: data, stage: "execute", err: "can't evaluate field X in type htt_test.diffTextData"},
		{name: "field of int through echo", input: `{{(0|echo).A}}`, data: data, stage: "execute", err: "can't evaluate field A in type int"},
		{name: "field of pointer through echo", input: `{{(.P | echo).X}}`, data: data, want: "x"},
		{name: "map key of pipe through echo", input: `{{(.M | echo).one}}`, data: data, want: "1"},
		{name: "len of pipe through echo", input: `{{len (.Sl | echo)}}`, data: data, want: "2"},
		{name: "with on nil pointer through echo", input: `{{with $x := echo .Nil}}a{{else}}b{{end}}`, data: data, want: "b"},
		{name: "nil interface through echo", input: `{{echo .E}}`, data: data, want: "<no value>"},
		{name: "parenthesized nil interface through echo", input: `{{(echo .E)}}`, data: data, want: "<no value>"},
		{name: "type of pipe through echo", input: `{{printf "%T" (echo 1)}}`, data: data, want: "int"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			checkBuiltinRow(t, "text/template", row, runFuncsStd(row.input, row.data, pipelineIfaceFuncs))
			checkBuiltinRow(t, "htt", row, runFuncsHTT(row.input, row.data, pipelineIfaceFuncs))
		})
	}
}

// builtinArgV is the struct parameter of argFuncs.
type builtinArgV struct {
	N int
}

// argFuncs take a struct and an interface with a method, which a constant
// can never become.
var argFuncs = texttemplate.FuncMap{
	"argStruct":   func(v builtinArgV) int { return v.N },
	"argStringer": func(s fmt.Stringer) string { return s.String() },
	"argInt":      func(i int) int { return i },
	"argUint":     func(u uint) uint { return u },
	"argFloat":    func(f float64) float64 { return f },
	"argComplex":  func(c complex128) complex128 { return c },
	"argString":   func(s string) string { return s },
	"argSum":      func(xs ...int) int { return len(xs) },
}

// TestArgumentMatchesTextTemplate: a constant that cannot become the
// parameter's type fails with text/template's "can't handle" message, which
// names the constant and the parameter type, not a type mismatch.
func TestArgumentMatchesTextTemplate(t *testing.T) {
	rows := []builtinRow{
		{name: "number for a struct", input: `{{argStruct 0}}`, stage: "execute", err: "can't handle 0 for arg of type htt_test.builtinArgV"},
		{name: "string for a struct", input: `{{argStruct "a"}}`, stage: "execute", err: `can't handle "a" for arg of type htt_test.builtinArgV`},
		{name: "bool for a struct", input: `{{argStruct true}}`, stage: "execute", err: "can't handle true for arg of type htt_test.builtinArgV"},
		{name: "number for an interface with methods", input: `{{argStringer 0}}`, stage: "execute", err: "can't handle 0 for arg of type fmt.Stringer"},
		{name: "string for an interface with methods", input: `{{argStringer "a"}}`, stage: "execute", err: `can't handle "a" for arg of type fmt.Stringer`},
		{name: "bool for an int", input: `{{argInt true}}`, stage: "execute", err: "expected integer; found true"},
		{name: "bool for an int element of a variadic", input: `{{argSum true}}`, stage: "execute", err: "expected integer; found true"},
		{name: "bool for a uint", input: `{{argUint true}}`, stage: "execute", err: "expected unsigned integer; found true"},
		{name: "bool for a float", input: `{{argFloat true}}`, stage: "execute", err: "expected float; found true"},
		{name: "bool for a complex", input: `{{argComplex true}}`, stage: "execute", err: "expected complex; found true"},
		{name: "bool for a string", input: `{{argString true}}`, stage: "execute", err: "expected string; found true"},
		{name: "bool for an interface with methods", input: `{{argStringer true}}`, stage: "execute", err: "can't handle true for arg of type fmt.Stringer"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			checkBuiltinRow(t, "text/template", row, runFuncsStd(row.input, row.data, argFuncs))
			checkBuiltinRow(t, "htt", row, runFuncsHTT(row.input, row.data, argFuncs))
		})
	}
}

type builtinCmpV struct {
	N int
}

type builtinCmpStringer struct {
	n int
}

func (s *builtinCmpStringer) String() string {
	return fmt.Sprint(s.n)
}

type builtinCmpData struct {
	Ptr, NilPtr    *int
	Iface1, Iface2 fmt.Stringer
	Map, NonNilMap map[int]int
	V1, V2, V3     builtinCmpV
	Three          int
	NegOne         int
	Uthree         uint
	S              []int
}

func newBuiltinCmpData() *builtinCmpData {
	one := 1
	return &builtinCmpData{
		Ptr:       &one,
		Iface1:    &builtinCmpStringer{n: 1},
		Iface2:    &builtinCmpStringer{n: 2},
		NonNilMap: map[int]int{},
		V1:        builtinCmpV{N: 1},
		V2:        builtinCmpV{N: 1},
		V3:        builtinCmpV{N: 2},
		Three:     3,
		NegOne:    -1,
		Uthree:    3,
		S:         []int{1},
	}
}

func TestBuiltinComparison(t *testing.T) {
	data := newBuiltinCmpData()
	runBuiltinRows(t, []builtinRow{
		{name: "eq int and float", input: `{{eq 2 2.0}}`, data: data, stage: "execute", err: "incompatible types for comparison"},
		{name: "eq same pointer", input: `{{eq .Ptr .Ptr}}`, data: data, want: "true"},
		{name: "eq pointer and nil pointer", input: `{{eq .Ptr .NilPtr}}`, data: data, want: "false"},
		{name: "eq nil pointers", input: `{{eq .NilPtr .NilPtr}}`, data: data, want: "true"},
		{name: "eq same interface", input: `{{eq .Iface1 .Iface1}}`, data: data, want: "true"},
		{name: "eq different interfaces", input: `{{eq .Iface1 .Iface2}}`, data: data, want: "false"},
		{name: "eq nil map with itself", input: `{{eq .Map .Map}}`, data: data, want: "true"},
		{name: "eq nil map and nil", input: `{{eq .Map nil}}`, data: data, want: "true"},
		{name: "eq nil and nil map", input: `{{eq nil .Map}}`, data: data, want: "true"},
		{name: "eq nil map and non-nil map", input: `{{eq .Map .NonNilMap}}`, data: data, want: "false"},
		{name: "eq equal struct values", input: `{{eq .V1 .V2}}`, data: data, want: "true"},
		{name: "eq unequal struct values", input: `{{eq .V1 .V3}}`, data: data, want: "false"},
		{name: "eq slice", input: `{{eq .S .S}}`, data: data, stage: "execute", err: "non-comparable type"},
		{name: "eq three operands", input: `{{eq 1 1 2}}`, data: data, want: "true"},
		{name: "ne int and uint", input: `{{ne .Three .Uthree}}`, data: data, want: "false"},
		{name: "ne negative int and uint", input: `{{ne .NegOne .Uthree}}`, data: data, want: "true"},
		{name: "lt negative int and uint", input: `{{lt .NegOne .Uthree}}`, data: data, want: "true"},
	})
}

func newBuiltinTruthData() map[string]any {
	return map[string]any{
		"Zero":        0,
		"One":         1,
		"Nil":         nil,
		"EmptyString": "",
		"Slice":       []int{1},
	}
}

func TestBuiltinInterfaceTruth(t *testing.T) {
	data := newBuiltinTruthData()
	runBuiltinRows(t, []builtinRow{
		{name: "and zero in any", input: `{{and .Zero true}}`, data: data, want: "0"},
		{name: "or zero in any", input: `{{or .Zero false}}`, data: data, want: "false"},
		{name: "not zero in any", input: `{{not .Zero}}`, data: data, want: "true"},
		{name: "if not zero in any", input: `{{if not .Zero}}yes{{else}}no{{end}}`, data: data, want: "yes"},
		{name: "and one then zero", input: `{{and .One .Zero}}`, data: data, want: "0"},
		{name: "or zero then one", input: `{{or .Zero .One}}`, data: data, want: "1"},
		{name: "not nil interface", input: `{{not .Nil}}`, data: data, want: "true"},
		{name: "or nil interface", input: `{{or .Nil false}}`, data: data, want: "false"},
		{name: "not empty string in any", input: `{{not .EmptyString}}`, data: data, want: "true"},
		{name: "or empty string in any", input: `{{or .EmptyString "x"}}`, data: data, want: "x"},
		{name: "not non-empty slice in any", input: `{{not .Slice}}`, data: data, want: "false"},
		{name: "and non-empty slice in any", input: `{{and .Slice true}}`, data: data, want: "true"},
	})
}

func newBuiltinCallData() map[string]any {
	return map[string]any{
		"PlusOne": func(n int) int {
			return n + 1
		},
		"Nil":  nil,
		"One":  1,
		"Zero": 0,
	}
}

func TestBuiltinCallInterface(t *testing.T) {
	data := newBuiltinCallData()
	runBuiltinRows(t, []builtinRow{
		{name: "call func in any", input: `{{call .PlusOne 1}}`, data: data, want: "2"},
		{name: "call func in any with field arg", input: `{{call .PlusOne .One}}`, data: data, want: "2"},
		{name: "call nil in any", input: `{{call .Nil 1}}`, data: data, stage: "execute", err: "call of nil"},
		{name: "call int in any", input: `{{call .Zero 1}}`, data: data, stage: "execute", err: "non-function .Zero of type int"},
	})
}

type builtinStrV struct {
	N int
}

func (v *builtinStrV) String() string {
	return fmt.Sprintf("<%d>", v.N)
}

// sentinelWriter fails every write with err, so a run that writes returns it.
type sentinelWriter struct {
	err error
}

func (w sentinelWriter) Write(p []byte) (int, error) {
	return 0, w.err
}

// TestBuiltinWriterError: a failing writer's error comes back as it is, not
// wrapped, with the same text text/template returns.
func TestBuiltinWriterError(t *testing.T) {
	errWriter := errors.New("writer failed")
	for _, input := range []string{"hello", "{{.}}"} {
		t.Run(input, func(t *testing.T) {
			std := texttemplate.Must(texttemplate.New("t").Parse(input))
			wantErr := std.Execute(sentinelWriter{err: errWriter}, "data")
			if wantErr == nil {
				t.Fatalf("text/template.Execute(%q) into a failing writer = nil, want an error", input)
			}
			set, err := htt.NewText[any]("t").Parse(input)
			if err != nil {
				t.Fatalf("htt.Parse(%q) = %v", input, err)
			}
			tree, ok := set.FindTree("t")
			if !ok {
				t.Fatal("template t not found")
			}
			gotErr := htt.ExecuteTemplate(sentinelWriter{err: errWriter}, tree, "data", htt.WithFinder(set))
			if !errors.Is(gotErr, errWriter) {
				t.Errorf("htt.ExecuteTemplate(%q) = %v, want an error that wraps %v", input, gotErr, errWriter)
			}
			if gotErr == nil || gotErr.Error() != wantErr.Error() {
				t.Errorf("htt.ExecuteTemplate(%q) = %v, want %v", input, gotErr, wantErr)
			}
		})
	}
}

type builtinNilT struct {
	X int
}

type builtinNilHolder struct {
	I fmt.Stringer
}

// TestBuiltinNilFieldErrors: a field or key read through a nil pointer or a nil
// interface fails with text/template's wording.
func TestBuiltinNilFieldErrors(t *testing.T) {
	runBuiltinRows(t, []builtinRow{
		{name: "missing field on nil pointer", input: `{{.MissingField}}`, data: (*builtinNilT)(nil), stage: "execute", err: "can't evaluate field MissingField in type *htt_test.builtinNilT"},
		{name: "field on nil pointer", input: `{{.X}}`, data: (*builtinNilT)(nil), stage: "execute", err: "nil pointer evaluating *htt_test.builtinNilT.X"},
		{name: "key on nil map pointer", input: `{{.MissingKey}}`, data: (*map[string]string)(nil), stage: "execute", err: "nil pointer evaluating *map[string]string.MissingKey"},
		{name: "method on nil interface field", input: `{{.I.String}}`, data: builtinNilHolder{}, stage: "execute", err: "nil pointer evaluating fmt.Stringer.String"},
	})
}

func TestBuiltinReflectValueData(t *testing.T) {
	runBuiltinRows(t, []builtinRow{
		{name: "range over reflect.Value data", input: `{{range .}}{{.String}}{{end}}`, data: reflect.ValueOf([]builtinStrV{{N: 1}}), want: "<1>"},
		{name: "index of reflect.Value data", input: `{{with index . 0}}{{.String}}{{end}}`, data: reflect.ValueOf([]builtinStrV{{N: 1}}), want: "<1>"},
		{name: "len of reflect.Value data", input: `{{len .}}`, data: reflect.ValueOf([]builtinStrV{{N: 1}}), want: "1"},
	})
}
