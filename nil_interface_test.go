package htt_test

import (
	"fmt"
	"strings"
	"testing"
	texttemplate "text/template"

	"github.com/typelate/htt"
)

type nilInterfaceData struct {
	Nil any
}

func TestNilInterfaceValues(t *testing.T) {
	tests := []struct {
		name  string
		input string
		data  any
		want  string
	}{
		{"nil interface field prints no value", "{{.Nil}}", nilInterfaceData{}, "<no value>"},
		{"field of nil data prints no value", "{{.foo}}", nil, "<no value>"},
		{"field of parenthesized nil data prints no value", "{{(.).foo}}", nil, "<no value>"},
		{"and with undefined field is no value", "{{and 1 .Unknown}}", nil, "<no value>"},
		{"or with undefined field is no value", "{{or 0 .Unknown}}", nil, "<no value>"},
		{"range over nil interface field is empty", "{{range .Nil}}-{{.}}-{{end}}", nilInterfaceData{}, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdGot, err := executeStd(test.input, test.data)
			if err != nil {
				t.Fatalf("text/template execute(%q) error: %v", test.input, err)
			}
			if stdGot != test.want {
				t.Fatalf("text/template execute(%q) = %q, want %q", test.input, stdGot, test.want)
			}
			got, err := executeHTT(test.input, test.data)
			if err != nil {
				t.Fatalf("htt execute(%q) error: %v", test.input, err)
			}
			if got != test.want {
				t.Errorf("htt execute(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func executeStd(input string, data any) (string, error) {
	tmpl, err := texttemplate.New("nil").Parse(input)
	if err != nil {
		return "", fmt.Errorf("parse %q: %w", input, err)
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, data); err != nil {
		return b.String(), fmt.Errorf("execute: %w", err)
	}
	return b.String(), nil
}

func executeHTT(input string, data any) (string, error) {
	set, err := htt.NewText[any]("nil").Parse(input)
	if err != nil {
		return "", fmt.Errorf("parse %q: %w", input, err)
	}
	tree, ok := set.FindTree("nil")
	if !ok {
		return "", fmt.Errorf("no tree named nil")
	}
	var b strings.Builder
	if err := htt.ExecuteTemplate(&b, tree, data, htt.WithFinder(set)); err != nil {
		return b.String(), fmt.Errorf("execute: %w", err)
	}
	return b.String(), nil
}
