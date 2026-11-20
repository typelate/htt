package htt_test

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	texttemplate "text/template"

	"github.com/typelate/htt"
)

// diffTextKnown maps a template text to the cause of its divergence from
// text/template. The target skips these texts so the fuzzer can go on past
// them. Each entry is an open finding in production code.
var diffTextKnown = map[string]string{}

// diffTextFuncs is execTestFuncs without count. count slices a 16-byte string
// in a goroutine of its own for n above 16, and no recover in the target can
// catch that panic, so it kills the fuzz worker.
func diffTextFuncs() map[string]any {
	funcs := execTestFuncs()
	delete(funcs, "count")
	return funcs
}

// diffTextDigits matches five or more digits in a row. A range over a count
// that large would spin both engines.
var diffTextDigits = regexp.MustCompile(`[0-9]{5}`)

// FuzzDiffText runs each template text through text/template and through htt
// in text mode, with the same data and the same functions. text/template must
// parse the text for the case to count. When htt refuses a text that
// text/template parses, that is a failure. When both parse, both must fail or
// both must succeed. A success must write the same bytes. A failure must carry
// the same error string and the same bytes written before the failure.
func FuzzDiffText(f *testing.F) {
	for _, test := range execTests {
		f.Add(test.input, "a'b/c&d=e</")
	}
	for _, src := range diffTextSeeds {
		f.Add(src, "a'b/c&d=e</")
	}
	f.Fuzz(func(t *testing.T, text, s string) {
		// Longer texts make the fuzzer slow without finding new shapes.
		if len(text) > 256 {
			return
		}
		// Recursion and missing definitions are covered by the table tests.
		if strings.Contains(text, "template") || strings.Contains(text, "block") {
			return
		}
		// A huge range count would spin both engines.
		if diffTextDigits.MatchString(text) {
			return
		}
		if _, known := diffTextKnown[text]; known {
			return
		}
		data := diffTextValue(s)
		std := diffTextRunStd(text, data)
		if std.panicked || std.parseErr != nil {
			// A text/template panic leaves no oracle. A parse error means
			// the text is not a template that either engine should run.
			return
		}
		ours := diffTextRunHTT(text, data)
		if ours.panicked {
			t.Fatalf("htt panicked: %v\n%s", ours.panicValue, diffTextReport(text, s, std, ours))
		}
		if ours.parseErr != nil {
			// parse_test.go lowers the parser's nesting limit to 3 for this
			// test binary, as text/template's own parse tests do for theirs.
			if strings.Contains(ours.parseErr.Error(), "max expression depth exceeded") {
				return
			}
			t.Fatalf("htt refuses a text that text/template parses: %v\n%s", ours.parseErr, diffTextReport(text, s, std, ours))
		}
		if (std.execErr == nil) != (ours.execErr == nil) {
			t.Fatalf("one engine fails where the other succeeds\n%s", diffTextReport(text, s, std, ours))
		}
		if std.execErr == nil {
			if ours.out != std.out {
				t.Fatalf("htt and text/template write different bytes\n%s", diffTextReport(text, s, std, ours))
			}
			return
		}
		if ours.execErr.Error() != std.execErr.Error() {
			t.Fatalf("error text differs\n%s", diffTextReport(text, s, std, ours))
		}
		if ours.out != std.out {
			t.Fatalf("htt and text/template write different bytes before failing\n%s", diffTextReport(text, s, std, ours))
		}
	})
}

