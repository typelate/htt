package template

import (
	"bytes"
	"errors"
	htmltemplate "html/template"
	"io"
	"strings"
	"testing"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

// TestCoverageExportRunsTheEscapedPath states that exporting coverage does
// not change what a template writes or refuses.
func TestCoverageExportRunsTheEscapedPath(t *testing.T) {
	const value = "a'b/c&d=e</"
	tests := []struct {
		name string
		src  string
	}{
		{"url attribute", `<a href="/x?q={{.}}">l</a>`},
		{"script", `<script>var x = {{.}};</script>`},
		{"template in attribute", `{{define "t"}}{{.}}{{end}}<p title="{{template "t" .}}">{{template "t" .}}</p>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := htmlTemplateOutput(t, tt.src, value)

			useCoverExport(t, nil)
			off := executeOutput(t, tt.src, value)

			useCoverExport(t, coverExportFor(t.TempDir(), io.Discard))
			on := executeOutput(t, tt.src, value)

			if off != want {
				t.Fatalf("with export off Execute wrote %s, want %s", off, want)
			}
			if on != want {
				t.Errorf("with export on Execute wrote %s, want %s", on, want)
			}
		})
	}
}

// TestCoverageExportRefusesWhatHTMLTemplateRefuses states that a template
// html/template refuses is refused the same way with coverage export on.
func TestCoverageExportRefusesWhatHTMLTemplateRefuses(t *testing.T) {
	// The branches of the if end in different contexts, so html/template
	// refuses it whatever the data.
	const src = `<a href="x{{if .}}"{{end}}>x</a>`
	var stdErr *htmltemplate.Error
	if err := htmltemplate.Must(htmltemplate.New("page").Parse(src)).Execute(io.Discard, false); !errors.As(err, &stdErr) {
		t.Fatalf("html/template Execute returned %v, want an *html/template.Error", err)
	}

	run := func(export *coverExport) (string, error) {
		useCoverExport(t, export)
		var out bytes.Buffer
		err := Must(New("page").Parse(src)).Execute(&out, false)
		return out.String(), err
	}
	offOut, offErr := run(nil)
	onOut, onErr := run(coverExportFor(t.TempDir(), io.Discard))

	var off, on *Error
	if !errors.As(offErr, &off) {
		t.Fatalf("with export off Execute returned %v, want an *Error", offErr)
	}
	if !errors.As(onErr, &on) {
		t.Fatalf("with export on Execute returned %v, want an *Error", onErr)
	}
	if on.ErrorCode != off.ErrorCode || int(on.ErrorCode) != int(stdErr.ErrorCode) {
		t.Errorf("with export on ErrorCode = %d, with it off %d, html/template %d; want all equal",
			on.ErrorCode, off.ErrorCode, stdErr.ErrorCode)
	}
	if offOut != "" || onOut != "" {
		t.Errorf("wrote %q with export off and %q with it on, want nothing", offOut, onOut)
	}
}

// TestCoverageExportProfileMatchesHTMLEngine states that one run exports the
// blocks the HTML engine counts for the same source and data.
func TestCoverageExportProfileMatchesHTMLEngine(t *testing.T) {
	const src = `<ul>{{range .}}<li class="i">{{.}}</li>{{else}}<li>none</li>{{end}}</ul>`
	data := []string{"a'b", "c&d", "e</"}

	dir := t.TempDir()
	export := coverExportFor(dir, io.Discard)
	useCoverExport(t, export)
	if err := Must(New("page").Parse(src)).Execute(io.Discard, data); err != nil {
		t.Fatal(err)
	}

	want := htmlEngineProfile(t, "page", src, data)
	if got := readFile(t, export.path); got != want {
		t.Errorf("profile after one run is\n%s\nwant\n%s", got, want)
	}
}

// TestCoverageExportAddedTextTree states that a text tree added with
// AddParseTree runs under the export and exports its blocks.
func TestCoverageExportAddedTextTree(t *testing.T) {
	const src = `<p>{{.}}</p>`
	tree, err := htt.ParseTree("added", "added", src, nil, lex.Options{
		LeftDelim:  "{{",
		RightDelim: "}}",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := func() string {
		useCoverExport(t, nil)
		set := New("main")
		if _, err := set.AddParseTree("added", tree); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := set.ExecuteTemplate(&out, "added", "<b>"); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}()

	dir := t.TempDir()
	export := coverExportFor(dir, io.Discard)
	useCoverExport(t, export)
	set := New("main")
	if _, err := set.AddParseTree("added", tree); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := set.ExecuteTemplate(&out, "added", "<b>"); err != nil {
		t.Fatalf("ExecuteTemplate with export on returned %v", err)
	}
	if out.String() != want {
		t.Errorf("with export on ExecuteTemplate wrote %s, want %s", out.String(), want)
	}
	profile := readFile(t, export.path)
	if !strings.Contains(profile, "\nadded:") {
		t.Errorf("profile has no block of the added tree:\n%s", profile)
	}
}

// executeOutput returns what Execute writes for src and value, with a
// failure as its text.
func executeOutput(t *testing.T, src string, value any) string {
	t.Helper()
	var out bytes.Buffer
	if err := Must(New("page").Parse(src)).Execute(&out, value); err != nil {
		return "error: " + err.Error()
	}
	return out.String()
}

func htmlTemplateOutput(t *testing.T, src string, value any) string {
	t.Helper()
	var out bytes.Buffer
	if err := htmltemplate.Must(htmltemplate.New("page").Parse(src)).Execute(&out, value); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// htmlEngineProfile is the profile the root package's HTML engine writes
// for one run of a fresh parse of src: the mode line and the newly covered
// blocks, as coverExport appends them.
func htmlEngineProfile(t *testing.T, name, src string, data any) string {
	t.Helper()
	tree, err := htt.ParseTree(name, name, src, make(map[string]*htt.Tree), lex.Options{
		EmitComment: true,
		ParseHTML:   true,
		LeftDelim:   "{{",
		RightDelim:  "}}",
		BreakOK:     true,
		ContinueOK:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	tree.ParseName, tree.Filepath = name, ""
	htt.AnnotateHTMLContexts(tree.Root)
	cp := new(htt.CoverProfile)
	if err := htt.ExecuteTemplate(io.Discard, tree, data, htt.WithCoverProfile(cp)); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, err := cp.WriteNewlyCovered(&out, make(map[string]struct{})); err != nil {
		t.Fatal(err)
	}
	return "mode: set\n" + out.String()
}
