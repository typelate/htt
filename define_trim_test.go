package htt_test

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

func TestDefineTrimMarkersInATextParse(t *testing.T) {
	for _, tt := range []struct {
		name string
		text string
	}{
		{name: "opener and end trim the body", text: `{{define "x" -}} a {{- end}}[{{template "x"}}]`},
		{name: "no markers", text: `{{define "x"}} a {{end}}[{{template "x"}}]`},
		{name: "left marker trims before, right marker trims after", text: `before {{- define "x"}}a{{end -}} after{{template "x"}}`},
		{name: "opener right marker and end right marker", text: `before {{define "x" -}} a {{end}} after|{{template "x"}}|`},
		{name: "end left marker without outward markers", text: `{{define "x"}} a {{- end}} after|{{template "x"}}|`},
		{name: "two defines with markers between", text: `{{define "x"}} a {{end -}}   {{- define "y"}} b {{end}}|{{template "x"}}{{template "y"}}|`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wantTmpl := template.Must(template.New("t").Parse(tt.text))
			var wantBuf bytes.Buffer
			if err := wantTmpl.Execute(&wantBuf, nil); err != nil {
				t.Fatalf("text/template execute(%q): %v", tt.text, err)
			}
			want := wantBuf.String()

			treeSet := make(map[string]*htt.Tree)
			root, err := htt.ParseTree("t", "t", tt.text, treeSet, lex.Options{})
			if err != nil {
				t.Fatalf("htt parse(%q): %v", tt.text, err)
			}
			trees := htt.DefinedTrees(root, treeSet)
			finder := htt.FindTreeFunc(func(name string) (*htt.Tree, bool) {
				tree, ok := trees[name]
				return tree, ok
			})
			var gotBuf bytes.Buffer
			if err := htt.ExecuteTemplate(&gotBuf, root, nil, htt.WithFinder(finder)); err != nil {
				t.Fatalf("htt execute(%q): %v", tt.text, err)
			}
			if got := gotBuf.String(); got != want {
				t.Errorf("execute(%q) = %q, want %q", tt.text, got, want)
			}
		})
	}
}
