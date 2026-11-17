package htt_test

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/typelate/htt"
)

// coverFromRow is one template run for TestCoverFromMatchesTheHTMLTreesOwnRun.
// entry names the tree to execute. file, when set, is read as the source.
type coverFromRow struct {
	name   string
	source string
	file   string
	entry  string
	data   any
}

// coverFromFileData covers the fields the testdata/*.gohtml files read.
var coverFromFileData = map[string]any{
	"Title":          "Title",
	"TextColor":      "red",
	"Content":        "content",
	"DefaultText":    "default",
	"JSONConfig":     "{}",
	"ExtraScript":    true,
	"ExtraScriptSrc": "/extra.js",
	"ChartData":      "[]",
	"Data":           "data",
	"URL":            "/url",
	"DarkMode":       true,
	"HasChart":       true,
	"Endpoints":      []string{},
}

func TestCoverFromMatchesTheHTMLTreesOwnRun(t *testing.T) {
	tests := []coverFromRow{
		{name: "element with attributes and text action", source: `<p class="a" id="b">hi {{.}}</p>`, entry: "root", data: "x"},
		{name: "action inside quoted attribute", source: `<a href="/items/{{.}}">link</a>`, entry: "root", data: "7"},
		{name: "if true around elements", source: `{{if .}}<b>yes</b>{{else}}<i>no</i>{{end}}`, entry: "root", data: true},
		{name: "if false around elements", source: `{{if .}}<b>yes</b>{{else}}<i>no</i>{{end}}`, entry: "root", data: false},
		{name: "range over zero items with else", source: `<ul>{{range .}}<li>{{.}}</li>{{else}}<li>none</li>{{end}}</ul>`, entry: "root", data: []string{}},
		{name: "range over three items with else", source: `<ul>{{range .}}<li>{{.}}</li>{{else}}<li>none</li>{{end}}</ul>`, entry: "root", data: []string{"a", "b", "c"}},
		{name: "with around an element", source: `{{with .}}<h1>{{.}}</h1>{{end}}`, entry: "root", data: "t"},
		{name: "define called twice", source: `{{define "item"}}<li>{{.}}</li>{{end}}{{template "item" .}}{{template "item" .}}`, entry: "root", data: "v"},
		{name: "void elements", source: `<img src="x"><br><input value="{{.}}">`, entry: "root", data: "v"},
		{name: "comment and doctype", source: `<!DOCTYPE html><!-- note --><p>{{.}}</p>`, entry: "root", data: "x"},
		{name: "script and style with actions", source: `<script>var x = {{.}};</script><style>p { color: {{.}}; }</style>`, entry: "root", data: "red"},
		{name: "trim markers between elements", source: "<p>a</p>\n  {{- . -}}\n  <p>b</p>", entry: "root", data: "x"},
		{name: "unclosed li with implied end tags", source: `<ul><li>one<li>two {{.}}</ul>`, entry: "root", data: "x"},
		{name: "attribute inside if, true", source: `<input {{if .}}checked{{end}}>`, entry: "root", data: true},
		{name: "attribute inside if, false", source: `<input {{if .}}checked{{end}}>`, entry: "root", data: false},
		{name: "testdata raw_text_elements", file: "testdata/raw_text_elements.gohtml", entry: "page", data: coverFromFileData},
		{name: "testdata script_heavy", file: "testdata/script_heavy.gohtml", entry: "dashboard", data: coverFromFileData},
		{name: "testdata assets_report", file: "testdata/assets_report.gohtml", entry: "assets_report", data: map[string]any{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := test.source
			if test.file != "" {
				buf, err := os.ReadFile(test.file)
				if err != nil {
					t.Fatal(err)
				}
				source = string(buf)
			}

			htmlRun := coverFromSet(t, false, source)
			if err := coverFromExecute(htmlRun, test.entry, test.data); err != nil {
				t.Fatalf("HTML execute %s: %v", test.entry, err)
			}
			want := coverFromCounts(t, htmlRun)

			htmlFrom := coverFromSet(t, false, source)
			textRun := coverFromSet(t, true, source)
			if err := coverFromExecute(textRun, test.entry, test.data); err != nil {
				t.Fatalf("text execute %s: %v", test.entry, err)
			}
			for _, name := range htmlFrom.Names() {
				htmlTree, ok := htmlFrom.FindTree(name)
				if !ok {
					t.Fatalf("HTML set has no tree %q", name)
				}
				textTree, ok := textRun.FindTree(name)
				if !ok {
					t.Fatalf("text set has no tree %q", name)
				}
				htmlTree.CoverFrom(textTree)
			}
			got := coverFromCounts(t, htmlFrom)

			if !slices.Equal(slices.Sorted(maps.Keys(want)), slices.Sorted(maps.Keys(got))) {
				t.Fatalf("block keys differ\nHTML run: %v\nCoverFrom: %v", slices.Sorted(maps.Keys(want)), slices.Sorted(maps.Keys(got)))
			}
			for _, key := range slices.Sorted(maps.Keys(want)) {
				if (want[key] == 0) != (got[key] == 0) {
					t.Errorf("%s: HTML run count %d, CoverFrom count %d (zero/non-zero differs)", key, want[key], got[key])
				} else if want[key] != got[key] {
					t.Errorf("%s: HTML run count %d, CoverFrom count %d", key, want[key], got[key])
				}
			}
		})
	}
}

// coverFromSet parses source as the body of a set named "root", with HTML
// structure unless text is set.
func coverFromSet(t *testing.T, text bool, source string) *htt.Template[any] {
	t.Helper()
	set := htt.New[any]("root")
	if text {
		set = htt.NewText[any]("root")
	}
	// assets_report.gohtml calls functions that no test registers. Its
	// calls sit in branches the empty data never takes.
	set = set.SkipFuncCheck()
	set, err := set.Parse(source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return set
}

// coverFromExecute runs the tree named entry in set with data.
func coverFromExecute(set *htt.Template[any], entry string, data any) error {
	tree, ok := set.FindTree(entry)
	if !ok {
		return fmt.Errorf("no tree named %s", entry)
	}
	var b strings.Builder
	return htt.ExecuteTemplate(&b, tree, data, htt.WithFinder(set))
}

// coverFromCounts maps each coverage block of every tree in set to its count.
func coverFromCounts(t *testing.T, set *htt.Template[any]) map[string]int64 {
	t.Helper()
	counts := make(map[string]int64)
	for _, name := range set.Names() {
		tree, ok := set.FindTree(name)
		if !ok {
			continue
		}
		for _, b := range tree.CoverBlocks() {
			// A define body is listed by both the tree that holds it and
			// its own tree; both entries read one node, so either value works.
			counts[b.String()] = b.Count
		}
	}
	return counts
}
