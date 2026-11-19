package template_test

import (
	"bytes"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"go/version"
	htmltemplate "html/template"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/typelate/htt/template"
)

// escaperCopyVersion is the Go release the copied escaper sources came from.
// An older Go's html/template lacks fixes this package already has.
const escaperCopyVersion = "go1.27.3"

// FuzzDiffEscape runs each template text through html/template and through
// this package. Both must fail or both succeed, and a success must write the
// same bytes. A failure must carry the same ErrorCode, and this package must
// write nothing. A text that html/template rejects at Parse is skipped. A text
// that html/template parses and this package does not is also skipped: the
// root package's FuzzStdlibAcceptsImpliesHTMLParses owns that property.
func FuzzDiffEscape(f *testing.F) {
	if version.Compare(runtime.Version(), escaperCopyVersion) < 0 {
		f.Skipf("running %s, but the escaper was copied from %s; rerun with Go %s or newer, whose html/template has the fixes this package carries",
			runtime.Version(), escaperCopyVersion, escaperCopyVersion)
	}
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

// TestEscaperCopyVersion checks that every copied html/template file names the
// Go release in escaperCopyVersion, so the fuzz target's version gate cannot
// drift from the sources it guards.
func TestEscaperCopyVersion(t *testing.T) {
	const prefix = "// Copied from html/template/"
	versionPattern := regexp.MustCompile(`of (go[0-9.]+)`)
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read template dir: %v", err)
	}
	var copied int
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, line := range strings.Split(string(src), "\n") {
			if !strings.HasPrefix(line, prefix) {
				continue
			}
			copied++
			m := versionPattern.FindStringSubmatch(line)
			if m == nil {
				t.Errorf("%s: no Go version in %q", name, line)
				continue
			}
			// The sentence-ending period after the version is not part of it.
			found := strings.TrimSuffix(m[1], ".")
			if found != escaperCopyVersion {
				t.Errorf("%s: copied from %s, want %s (escaperCopyVersion)", name, found, escaperCopyVersion)
			}
		}
	}
	if copied == 0 {
		t.Fatal("no files carry the 'Copied from html/template/' line")
	}
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
