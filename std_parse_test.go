// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Copied from text/template/parse/parse_test.go of go1.27.3, with the changes
// marked below, and from the inputs of text/template/parse/lex_test.go of the
// same release. Each row goes through text/template's parser and through
// htt.ParseTree. A row where the two disagree is skipped with its details
// rather than failed; the skip message starts with the difference's class:
//
//	[1] htt accepts what text/template refuses
//	[2] htt refuses what text/template accepts
//	[3] round trip differs
//	[4] error text or line differs
//	[5] a behavior test's expectation differs

//lint:file-ignore U1000,ST1008,ST1012 the copied declarations stay as the standard library has them

package htt_test

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"text/template/parse"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

// Changed: prefixed ttp so the names do not collide with other files of package htt_test.
const (
	ttpNoError  = true
	ttpHasError = false
)

type ttpParseTest struct {
	name   string
	input  string
	ok     bool
	result string // what the user would see in an error message.
}

// Changed: renamed from parseTests. The rows are verbatim.
var ttpParseTests = []ttpParseTest{
	{
		"empty", "", ttpNoError,
		``,
	},
	{
		"comment", "{{/*\n\n\n*/}}", ttpNoError,
		``,
	},
	{
		"spaces", " \t\n", ttpNoError,
		`" \t\n"`,
	},
	{
		"text", "some text", ttpNoError,
		`"some text"`,
	},
	{
		"emptyAction", "{{}}", ttpHasError,
		`{{}}`,
	},
	{
		"field", "{{.X}}", ttpNoError,
		`{{.X}}`,
	},
	{
		"simple command", "{{printf}}", ttpNoError,
		`{{printf}}`,
	},
	{
		"$ invocation", "{{$}}", ttpNoError,
		"{{$}}",
	},
	{
		"variable invocation", "{{with $x := 3}}{{$x 23}}{{end}}", ttpNoError,
		"{{with $x := 3}}{{$x 23}}{{end}}",
	},
	{
		"variable with fields", "{{$.I}}", ttpNoError,
		"{{$.I}}",
	},
	{
		"multi-word command", "{{printf `%d` 23}}", ttpNoError,
		"{{printf `%d` 23}}",
	},
	{
		"pipeline", "{{.X|.Y}}", ttpNoError,
		`{{.X | .Y}}`,
	},
	{
		"pipeline with decl", "{{$x := .X|.Y}}", ttpNoError,
		`{{$x := .X | .Y}}`,
	},
	{
		"nested pipeline", "{{.X (.Y .Z) (.A | .B .C) (.E)}}", ttpNoError,
		`{{.X (.Y .Z) (.A | .B .C) (.E)}}`,
	},
	{
		"field applied to parentheses", "{{(.Y .Z).Field}}", ttpNoError,
		`{{(.Y .Z).Field}}`,
	},
	{
		"simple if", "{{if .X}}hello{{end}}", ttpNoError,
		`{{if .X}}"hello"{{end}}`,
	},
	{
		"if with else", "{{if .X}}true{{else}}false{{end}}", ttpNoError,
		`{{if .X}}"true"{{else}}"false"{{end}}`,
	},
	{
		"if with else if", "{{if .X}}true{{else if .Y}}false{{end}}", ttpNoError,
		`{{if .X}}"true"{{else}}{{if .Y}}"false"{{end}}{{end}}`,
	},
	{
		"if else chain", "+{{if .X}}X{{else if .Y}}Y{{else if .Z}}Z{{end}}+", ttpNoError,
		`"+"{{if .X}}"X"{{else}}{{if .Y}}"Y"{{else}}{{if .Z}}"Z"{{end}}{{end}}{{end}}"+"`,
	},
	{
		"simple range", "{{range .X}}hello{{end}}", ttpNoError,
		`{{range .X}}"hello"{{end}}`,
	},
	{
		"chained field range", "{{range .X.Y.Z}}hello{{end}}", ttpNoError,
		`{{range .X.Y.Z}}"hello"{{end}}`,
	},
	{
		"nested range", "{{range .X}}hello{{range .Y}}goodbye{{end}}{{end}}", ttpNoError,
		`{{range .X}}"hello"{{range .Y}}"goodbye"{{end}}{{end}}`,
	},
	{
		"range with else", "{{range .X}}true{{else}}false{{end}}", ttpNoError,
		`{{range .X}}"true"{{else}}"false"{{end}}`,
	},
	{
		"range over pipeline", "{{range .X|.M}}true{{else}}false{{end}}", ttpNoError,
		`{{range .X | .M}}"true"{{else}}"false"{{end}}`,
	},
	{
		"range []int", "{{range .SI}}{{.}}{{end}}", ttpNoError,
		`{{range .SI}}{{.}}{{end}}`,
	},
	{
		"range 1 var", "{{range $x := .SI}}{{.}}{{end}}", ttpNoError,
		`{{range $x := .SI}}{{.}}{{end}}`,
	},
	{
		"range 2 vars", "{{range $x, $y := .SI}}{{.}}{{end}}", ttpNoError,
		`{{range $x, $y := .SI}}{{.}}{{end}}`,
	},
	{
		"range with break", "{{range .SI}}{{.}}{{break}}{{end}}", ttpNoError,
		`{{range .SI}}{{.}}{{break}}{{end}}`,
	},
	{
		"range with continue", "{{range .SI}}{{.}}{{continue}}{{end}}", ttpNoError,
		`{{range .SI}}{{.}}{{continue}}{{end}}`,
	},
	{
		"constants", "{{range .SI 1 -3.2i true false 'a' nil}}{{end}}", ttpNoError,
		`{{range .SI 1 -3.2i true false 'a' nil}}{{end}}`,
	},
	{
		"template", "{{template `x`}}", ttpNoError,
		`{{template "x"}}`,
	},
	{
		"template with arg", "{{template `x` .Y}}", ttpNoError,
		`{{template "x" .Y}}`,
	},
	{
		"with", "{{with .X}}hello{{end}}", ttpNoError,
		`{{with .X}}"hello"{{end}}`,
	},
	{
		"with with else", "{{with .X}}hello{{else}}goodbye{{end}}", ttpNoError,
		`{{with .X}}"hello"{{else}}"goodbye"{{end}}`,
	},
	{
		"with with else with", "{{with .X}}hello{{else with .Y}}goodbye{{end}}", ttpNoError,
		`{{with .X}}"hello"{{else}}{{with .Y}}"goodbye"{{end}}{{end}}`,
	},
	{
		"with else chain", "{{with .X}}X{{else with .Y}}Y{{else with .Z}}Z{{end}}", ttpNoError,
		`{{with .X}}"X"{{else}}{{with .Y}}"Y"{{else}}{{with .Z}}"Z"{{end}}{{end}}{{end}}`,
	},
	// Trimming spaces.
	{"trim left", "x \r\n\t{{- 3}}", ttpNoError, `"x"{{3}}`},
	{"trim right", "{{3 -}}\n\n\ty", ttpNoError, `{{3}}"y"`},
	{"trim left and right", "x \r\n\t{{- 3 -}}\n\n\ty", ttpNoError, `"x"{{3}}"y"`},
	{"trim with extra spaces", "x\n{{-  3   -}}\ny", ttpNoError, `"x"{{3}}"y"`},
	{"comment trim left", "x \r\n\t{{- /* hi */}}", ttpNoError, `"x"`},
	{"comment trim right", "{{/* hi */ -}}\n\n\ty", ttpNoError, `"y"`},
	{"comment trim left and right", "x \r\n\t{{- /* */ -}}\n\n\ty", ttpNoError, `"x""y"`},
	{
		"block definition", `{{block "foo" .}}hello{{end}}`, ttpNoError,
		`{{template "foo" .}}`,
	},

	{"newline in assignment", "{{ $x \n := \n 1 \n }}", ttpNoError, "{{$x := 1}}"},
	{"newline in empty action", "{{\n}}", ttpHasError, "{{\n}}"},
	{"newline in pipeline", "{{\n\"x\"\n|\nprintf\n}}", ttpNoError, `{{"x" | printf}}`},
	{"newline in comment", "{{/*\nhello\n*/}}", ttpNoError, ""},
	{"newline in comment", "{{-\n/*\nhello\n*/\n-}}", ttpNoError, ""},
	{
		"spaces around continue", "{{range .SI}}{{.}}{{ continue }}{{end}}", ttpNoError,
		`{{range .SI}}{{.}}{{continue}}{{end}}`,
	},
	{
		"spaces around break", "{{range .SI}}{{.}}{{ break }}{{end}}", ttpNoError,
		`{{range .SI}}{{.}}{{break}}{{end}}`,
	},

	// Errors.
	{"unclosed action", "hello{{range", ttpHasError, ""},
	{"unmatched end", "{{end}}", ttpHasError, ""},
	{"unmatched else", "{{else}}", ttpHasError, ""},
	{"unmatched else after if", "{{if .X}}hello{{end}}{{else}}", ttpHasError, ""},
	{"multiple else", "{{if .X}}1{{else}}2{{else}}3{{end}}", ttpHasError, ""},
	{"missing end", "hello{{range .x}}", ttpHasError, ""},
	{"missing end after else", "hello{{range .x}}{{else}}", ttpHasError, ""},
	{"undefined function", "hello{{undefined}}", ttpHasError, ""},
	{"undefined variable", "{{$x}}", ttpHasError, ""},
	{"variable undefined after end", "{{with $x := 4}}{{end}}{{$x}}", ttpHasError, ""},
	{"variable undefined in template", "{{template $v}}", ttpHasError, ""},
	{"declare with field", "{{with $x.Y := 4}}{{end}}", ttpHasError, ""},
	{"template with field ref", "{{template .X}}", ttpHasError, ""},
	{"template with var", "{{template $v}}", ttpHasError, ""},
	{"invalid punctuation", "{{printf 3, 4}}", ttpHasError, ""},
	{"multidecl outside range", "{{with $v, $u := 3}}{{end}}", ttpHasError, ""},
	{"too many decls in range", "{{range $u, $v, $w := 3}}{{end}}", ttpHasError, ""},
	{"dot applied to parentheses", "{{printf (printf .).}}", ttpHasError, ""},
	{"adjacent args", "{{printf 3`x`}}", ttpHasError, ""},
	{"adjacent args with .", "{{printf `x`.}}", ttpHasError, ""},
	{"extra end after if", "{{if .X}}a{{else if .Y}}b{{end}}{{end}}", ttpHasError, ""},
	{"break outside range", "{{range .}}{{end}} {{break}}", ttpHasError, ""},
	{"continue outside range", "{{range .}}{{end}} {{continue}}", ttpHasError, ""},
	{"break in range else", "{{range .}}{{else}}{{break}}{{end}}", ttpHasError, ""},
	{"continue in range else", "{{range .}}{{else}}{{continue}}{{end}}", ttpHasError, ""},
	// Other kinds of assignments and operators aren't available yet.
	{"bug0a", "{{$x := 0}}{{$x}}", ttpNoError, "{{$x := 0}}{{$x}}"},
	{"bug0b", "{{$x += 1}}{{$x}}", ttpHasError, ""},
	{"bug0c", "{{$x ! 2}}{{$x}}", ttpHasError, ""},
	{"bug0d", "{{$x % 3}}{{$x}}", ttpHasError, ""},
	// Check the parse fails for := rather than comma.
	{"bug0e", "{{range $x := $y := 3}}{{end}}", ttpHasError, ""},
	// Another bug: variable read must ignore following punctuation.
	{"bug1a", "{{$x:=.}}{{$x!2}}", ttpHasError, ""},                     // ! is just illegal here.
	{"bug1b", "{{$x:=.}}{{$x+2}}", ttpHasError, ""},                     // $x+2 should not parse as ($x) (+2).
	{"bug1c", "{{$x:=.}}{{$x +2}}", ttpNoError, "{{$x := .}}{{$x +2}}"}, // It's OK with a space.
	// Check the range handles assignment vs. declaration properly.
	{"bug2a", "{{range $x := 0}}{{$x}}{{end}}", ttpNoError, "{{range $x := 0}}{{$x}}{{end}}"},
	{"bug2b", "{{range $x = 0}}{{$x}}{{end}}", ttpNoError, "{{range $x = 0}}{{$x}}{{end}}"},
	// dot following a literal value
	{"dot after integer", "{{1.E}}", ttpHasError, ""},
	{"dot after float", "{{0.1.E}}", ttpHasError, ""},
	{"dot after boolean", "{{true.E}}", ttpHasError, ""},
	{"dot after char", "{{'a'.any}}", ttpHasError, ""},
	{"dot after string", `{{"hello".guys}}`, ttpHasError, ""},
	{"dot after dot", "{{..E}}", ttpHasError, ""},
	{"dot after nil", "{{nil.E}}", ttpHasError, ""},
	// Wrong pipeline
	{"wrong pipeline dot", "{{12|.}}", ttpHasError, ""},
	{"wrong pipeline number", "{{.|12|printf}}", ttpHasError, ""},
	{"wrong pipeline string", "{{.|printf|\"error\"}}", ttpHasError, ""},
	{"wrong pipeline char", "{{12|printf|'e'}}", ttpHasError, ""},
	{"wrong pipeline boolean", "{{.|true}}", ttpHasError, ""},
	{"wrong pipeline nil", "{{'c'|nil}}", ttpHasError, ""},
	{"empty pipeline", `{{printf "%d" ( ) }}`, ttpHasError, ""},
	// Missing pipeline in block
	{"block definition", `{{block "foo"}}hello{{end}}`, ttpHasError, ""},

	// Expression nested depth tests.
	{"paren nesting normal", "{{ (( 1 )) }}", ttpNoError, "{{((1))}}"},
	{"paren nesting at limit", "{{ ((( 1 ))) }}", ttpNoError, "{{(((1)))}}"},
	{"paren nesting exceeds limit", "{{ (((( 1 )))) }}", ttpHasError, "template: test:1: max expression depth exceeded"},
	{"paren nesting in pipeline", "{{ ((( 1 ))) | printf }}", ttpNoError, "{{(((1))) | printf}}"},
	{"paren nesting in pipeline exceeds limit", "{{ (((( 1 )))) | printf }}", ttpHasError, "template: test:1: max expression depth exceeded"},
	{"paren nesting with other constructs", "{{ if ((( true ))) }}YES{{ end }}", ttpNoError, "{{if (((true)))}}\"YES\"{{end}}"},
	{"paren nesting with other constructs exceeds limit", "{{ if (((( true )))) }}YES{{ end }}", ttpHasError, "template: test:1: max expression depth exceeded"},
}

