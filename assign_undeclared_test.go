package htt_test

import (
	"bytes"
	"strings"
	"testing"
	texttemplate "text/template"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

// outcome records where a template stopped and what it produced: "parse",
// "execute", or "ok", with the error text or the output.
type outcome struct {
	stage string
	text  string
}

func runStdText(text string, data any) outcome {
	tmpl, err := texttemplate.New("z").Parse(text)
	if err != nil {
		return outcome{"parse", err.Error()}
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return outcome{"execute", err.Error()}
	}
	return outcome{"ok", buf.String()}
}

func runHTT(text string, html bool, data any) outcome {
	opts := lex.Options{
		LeftDelim:  "{{",
		RightDelim: "}}",
		ParseHTML:  html,
		BreakOK:    true,
		ContinueOK: true,
	}
	tree, err := htt.ParseTree("z", "z", text, nil, opts)
	if err != nil {
		return outcome{"parse", err.Error()}
	}
	if html {
		htt.AnnotateHTMLContexts(tree.Root)
	}
	var buf bytes.Buffer
	if err := htt.ExecuteTemplate(&buf, tree, data); err != nil {
		return outcome{"execute", err.Error()}
	}
	return outcome{"ok", buf.String()}
}

func TestAssignmentToUndeclaredVariable(t *testing.T) {
	t.Parallel()
	slice := []int{1, 2}
	tests := []struct {
		name     string
		text     string
		html     bool
		data     any
		variable string // the undeclared variable named in the execute error, if any
	}{
		{name: "plain assignment", text: `{{$x = 1}}`, variable: "$x"},
		{name: "if assignment", text: `{{if $x = 1}}a{{end}}`, variable: "$x"},
		{name: "with assignment", text: `{{with $x = .}}a{{end}}`, data: 1, variable: "$x"},
		{name: "range assignment", text: `{{range $x = .}}a{{end}}`, data: slice, variable: "$x"},
		{name: "range index assignment", text: `{{range $i, $x = .}}a{{end}}`, data: slice, variable: "$i"},
		{name: "declared then assigned", text: `{{$x := 1}}{{$x = 2}}{{$x}}`},
		{name: "declared in outer scope", text: `{{$x := 1}}{{if true}}{{$x = 2}}{{end}}{{$x}}`},
		{name: "declared inside a closed scope", text: `{{if true}}{{$y := 1}}{{end}}{{$y = 2}}`, variable: "$y"},
		{name: "assign to dollar", text: `{{$ = 1}}{{$}}`},
		{name: "html if assignment", text: `<p>{{if $x = 1}}a{{end}}</p>`, html: true, variable: "$x"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			want := runStdText(tc.text, tc.data)
			got := runHTT(tc.text, tc.html, tc.data)

			if want.stage != got.stage {
				t.Fatalf("stage for %q = %s (%s), want %s (%s)", tc.text, got.stage, got.text, want.stage, want.text)
			}
			switch want.stage {
			case "execute":
				if !strings.Contains(want.text, "undefined variable: "+tc.variable) {
					t.Fatalf("text/template error for %q = %q, want it to name %s", tc.text, want.text, tc.variable)
				}
				if !strings.Contains(got.text, "undefined variable: "+tc.variable) {
					t.Errorf("htt error for %q = %q, want it to contain %q", tc.text, got.text, "undefined variable: "+tc.variable)
				}
			case "ok":
				if got.text != want.text {
					t.Errorf("output for %q = %q, want %q", tc.text, got.text, want.text)
				}
			default:
				t.Fatalf("text/template failed to parse %q: %s", tc.text, want.text)
			}
		})
	}
}
