package htt_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	texttemplate "text/template"

	"github.com/typelate/htt"
)

type callArgsV struct {
	N int
}

type callArgsData struct {
	Nil      any
	S        string
	V        callArgsV
	VP       *callArgsV
	NilPtr   func(*int) bool
	NilMap   func(map[string]int) bool
	NilSlice func([]int) bool
	NilFunc  func(func()) bool
	NilChan  func(chan int) bool
	NilIface func(fmt.Stringer) bool
}

// callArgsNilData returns data whose function fields report whether the
// argument they receive is nil. A call builtin needs the function as a value.
func callArgsNilData() *callArgsData {
	return &callArgsData{
		NilPtr:   func(p *int) bool { return p == nil },
		NilMap:   func(m map[string]int) bool { return m == nil },
		NilSlice: func(s []int) bool { return s == nil },
		NilFunc:  func(f func()) bool { return f == nil },
		NilChan:  func(c chan int) bool { return c == nil },
		NilIface: func(s fmt.Stringer) bool { return s == nil },
	}
}

// callArgsFuncs returns the functions the argument tests call. Each returns
// text that shows the argument it received, so a row's output shows what the
// function was given.
func callArgsFuncs() map[string]any {
	return map[string]any{
		"takeInt":  func(x int) string { return fmt.Sprint(x) },
		"takeU16":  func(x uint16) string { return fmt.Sprint(x) },
		"takeU8":   func(x uint8) string { return fmt.Sprint(x) },
		"takeUint": func(x uint) string { return fmt.Sprint(x) },
		"takeI32":  func(x int32) string { return fmt.Sprint(x) },
		"takeF64":  func(x float64) string { return fmt.Sprint(x) },
		"takeC128": func(x complex128) string { return fmt.Sprint(x) },
		"takeBool": func(x bool) string { return fmt.Sprint(x) },
		"takeStr":  func(x string) string { return x },
		"takeAny":  func(x any) string { return fmt.Sprintf("%T", x) },
		"variadicInt": func(xs ...int) string {
			return fmt.Sprint(xs)
		},
		"takePtr":   func(p *int) bool { return p == nil },
		"takeMap":   func(m map[string]int) bool { return m == nil },
		"takeSlice": func(s []int) bool { return s == nil },
		"takeFunc":  func(f func()) bool { return f == nil },
		"takeChan":  func(c chan int) bool { return c == nil },
		"takeIface": func(s fmt.Stringer) bool { return s == nil },
		"takeV": func(v callArgsV) string {
			return fmt.Sprint(v.N)
		},
		"takeVPtr": func(v *callArgsV) string {
			if v == nil {
				return "nil"
			}
			return fmt.Sprint(v.N)
		},
		"takeTwo": func(a callArgsV, b *callArgsV) string {
			return fmt.Sprintf("%d,%d", a.N, b.N)
		},
		"mkV": func() callArgsV { return callArgsV{N: 7} },
	}
}

// callArgsRow is one argument case. Exactly one of want and wantErr applies:
// want is the output, wantErr a substring of the error.
type callArgsRow struct {
	name    string
	input   string
	data    any
	want    string
	wantErr string
}

func stdCallArgs(input string, data any) (string, error) {
	tmpl, err := texttemplate.New("t").Funcs(callArgsFuncs()).Parse(input)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	err = tmpl.Execute(&b, data)
	return b.String(), err
}

func httCallArgs(input string, data any) (string, error) {
	funcs := callArgsFuncs()
	set, err := htt.NewText[any]("t").Functions(funcs).Parse(input)
	if err != nil {
		return "", err
	}
	tree, ok := set.FindTree("t")
	if !ok {
		return "", errors.New("template t not found")
	}
	var b strings.Builder
	err = htt.ExecuteTemplate(&b, tree, data, htt.WithFuncs(funcs), htt.WithFinder(set))
	return b.String(), err
}

// checkCallArgs compares one engine's result with the row. The row is checked
// against text/template first, so a wrong want fails the test, not htt.
func checkCallArgs(t *testing.T, engine string, row callArgsRow, out string, err error) {
	t.Helper()
	if row.wantErr != "" {
		if err == nil || !strings.Contains(err.Error(), row.wantErr) {
			t.Fatalf("%s(%q) error = %v, want error containing %q", engine, row.input, err, row.wantErr)
		}
		return
	}
	if err != nil {
		t.Fatalf("%s(%q) error = %v, want output %q", engine, row.input, err, row.want)
	}
	if out != row.want {
		t.Fatalf("%s(%q) = %q, want %q", engine, row.input, out, row.want)
	}
}