// Changed: renamed from builtins. The functions text/template's test
// registers, for both parsers.
var ttpBuiltins = map[string]any{
	"printf":   fmt.Sprintf,
	"contains": strings.Contains,
}

// ttpFunctions names the functions in ttpBuiltins, as lex.Options.Functions takes them.
var ttpFunctions = map[string]bool{"printf": true, "contains": true}

// Changed: the error table is parseTests-shaped, like errorTests in the
// standard library. Every row is a refusal. Rows use no function map, so
// htt gets an empty, non-nil one: only builtins are known.
var ttpErrorTests = []ttpParseTest{
	// Check line numbers are accurate.
	{
		"unclosed1",
		"line1\n{{",
		ttpHasError, `unclosed1:2: unclosed action`,
	},
	{
		"unclosed2",
		"line1\n{{define `x`}}line2\n{{",
		ttpHasError, `unclosed2:3: unclosed action`,
	},
	{
		"unclosed3",
		"line1\n{{\"x\"\n\"y\"\n",
		ttpHasError, `unclosed3:4: unclosed action started at unclosed3:2`,
	},
	{
		"unclosed4",
		"{{\n\n\n\n\n",
		ttpHasError, `unclosed4:6: unclosed action started at unclosed4:1`,
	},
	{
		"var1",
		"line1\n{{\nx\n}}",
		ttpHasError, `var1:3: function "x" not defined`,
	},
	// Specific errors.
	{
		"function",
		"{{foo}}",
		ttpHasError, `function "foo" not defined`,
	},
	{
		"comment1",
		"{{/*}}",
		ttpHasError, `comment1:1: unclosed comment`,
	},
	{
		"comment2",
		"{{/*\nhello\n}}",
		ttpHasError, `comment2:1: unclosed comment`,
	},
	{
		"lparen",
		"{{.X (1 2 3}}",
		ttpHasError, `unclosed left paren`,
	},
	{
		"rparen",
		"{{.X 1 2 3 ) }}",
		ttpHasError, "unexpected right paren",
	},
	{
		"rparen2",
		"{{(.X 1 2 3",
		ttpHasError, `unclosed action`,
	},
	{
		"space",
		"{{`x`3}}",
		ttpHasError, `in operand`,
	},
	{
		"idchar",
		"{{a#}}",
		ttpHasError, `'#'`,
	},
	{
		"charconst",
		"{{'a}}",
		ttpHasError, `unterminated character constant`,
	},
	{
		"stringconst",
		`{{"a}}`,
		ttpHasError, `unterminated quoted string`,
	},
	{
		"rawstringconst",
		"{{`a}}",
		ttpHasError, `unterminated raw quoted string`,
	},
	{
		"number",
		"{{0xi}}",
		ttpHasError, `number syntax`,
	},
	{
		"multidefine",
		"{{define `a`}}a{{end}}{{define `a`}}b{{end}}",
		ttpHasError, `multiple definition of template`,
	},
	{
		"eof",
		"{{range .X}}",
		ttpHasError, `unexpected EOF`,
	},
	{
		"variable",
		// Declare $x so it's defined, to avoid that error, and then check we don't parse a declaration.
		"{{$x := 23}}{{with $x.y := 3}}{{$x 23}}{{end}}",
		ttpHasError, `unexpected ":="`,
	},
	{
		"multidecl",
		"{{$a,$b,$c := 23}}",
		ttpHasError, `too many declarations`,
	},
	{
		"undefvar",
		"{{$a}}",
		ttpHasError, `undefined variable`,
	},
	{
		"wrongdot",
		"{{true.any}}",
		ttpHasError, `unexpected . after term`,
	},
	{
		"wrongpipeline",
		"{{12|false}}",
		ttpHasError, `non executable command in pipeline`,
	},
	{
		"emptypipeline",
		`{{ ( ) }}`,
		ttpHasError, `missing value for parenthesized pipeline`,
	},
	{
		"multilinerawstring",
		"{{ $v := `\n` }} {{",
		ttpHasError, `multilinerawstring:2: unclosed action`,
	},
	{
		"rangeundefvar",
		"{{range $k}}{{end}}",
		ttpHasError, `undefined variable`,
	},
	{
		"rangeundefvars",
		"{{range $k, $v}}{{end}}",
		ttpHasError, `undefined variable`,
	},
	{
		"rangemissingvalue1",
		"{{range $k,}}{{end}}",
		ttpHasError, `missing value for range`,
	},
	{
		"rangemissingvalue2",
		"{{range $k, $v := }}{{end}}",
		ttpHasError, `missing value for range`,
	},
	{
		"rangenotvariable1",
		"{{range $k, .}}{{end}}",
		ttpHasError, `range can only initialize variables`,
	},
	{
		"rangenotvariable2",
		"{{range $k, 123 := .}}{{end}}",
		ttpHasError, `range can only initialize variables`,
	},
}

