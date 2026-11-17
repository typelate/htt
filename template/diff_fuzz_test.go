package template_test

import (
	"bytes"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	htmltemplate "html/template"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/typelate/htt/template"
)

// FuzzDiffEscape runs each template text through html/template and through
// this package. Both must fail or both succeed, and a success must write the
// same bytes. A failure must carry the same ErrorCode, and this package must
// write nothing. A text that html/template rejects at Parse is skipped. A text
// that html/template parses and this package does not is also skipped: the
// root package's FuzzStdlibAcceptsImpliesHTMLParses owns that property.
func FuzzDiffEscape(f *testing.F) {
	const value = "a'b/c&d=e</"
	for _, input := range compatTableInputs(f) {
		f.Add(input, value)
	}
	f.Add(`<a data-on:click="@post('{{.}}')">`, value)
	f.Add(`<a href="/x?q={{.}}">`, value)
	f.Fuzz(func(t *testing.T, text, value string) {
		// Recursive templates never finish.
		if len(text) > 256 || strings.Contains(text, "{{template") || strings.Contains(text, "{{block") {
			return
		}
		stdTmpl, err := htmltemplate.New("z").Parse(text)
		if err != nil {
			return
		}
		httTmpl, err := template.New("z").Parse(text)
		if err != nil {
			return
		}
		var stdBuf, httBuf bytes.Buffer
		stdErr := stdTmpl.Execute(&stdBuf, value)
		httErr := httTmpl.Execute(&httBuf, value)
		if (stdErr == nil) != (httErr == nil) {
			t.Fatalf("one engine fails where the other renders\n  text: %q\n  value: %q\n  htt:  %q, err %v\n  std:  %q, err %v",
				text, value, httBuf.String(), httErr, stdBuf.String(), stdErr)
		}
		if stdErr == nil {
			if got, want := httBuf.String(), stdBuf.String(); got != want {
				t.Fatalf("htt and html/template write different bytes\n  text: %q\n  value: %q\n  htt:  %q\n  std:  %q",
					text, value, got, want)
			}
			return
		}
		var stdCode *htmltemplate.Error
		if !errors.As(stdErr, &stdCode) {
			return
		}
		var httCode *template.Error
		if !errors.As(httErr, &httCode) || int(httCode.ErrorCode) != int(stdCode.ErrorCode) {
			t.Fatalf("error codes differ\n  text: %q\n  value: %q\n  htt:  err %v\n  std:  err %v",
				text, value, httErr, stdErr)
		}
		if httBuf.Len() != 0 {
			t.Fatalf("htt wrote %q before failing with %v\n  text: %q\n  value: %q",
				httBuf.String(), httErr, text, value)
		}
	})
}

// compatTableInputs returns the input of every row of the TestCompatHTMLEscape
// and TestCompatHTMLErrors tables. Those tables are local to their tests, so
// the rows are read from the syntax tree of compat_html_test.go.
func compatTableInputs(t testing.TB) []string {
	t.Helper()
	src, err := os.ReadFile("compat_html_test.go")
	if err != nil {
		t.Fatalf("read compat_html_test.go: %v", err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "compat_html_test.go", src, 0)
	if err != nil {
		t.Fatalf("parse compat_html_test.go: %v", err)
	}
	var inputs []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || (fn.Name.Name != "TestCompatHTMLEscape" && fn.Name.Name != "TestCompatHTMLErrors") {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			arr, ok := lit.Type.(*ast.ArrayType)
			if !ok {
				return true
			}
			st, ok := arr.Elt.(*ast.StructType)
			if !ok {
				return true
			}
			idx := fieldIndex(st, "input")
			if idx < 0 {
				return true
			}
			for _, row := range lit.Elts {
				rowLit, ok := row.(*ast.CompositeLit)
				if !ok || idx >= len(rowLit.Elts) {
					t.Fatalf("%s: row is not a positional literal with an input", fn.Name.Name)
				}
				s, ok := constString(rowLit.Elts[idx])
				if !ok {
					t.Fatalf("%s: input is not a constant string", fn.Name.Name)
				}
				inputs = append(inputs, s)
			}
			return false
		})
	}
	if len(inputs) == 0 {
		t.Fatal("no table rows found in compat_html_test.go")
	}
	return inputs
}

// fieldIndex returns the position of the named field in st, or -1.
func fieldIndex(st *ast.StructType, name string) int {
	i := 0
	for _, field := range st.Fields.List {
		for _, id := range field.Names {
			if id.Name == name {
				return i
			}
			i++
		}
	}
	return -1
}

// constString evaluates a string literal, or a + of string literals.
func constString(e ast.Expr) (string, bool) {
	switch e := e.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(e.Value)
		return s, err == nil
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		x, ok := constString(e.X)
		if !ok {
			return "", false
		}
		y, ok := constString(e.Y)
		return x + y, ok
	}
	return "", false
}