// diffTextSeeds are templates that use the fields, methods, and builtins of
// diffTextData. None of them contains the words the target skips.
var diffTextSeeds = []string{
	`{{.S}}-{{.I}}-{{.F}}-{{.B}}`,
	`{{range .Sl}}[{{.}}]{{end}}`,
	`{{range $k, $v := .M}}{{$k}}={{$v}};{{end}}`,
	`{{with .P}}{{.X}}{{else}}none{{end}}`,
	`{{with .Nil}}{{.X}}{{else with .S}}{{.}}{{else}}none{{end}}`,
	`{{if and .B (eq .I 3)}}yes{{else}}no{{end}}`,
	`{{or .E "fallback"}} {{not .B}}`,
	`{{index .Sl 1}} {{index .M "one"}}`,
	`{{slice .S 0 1}}{{slice .Sl 1}}`,
	`{{len .Sl}} {{len .S}} {{len .M}}`,
	`{{printf "%d|%s|%q" .I .S .S}}`,
	`{{call .Method0}} {{.Method1 4}} {{call .Method2}}`,
	`{{$x := .I}}{{$x = 9}}{{$x}}`,
	`{{range .Sl}}{{if eq . "b"}}{{break}}{{end}}{{.}}{{end}}`,
	`{{range .Sl}}{{if eq . "a"}}{{continue}}{{end}}{{.}}{{end}}`,
	"{{- .S -}}  x  {{- .I }}",
	`{{lt .I 4}} {{le .F 1.5}} {{ne .S "q"}} {{ge .I 3}}`,
	`{{.St}} {{.E}} {{.Method2}}`,
	`{{with $v := .S}}{{$v}}{{else with .I}}{{.}}{{end}}`,
	`{{range $i, $e := .Sl}}{{$i}}{{$e}}{{else}}empty{{end}}`,
	`{{.P.X}} {{.Nil.X}}`,
	`{{echo .S | printf "%s!"}}`,
	`{{add 1 2 .I}}{{typeOf .M}}`,
	`{{range .I}}{{.}}{{end}}`,
	`{{with .Sl}}{{index . 0 | printf "%v"}}{{end}}`,
}

// diffTextStringer is the value of diffTextData.St.
type diffTextStringer string

func (s diffTextStringer) String() string { return string(s) }

// diffTextInner is the pointee of diffTextData.P and diffTextData.Nil.
type diffTextInner struct {
	X string
}

// diffTextData is the data both engines run each template against.
type diffTextData struct {
	S   string
	I   int
	F   float64
	B   bool
	Sl  []string
	M   map[string]int
	P   *diffTextInner
	Nil *diffTextInner
	E   any
	St  fmt.Stringer
}

// Method0 takes no arguments.
func (d diffTextData) Method0() string { return "m0" }

// Method1 takes one int argument.
func (d diffTextData) Method1(n int) int { return n + d.I }

// Method2 returns an error when S is empty.
func (d diffTextData) Method2() (string, error) {
	if d.S == "" {
		return "", errors.New("method2: empty S")
	}
	return d.S, nil
}

// diffTextValue returns the data for one run. Only S depends on the input.
func diffTextValue(s string) diffTextData {
	return diffTextData{
		S:  s,
		I:  3,
		F:  1.5,
		B:  true,
		Sl: []string{"a", "b"},
		M:  map[string]int{"one": 1},
		P:  &diffTextInner{X: "x"},
		St: diffTextStringer("st"),
	}
}

// diffTextOutcome is what one engine did with one text.
type diffTextOutcome struct {
	parseErr   error
	out        string
	execErr    error
	panicked   bool
	panicValue any
}

// diffTextRunStd runs text through text/template, recovering any panic.
func diffTextRunStd(text string, data diffTextData) (o diffTextOutcome) {
	defer func() {
		if r := recover(); r != nil {
			o.panicked, o.panicValue = true, r
		}
	}()
	tmpl, err := texttemplate.New("z").Funcs(diffTextFuncs()).Parse(text)
	if err != nil {
		o.parseErr = err
		return o
	}
	var b strings.Builder
	o.execErr = tmpl.Execute(&b, data)
	o.out = b.String()
	return o
}

// diffTextRunHTT runs text through htt in text mode, recovering any panic.
func diffTextRunHTT(text string, data diffTextData) (o diffTextOutcome) {
	defer func() {
		if r := recover(); r != nil {
			o.panicked, o.panicValue = true, r
		}
	}()
	funcs := diffTextFuncs()
	set, err := htt.NewText[any]("z").Functions(funcs).Parse(text)
	if err != nil {
		o.parseErr = err
		return o
	}
	tree, ok := set.FindTree("z")
	if !ok {
		o.parseErr = errors.New("no tree named z")
		return o
	}
	var b bytes.Buffer
	o.execErr = htt.ExecuteTemplate(&b, tree, data, htt.WithFuncs(funcs), htt.WithFinder(set))
	o.out = b.String()
	return o
}

// diffTextReport formats both outcomes for a failure message.
func diffTextReport(text, s string, std, ours diffTextOutcome) string {
	return fmt.Sprintf("  text: %q\n  string: %q\n  text/template: out %q, err %v\n  htt: out %q, err %v",
		text, s, std.out, std.execErr, ours.out, ours.execErr)
}