// ttpLexInput is one input of lex_test.go. Changed: only the inputs and
// delimiters are kept; the expected items are not compared.
type ttpLexInput struct {
	table, name, input, left, right string
}

var (
	ttpRaw   = "`" + `abc\n\t\" ` + "`"
	ttpRawNL = "`now is{{\n}}the time`" // Contains newline inside raw quote.
)

var ttpLexInputs = []ttpLexInput{
	// lexTests
	{"lexTests", "empty", "", "{{", "}}"},
	{"lexTests", "spaces", " \t\n", "{{", "}}"},
	{"lexTests", "text", `now is the time`, "{{", "}}"},
	{"lexTests", "text with comment", "hello-{{/* this is a comment */}}-world", "{{", "}}"},
	{"lexTests", "punctuation", "{{,@% }}", "{{", "}}"},
	{"lexTests", "parens", "{{((3))}}", "{{", "}}"},
	{"lexTests", "empty action", `{{}}`, "{{", "}}"},
	{"lexTests", "for", `{{for}}`, "{{", "}}"},
	{"lexTests", "block", `{{block "foo" .}}`, "{{", "}}"},
	{"lexTests", "quote", `{{"abc \n\t\" "}}`, "{{", "}}"},
	{"lexTests", "raw quote", "{{" + ttpRaw + "}}", "{{", "}}"},
	{"lexTests", "raw quote with newline", "{{" + ttpRawNL + "}}", "{{", "}}"},
	{"lexTests", "numbers", "{{1 02 0x14 0X14 -7.2i 1e3 1E3 +1.2e-4 4.2i 1+2i 1_2 0x1.e_fp4 0X1.E_FP4}}", "{{", "}}"},
	{"lexTests", "characters", `{{'a' '\n' '\'' '\\' 'ÿ' '\xFF' '本'}}`, "{{", "}}"},
	{"lexTests", "bools", "{{true false}}", "{{", "}}"},
	{"lexTests", "dot", "{{.}}", "{{", "}}"},
	{"lexTests", "nil", "{{nil}}", "{{", "}}"},
	{"lexTests", "dots", "{{.x . .2 .x.y.z}}", "{{", "}}"},
	{"lexTests", "keywords", "{{range if else end with}}", "{{", "}}"},
	{"lexTests", "variables", "{{$c := printf $ $hello $23 $ $var.Field .Method}}", "{{", "}}"},
	{"lexTests", "variable invocation", "{{$x 23}}", "{{", "}}"},
	{"lexTests", "pipeline", `intro {{echo hi 1.2 |noargs|args 1 "hi"}} outro`, "{{", "}}"},
	{"lexTests", "declaration", "{{$v := 3}}", "{{", "}}"},
	{"lexTests", "2 declarations", "{{$v , $w := 3}}", "{{", "}}"},
	{"lexTests", "field of parenthesized expression", "{{(.X).Y}}", "{{", "}}"},
	{"lexTests", "trimming spaces before and after", "hello- {{- 3 -}} -world", "{{", "}}"},
	{"lexTests", "trimming spaces before and after comment", "hello- {{- /* hello */ -}} -world", "{{", "}}"},
	{"lexTests", "badchar", "#{{\x01}}", "{{", "}}"},
	{"lexTests", "unclosed action", "{{", "{{", "}}"},
	{"lexTests", "EOF in action", "{{range", "{{", "}}"},
	{"lexTests", "unclosed quote", "{{\"\n\"}}", "{{", "}}"},
	{"lexTests", "unclosed raw quote", "{{`xx}}", "{{", "}}"},
	{"lexTests", "unclosed char constant", "{{'\n}}", "{{", "}}"},
	{"lexTests", "bad number", "{{3k}}", "{{", "}}"},
	{"lexTests", "unclosed paren", "{{(3}}", "{{", "}}"},
	{"lexTests", "extra right paren", "{{3)}}", "{{", "}}"},
	{"lexTests", "long pipeline deadlock", "{{|||||}}", "{{", "}}"},
	{"lexTests", "text with bad comment", "hello-{{/*/}}-world", "{{", "}}"},
	{"lexTests", "text with comment close separated from delim", "hello-{{/* */ }}-world", "{{", "}}"},
	{"lexTests", "unmatched right delimiter", "hello-{.}}-world", "{{", "}}"},

	// lexDelimTests, with $$ and @@
	{"lexDelimTests", "punctuation", "$$,@%{{}}@@", "$$", "@@"},
	{"lexDelimTests", "empty action", `$$@@`, "$$", "@@"},
	{"lexDelimTests", "for", `$$for@@`, "$$", "@@"},
	{"lexDelimTests", "quote", `$$"abc \n\t\" "@@`, "$$", "@@"},
	{"lexDelimTests", "raw quote", "$$" + ttpRaw + "@@", "$$", "@@"},

	// TestDelimsAlphaNumeric and TestDelimsAndMarkers
	{"TestDelimsAlphaNumeric", "right delimiter with alphanumeric start", "{{hub .host hub}}", "{{hub", "hub}}"},
	{"TestDelimsAndMarkers", "delims that look like markers", "{{- .x -}} {{- - .x - -}}", "{{- ", " -}}"},

	// lexPosTests
	{"lexPosTests", "empty", "", "{{", "}}"},
	{"lexPosTests", "punctuation", "{{,@%#}}", "{{", "}}"},
	{"lexPosTests", "sample", "0123{{hello}}xyz", "{{", "}}"},
	{"lexPosTests", "trimafter", "{{x -}}\n{{y}}", "{{", "}}"},
	{"lexPosTests", "trimbefore", "{{x}}\n{{- y}}", "{{", "}}"},
	{"lexPosTests", "longcomment", "{{/*\n*/}}\n{{undefinedFunction \"test\"}}", "{{", "}}"},
}

