package htt_test

import (
	"fmt"
	"strings"
	"testing"
	texttemplate "text/template"

	"github.com/typelate/htt"
)

// valueKindsStd executes input with text/template, the oracle for each row.
func valueKindsStd(input string, data any) (string, error) {
	tmpl, err := texttemplate.New("case").Parse(input)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	err = tmpl.Execute(&b, data)
	return b.String(), err
}

// valueKindsHTT executes input with htt in text mode.
func valueKindsHTT(input string, data any) (string, error) {
	set, err := htt.NewText[any]("case").Parse(input)
	if err != nil {
		return "", err
	}
	tree, ok := set.FindTree("case")
	if !ok {
		return "", fmt.Errorf("template tree %q not found", "case")
	}
	var b strings.Builder
	err = htt.ExecuteTemplate(&b, tree, data, htt.WithFinder(set))
	return b.String(), err
}

func TestInterfaceHeldValues(t *testing.T) {
	type holder struct {
		Anything any
		Ptr      *any
	}
	inner := any([]string{"a", "b"})
	data := holder{Anything: []int{7, 8}, Ptr: &inner}
	tests := []struct {
		name, input, want string
	}{
		{"len of interface field", "{{len .Anything}}", "2"},
		{"range over interface field", "{{range .Anything}}-{{.}}-{{end}}", "-7--8-"},
		{"index through pointer to interface", "{{index .Ptr 1}}", "b"},
		{"range through pointer to interface", "{{range .Ptr}}-{{.}}-{{end}}", "-a--b-"},
		{"slice of interface field", "{{slice .Anything 1}}", "[8]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			std, err := valueKindsStd(tc.input, data)
			if err != nil {
				t.Fatalf("text/template %q: %v", tc.input, err)
			}
			if std != tc.want {
				t.Fatalf("text/template %q = %q, want %q", tc.input, std, tc.want)
			}
			got, err := valueKindsHTT(tc.input, data)
			if err != nil {
				t.Fatalf("htt %q: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("htt %q = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
