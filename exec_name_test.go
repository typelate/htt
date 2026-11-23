package htt_test

import (
	"io"
	"testing"
	texttemplate "text/template"
	"text/template/parse"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

// A called template is named in an error as it was called, as text/template
// names a tree by the name it was added under.
func TestExecutionErrorNamesTheTemplateAsCalled(t *testing.T) {
	const mainText, calledText = `{{template "alias" .}}`, `{{.X}}`

	trees, err := parse.Parse("t", calledText, "{{", "}}")
	if err != nil {
		t.Fatal(err)
	}
	std := texttemplate.Must(texttemplate.New("main").Parse(mainText))
	if _, err := std.AddParseTree("alias", trees["t"]); err != nil {
		t.Fatal(err)
	}
	want := std.Execute(io.Discard, 1)
	if want == nil {
		t.Fatal("text/template did not fail")
	}

	options := lex.Options{LeftDelim: "{{", RightDelim: "}}"}
	main, err := htt.ParseTree("main", "main", mainText, nil, options)
	if err != nil {
		t.Fatal(err)
	}
	called, err := htt.ParseTree("t", "t", calledText, nil, options)
	if err != nil {
		t.Fatal(err)
	}
	finder := htt.FindTreeFunc(func(name string) (*htt.Tree, bool) {
		return called, name == "alias"
	})
	got := htt.ExecuteTemplate(io.Discard, main, 1, htt.WithFinder(finder))
	if got == nil {
		t.Fatal("htt did not fail")
	}
	if got.Error() != want.Error() {
		t.Errorf("error = %q, want %q", got, want)
	}
}