// ttpPanic is a panic from htt, reported as an error of the parse that made it.
type ttpPanic struct{ value any }

func (p ttpPanic) Error() string { return fmt.Sprintf("panic: %v", p.value) }

// ttpStdParse parses input with text/template's parser, as its tests do.
func ttpStdParse(name, input, left, right string, mode parse.Mode, funcs ...map[string]any) (*parse.Tree, error) {
	tr := parse.New(name)
	tr.Mode = mode
	return tr.Parse(input, left, right, make(map[string]*parse.Tree), funcs...)
}

// ttpHTTParse parses input with htt in text mode. A panic is returned as a ttpPanic.
func ttpHTTParse(name, input string, options lex.Options) (tree *htt.Tree, err error) {
	defer func() {
		if r := recover(); r != nil {
			tree, err = nil, ttpPanic{value: r}
		}
	}()
	return htt.ParseTree(name, name, input, nil, options)
}

// ttpCatch returns the value f panics with, or nil.
func ttpCatch(f func()) (p any) {
	defer func() { p = recover() }()
	f()
	return nil
}

// ttpCanon parses src with text/template and returns its canonical form.
func ttpCanon(name, src string, mode parse.Mode, funcs ...map[string]any) (string, error) {
	tree, err := ttpStdParse(name, src, "", "", mode, funcs...)
	if err != nil {
		return "", err
	}
	return tree.Root.String(), nil
}