func runCallArgsRows(t *testing.T, rows []callArgsRow) {
	t.Helper()
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			out, err := stdCallArgs(row.input, row.data)
			checkCallArgs(t, "text/template", row, out, err)
			out, err = httCallArgs(row.input, row.data)
			checkCallArgs(t, "htt", row, out, err)
		})
	}
}

func TestNilLiteralArgument(t *testing.T) {
	runCallArgsRows(t, []callArgsRow{
		{name: "nil for int parameter", input: "{{takeInt nil}}", wantErr: "cannot assign nil to int"},
		{name: "nil for variadic int parameter", input: "{{variadicInt 1 nil}}", wantErr: "cannot assign nil to int"},
		{name: "nil for pointer parameter", input: "{{takePtr nil}}", data: &callArgsData{}, want: "true"},
	})
}

func TestNilInterfaceArgument(t *testing.T) {
	runCallArgsRows(t, []callArgsRow{
		{name: "nil interface pipeline to pointer", input: "{{ .Nil | call .NilPtr }}", data: callArgsNilData(), want: "true"},
		{name: "nil interface field to pointer", input: "{{ call .NilPtr .Nil }}", data: callArgsNilData(), want: "true"},
		{name: "nil interface pipeline to map", input: "{{ .Nil | call .NilMap }}", data: callArgsNilData(), want: "true"},
		{name: "nil interface field to slice", input: "{{ call .NilSlice .Nil }}", data: callArgsNilData(), want: "true"},
		{name: "nil interface field to func", input: "{{ call .NilFunc .Nil }}", data: callArgsNilData(), want: "true"},
		{name: "nil interface field to chan", input: "{{ call .NilChan .Nil }}", data: callArgsNilData(), want: "true"},
		{name: "nil interface field to interface", input: "{{ call .NilIface .Nil }}", data: callArgsNilData(), want: "true"},
	})
}

func callArgsConvData() *callArgsData {
	return &callArgsData{S: "x", V: callArgsV{N: 4}, VP: &callArgsV{N: 5}}
}

func TestArgumentConversion(t *testing.T) {
	runCallArgsRows(t, []callArgsRow{
		{name: "untyped int constant to uint16", input: "{{takeU16 3}}", data: callArgsConvData(), want: "3"},
		{name: "addressable value to pointer", input: "{{takeVPtr .V}}", data: callArgsConvData(), want: "4"},
		{name: "pointer to value", input: "{{takeV .VP}}", data: callArgsConvData(), want: "5"},
		{name: "value and addressable value to pointer", input: "{{takeTwo .V .V}}", data: callArgsConvData(), want: "4,4"},
		{name: "pointer to value and pointer", input: "{{takeTwo .VP .VP}}", data: callArgsConvData(), want: "5,5"},
		{name: "string to uint16", input: "{{takeU16 .S}}", data: callArgsConvData(), wantErr: "wrong type for value; expected uint16"},
		{name: "value that is not addressable to pointer", input: "{{takeVPtr mkV}}", data: callArgsConvData(), wantErr: "wrong type for value; expected *htt_test.callArgsV"},
	})
}

func TestConstantArgumentTakesTheParametersKind(t *testing.T) {
	runCallArgsRows(t, []callArgsRow{
		{name: "float constant with integral value to int", input: "{{takeInt 3.0}}", want: "3"},
		{name: "float constant with fraction to int", input: "{{takeInt 3.5}}", wantErr: "expected integer; found 3.5"},
		{name: "integer constant to float64", input: "{{takeF64 3}}", want: "3"},
		{name: "integer constant to uint8", input: "{{takeU8 3}}", want: "3"},
		{name: "negative integer constant to uint", input: "{{takeUint -1}}", wantErr: "expected unsigned integer; found -1"},
		{name: "character constant to int32", input: "{{takeI32 'a'}}", want: "97"},
		{name: "string constant to int", input: `{{takeInt "x"}}`, wantErr: "expected integer; found"},
		{name: "boolean constant to bool", input: "{{takeBool true}}", want: "true"},
		{name: "integer constant to bool", input: "{{takeBool 1}}", wantErr: "expected bool; found 1"},
		{name: "string constant to string", input: `{{takeStr "x"}}`, want: "x"},
		{name: "integer constant to any", input: "{{takeAny 3}}", want: "int"},
		{name: "float constant to any", input: "{{takeAny 3.5}}", want: "float64"},
		{name: "complex constant to complex128", input: "{{takeC128 1+2i}}", want: "(1+2i)"},
	})
}
