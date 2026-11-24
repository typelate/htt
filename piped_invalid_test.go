package htt_test

import (
	"testing"
	texttemplate "text/template"
)

// pipedInvalidFuncs take the piped value in parameters of several kinds.
var pipedInvalidFuncs = texttemplate.FuncMap{
	"echo":    func(v any) any { return v },
	"takeInt": func(i int) int { return i },
	"takePtr": func(p *diffTextInner) bool { return p == nil },
	"takeMap": func(m map[string]int) int { return len(m) },
	"count":   func(vs ...any) int { return len(vs) },
}

// A value with nothing in it, such as a missing map entry or a nil
// interface, is still an argument when it is piped to the next command:
// text/template passes it on as a nil, and the command is called with it.
func TestInvalidPipedValueMatchesTextTemplate(t *testing.T) {
	data := diffTextValue("s")
	for _, input := range []string{
		`{{.M.A|print}}`,
		`{{.M.A | printf "%v"}}`,
		`{{.M.A | printf "%v %v" 1}}`,
		`{{.E | print}}`,
		`{{echo .E | print}}`,
		`{{.M.A | echo | print}}`,
		`{{.M.A | count}}`,
		`{{.M.A | count 1 2}}`,
		`{{.M.A | len}}`,
		`{{.M.A | not}}`,
		`{{.M.A | and 1}}`,
		`{{.M.A | and 0}}`,
		`{{.M.A | or 0}}`,
		`{{.M.A | or 1}}`,
		`{{.M.A | eq 1}}`,
		`{{.M.A | html}}`,
		`{{.M.A | js}}`,
		`{{.M.A | urlquery}}`,
		`{{.M.A | index .Sl}}`,
		`{{.M.A | takeInt}}`,
		`{{.M.A | takePtr}}`,
		`{{.M.A | takeMap}}`,
		`{{.M.A | .S}}`,
		`{{.M.A | .P.X}}`,
		`{{.M.A | $.Method}}`,
		`{{.M.A | call}}`,
		`{{.M.A | call echo}}`,
		`{{$x := .M.A}}{{$x | print}}`,
		`{{with .M.A | print}}{{.}}{{else}}none{{end}}`,
		`{{if .M.A | not}}yes{{end}}`,
		`{{range .M.A | print}}{{.}}{{else}}none{{end}}`,
		`{{(.M.A) | print}}`,
		`{{.M.A | print | print}}`,
		`{{nil | print}}`,
		`{{.Nil | print}}`,
		`{{.M.A}}`,
	} {
		t.Run(input, func(t *testing.T) {
			want := runFuncsStd(input, data, pipedInvalidFuncs)
			got := runFuncsHTT(input, data, pipedInvalidFuncs)
			if got != want {
				t.Errorf("htt:           %+v\ntext/template: %+v", got, want)
			}
		})
	}
}