// ttpRoot returns the source text htt holds for a tree's root.
func ttpRoot(tree *htt.Tree) string {
	if tree == nil || tree.Root == nil {
		return ""
	}
	return tree.Root.String()
}

// ttpErrLine returns the LINE of a "template: NAME:LINE: msg" error.
func ttpErrLine(err error, name string) (int, bool) {
	rest, ok := strings.CutPrefix(err.Error(), "template: "+name+":")
	if !ok {
		return 0, false
	}
	digits, _, ok := strings.Cut(rest, ":")
	if !ok {
		return 0, false
	}
	line, convErr := strconv.Atoi(digits)
	return line, convErr == nil
}

// ttpTextOptions returns htt's lex options for a text-mode parse with the
// given delimiters and function names. A nil functions map disables the check.
func ttpTextOptions(left, right string, functions map[string]bool) lex.Options {
	return lex.Options{
		LeftDelim:  left,
		RightDelim: right,
		Functions:  functions,
		BreakOK:    true,
		ContinueOK: true,
	}
}

// TestParserAgreesWithTextTemplateOnItsParseTable runs each row of parseTests
// through both parsers. Both must accept or both refuse, and when both accept,
// the htt source of the root must reparse to text/template's canonical form.
func TestParserAgreesWithTextTemplateOnItsParseTable(t *testing.T) {
	for _, test := range ttpParseTests {
		t.Run(test.name, func(t *testing.T) {
			// Changed: text/template's test lowers an unexported depth limit
			// (maxStackDepth = 3). The real parser accepts these at its normal
			// limit, so the three rows that expect the limit are skipped.
			if strings.Contains(test.result, "max expression depth exceeded") {
				t.Skip("not applicable: text/template's test lowers an unexported limit")
			}
			stdTree, stdErr := ttpStdParse(test.name, test.input, "", "", 0, ttpBuiltins)
			httTree, httErr := ttpHTTParse(test.name, test.input, ttpTextOptions("{{", "}}", ttpFunctions))
			_, httPanicked := httErr.(ttpPanic)
			switch {
			case stdErr != nil && httErr == nil:
				t.Errorf("differs from text/template: [1] parseTests row %q, input %q: text/template refuses with %v; htt accepts, root %q",
					test.name, test.input, stdErr, ttpRoot(httTree))
			case stdErr == nil && httPanicked:
				t.Errorf("differs from text/template: [2] parseTests row %q, input %q: text/template accepts, root %q; htt panics: %v",
					test.name, test.input, stdTree.Root.String(), httErr)
			case stdErr == nil && httErr != nil:
				t.Errorf("differs from text/template: [2] parseTests row %q, input %q: text/template accepts, root %q; htt refuses with %v",
					test.name, test.input, stdTree.Root.String(), httErr)
			case stdErr != nil && httErr != nil:
				return
			}
			want := stdTree.Root.String()
			back, err := ttpCanon(test.name, ttpRoot(httTree), 0, ttpBuiltins)
			if err != nil {
				t.Errorf("differs from text/template: [3] parseTests row %q, input %q: htt root %q does not reparse with text/template: %v",
					test.name, test.input, ttpRoot(httTree), err)
			}
			if back != want {
				t.Errorf("differs from text/template: [3] parseTests row %q, input %q: round trip: text/template canonical %q; htt root %q reparses to %q",
					test.name, test.input, want, ttpRoot(httTree), back)
			}
		})
	}
}

// TestParserAgreesWithTextTemplateOnItsErrorTable runs each row of errorTests
// through both parsers. Both must refuse, htt's error must contain the
// table's text, and the LINE of the two errors must agree when both have one.
func TestParserAgreesWithTextTemplateOnItsErrorTable(t *testing.T) {
	for _, test := range ttpErrorTests {
		t.Run(test.name, func(t *testing.T) {
			_, stdErr := ttpStdParse(test.name, test.input, "", "", 0)
			_, httErr := ttpHTTParse(test.name, test.input, ttpTextOptions("{{", "}}", map[string]bool{}))
			_, httPanicked := httErr.(ttpPanic)
			switch {
			case stdErr == nil && httErr == nil:
				t.Errorf("differs from text/template: [0] errorTests row %q, input %q: both accept; the table says refuse",
					test.name, test.input)
			case stdErr == nil:
				t.Errorf("differs from text/template: [2] errorTests row %q, input %q: text/template accepts; htt refuses with %v",
					test.name, test.input, httErr)
			case httPanicked:
				t.Errorf("differs from text/template: [4] errorTests row %q, input %q: text/template refuses with %v; htt panics: %v",
					test.name, test.input, stdErr, httErr)
			case httErr == nil:
				t.Errorf("differs from text/template: [1] errorTests row %q, input %q: text/template refuses with %v; htt accepts",
					test.name, test.input, stdErr)
			}
			if !strings.Contains(httErr.Error(), test.result) {
				t.Errorf("differs from text/template: [4] errorTests row %q, input %q: error text: want it to contain %q; text/template %v; htt %v",
					test.name, test.input, test.result, stdErr, httErr)
			}
			stdLine, stdHas := ttpErrLine(stdErr, test.name)
			httLine, httHas := ttpErrLine(httErr, test.name)
			if stdHas && httHas && stdLine != httLine {
				t.Errorf("differs from text/template: [4] errorTests row %q, input %q: line: text/template %d; htt %d (text/template %v; htt %v)",
					test.name, test.input, stdLine, httLine, stdErr, httErr)
			}
		})
	}
}

// TestParserAgreesOnTheLexerInputs runs every input of lex_test.go through both
// parsers with the same delimiters. Changed: text/template runs with
// parse.SkipFuncCheck and htt with nil Functions, so no function name is
// checked on either side. Only acceptance is compared.
func TestParserAgreesOnTheLexerInputs(t *testing.T) {
	for _, in := range ttpLexInputs {
		t.Run(in.table+"/"+in.name, func(t *testing.T) {
			_, stdErr := ttpStdParse(in.name, in.input, in.left, in.right, parse.SkipFuncCheck)
			_, httErr := ttpHTTParse(in.name, in.input, ttpTextOptions(in.left, in.right, nil))
			_, httPanicked := httErr.(ttpPanic)
			switch {
			case stdErr != nil && httErr == nil:
				t.Errorf("differs from text/template: [1] %s row %q, delims %q %q, input %q: text/template refuses with %v; htt accepts",
					in.table, in.name, in.left, in.right, in.input, stdErr)
			case stdErr == nil && httPanicked:
				t.Errorf("differs from text/template: [2] %s row %q, delims %q %q, input %q: text/template accepts; htt panics: %v",
					in.table, in.name, in.left, in.right, in.input, httErr)
			case stdErr == nil && httErr != nil:
				t.Errorf("differs from text/template: [2] %s row %q, delims %q %q, input %q: text/template accepts; htt refuses with %v",
					in.table, in.name, in.left, in.right, in.input, httErr)
			}
		})
	}
}

// TestStdIsEmpty is TestIsEmpty with htt's IsEmptyTree and ParseTree.
// Changed: the expected values are text/template's.
func TestStdIsEmpty(t *testing.T) {
	isEmptyTests := []struct {
		name  string
		input string
		empty bool
	}{
		{"empty", ``, true},
		{"nonempty", `hello`, false},
		{"spaces only", " \t\n \t\n", true},
		{"comment only", "{{/* comment */}}", true},
		{"definition", `{{define "x"}}something{{end}}`, true},
		{"definitions and space", "{{define `x`}}something{{end}}\n\n{{define `y`}}something{{end}}\n\n", true},
		{"definitions and text", "{{define `x`}}something{{end}}\nx\n{{define `y`}}something{{end}}\ny\n", false},
		{"definition and action", "{{define `x`}}something{{end}}{{if 3}}foo{{end}}", false},
	}
	var nilEmpty bool
	if p := ttpCatch(func() { nilEmpty = htt.IsEmptyTree(nil) }); p != nil || !nilEmpty {
		t.Errorf("differs from text/template: [5] IsEmpty(nil) = %t, panic %v; text/template: nil tree is empty", nilEmpty, p)
	}
	for _, test := range isEmptyTests {
		t.Run(test.name, func(t *testing.T) {
			tree, err := ttpHTTParse("root", test.input, ttpTextOptions("{{", "}}", nil))
			if err != nil {
				t.Errorf("differs from text/template: [2] IsEmpty row %q, input %q: text/template accepts; htt refuses with %v",
					test.name, test.input, err)
			}
			var empty bool
			if p := ttpCatch(func() { empty = htt.IsEmptyTree(tree.Root) }); p != nil {
				t.Errorf("differs from text/template: [5] IsEmpty row %q, input %q: htt.IsEmptyTree panics: %v",
					test.name, test.input, p)
			}
			if empty != test.empty {
				t.Errorf("differs from text/template: [5] IsEmpty row %q, input %q: want %t; htt %t",
					test.name, test.input, test.empty, empty)
			}
		})
	}
}

// TestStdKeywordsAndFuncs is TestKeywordsAndFuncs with htt's options. A
// registered "break" function makes break an identifier; without one it is
// a keyword and the parse fails.
func TestStdKeywordsAndFuncs(t *testing.T) {
	const input = `{{range .X}}{{break 20}}{{end}}`
	t.Run("with break func", func(t *testing.T) {
		options := ttpTextOptions("{{", "}}", map[string]bool{"break": true})
		options.BreakOK = false
		tree, err := ttpHTTParse("keywords", input, options)
		if err != nil || tree == nil {
			t.Errorf("differs from text/template: [2] keywords with break func, input %q: text/template accepts; htt refuses with %v",
				input, err)
		}
	})
	t.Run("without break func", func(t *testing.T) {
		tree, err := ttpHTTParse("keywords", input, ttpTextOptions("{{", "}}", map[string]bool{}))
		if err == nil || tree != nil {
			t.Errorf("differs from text/template: [1] keywords without break func, input %q: text/template refuses; htt accepts, root %q",
				input, ttpRoot(tree))
		}
	})
}

// TestStdSkipFuncCheck is TestSkipFuncCheck with htt's nil Functions.
// Changed: htt's String is the source span, so the canonical form is
// checked by reparsing that span with text/template in SkipFuncCheck mode.
func TestStdSkipFuncCheck(t *testing.T) {
	const input = "{{fn 1 2}}"
	const want = "{{fn 1 2}}"
	tree, err := ttpHTTParse("skip func check", input, ttpTextOptions("{{", "}}", nil))
	if err != nil {
		t.Errorf("differs from text/template: [2] skip func check, input %q: text/template accepts; htt refuses with %v", input, err)
	}
	got, err := ttpCanon("skip func check", ttpRoot(tree), parse.SkipFuncCheck)
	if err != nil {
		t.Errorf("differs from text/template: [3] skip func check, input %q: htt root %q does not reparse: %v", input, ttpRoot(tree), err)
	}
	if got != want {
		t.Errorf("differs from text/template: [5] skip func check, input %q: want %q; htt root reparses to %q", input, want, got)
	}
}

// TestStdBlock is TestBlock with htt's treeSet. Changed: htt's String is the
// source span, so each canonical form is checked by reparsing the htt source
// with text/template.
func TestStdBlock(t *testing.T) {
	const (
		input = `a{{block "inner" .}}bar{{.}}baz{{end}}b`
		outer = `a{{template "inner" .}}b`
		inner = `bar{{.}}baz`
	)
	treeSet := make(map[string]*htt.Tree)
	tree, err := htt.ParseTree("outer", "outer", input, treeSet, ttpTextOptions("{{", "}}", nil))
	if err != nil {
		t.Errorf("differs from text/template: [2] block, input %q: text/template accepts; htt refuses with %v", input, err)
	}
	if got, err := ttpCanon("outer", ttpRoot(tree), 0); err != nil || got != outer {
		t.Errorf("differs from text/template: [3] block outer, input %q: want %q; htt root %q reparses to %q (err %v)",
			input, outer, ttpRoot(tree), got, err)
	}
	inTmpl := treeSet["inner"]
	if inTmpl == nil {
		t.Errorf("differs from text/template: [5] block did not define template %q; text/template defines it", "inner")
	}
	if got, err := ttpCanon("inner", ttpRoot(inTmpl), 0); err != nil || got != inner {
		t.Errorf("differs from text/template: [3] block inner, input %q: want %q; htt root %q reparses to %q (err %v)",
			input, inner, ttpRoot(inTmpl), got, err)
	}
}

// TestStdLineNum is TestLineNum with htt's PositionAt. Changed: text/template's
// TextNode has no Line, so only ActionNodes and their pipelines are compared,
// in order. htt's line comes from PositionAt(node.Start()).
func TestStdLineNum(t *testing.T) {
	const count = 3
	text := strings.Repeat("{{printf 1234}}\n", count)
	stdTree, err := ttpStdParse("bench", text, "", "", 0, ttpBuiltins)
	if err != nil {
		t.Fatal(err)
	}
	httTree, err := ttpHTTParse("bench", text, ttpTextOptions("{{", "}}", ttpFunctions))
	if err != nil {
		t.Errorf("differs from text/template: [2] line numbers, input %q: text/template accepts; htt refuses with %v", text, err)
	}
	var stdLines, httLines []int
	for _, n := range stdTree.Root.Nodes {
		if action, ok := n.(*parse.ActionNode); ok {
			stdLines = append(stdLines, action.Line, action.Pipe.Line)
		}
	}
	for _, n := range httTree.Root.Nodes {
		if action, ok := n.(*htt.ActionNode); ok {
			httLines = append(httLines, httTree.PositionAt(action.Start()).Line, httTree.PositionAt(action.Pipe.Start()).Line)
		}
	}
	if !slices.Equal(stdLines, httLines) {
		t.Errorf("differs from text/template: [4] line numbers, input %q: text/template lines %v; htt lines %v",
			text, stdLines, httLines)
	}
}

// TestStdParseWithComments is TestParseWithComments with htt's EmitComment.
// Changed: htt's String is the source span, so each canonical form is checked
// by reparsing the htt source with text/template in ParseComments mode.
func TestStdParseWithComments(t *testing.T) {
	tests := [...]ttpParseTest{
		{"comment", "{{/*\n\n\n*/}}", ttpNoError, "{{/*\n\n\n*/}}"},
		{"comment trim left", "x \r\n\t{{- /* hi */}}", ttpNoError, `"x"{{/* hi */}}`},
		{"comment trim right", "{{/* hi */ -}}\n\n\ty", ttpNoError, `{{/* hi */}}"y"`},
		{"comment trim left and right", "x \r\n\t{{- /* */ -}}\n\n\ty", ttpNoError, `"x"{{/* */}}"y"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := ttpTextOptions("{{", "}}", nil)
			options.EmitComment = true
			tree, err := ttpHTTParse(test.name, test.input, options)
			if err != nil {
				t.Errorf("differs from text/template: [2] comments row %q, input %q: text/template accepts; htt refuses with %v",
					test.name, test.input, err)
			}
			got, err := ttpCanon(test.name, ttpRoot(tree), parse.ParseComments)
			if err != nil {
				t.Errorf("differs from text/template: [3] comments row %q, input %q: htt root %q does not reparse: %v",
					test.name, test.input, ttpRoot(tree), err)
			}
			// Changed: the table's result is in the standard test's own text
			// format, which quotes text nodes; compare with text/template's
			// canonical form of the same input instead.
			want, err := ttpCanon(test.name, test.input, parse.ParseComments)
			if err != nil {
				t.Fatalf("text/template refuses %q: %v", test.input, err)
			}
			if got != want {
				t.Errorf("differs from text/template: [5] comments row %q, input %q: want %q; htt root reparses to %q",
					test.name, test.input, want, got)
			}
		})
	}
}
