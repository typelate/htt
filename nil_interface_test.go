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

type nilInterfaceHolder struct {
	I namedThing
	P *namedThing
	M map[string]namedThing
}

// TestMethodOnNilInterfaceFails: a method call on a nil interface value is
// an execution error that names the field, not a panic.
func TestMethodOnNilInterfaceFails(t *testing.T) {
	var nilThing namedThing
	data := &nilInterfaceHolder{P: &nilThing, M: map[string]namedThing{"k": nil}}
	tests := []struct {
		name    string
		mode    string
		tmpl    string
		wantErr bool
	}{
		{name: "field", mode: "text", tmpl: "{{.I.Name}}", wantErr: true},
		{name: "pointer to interface", mode: "text", tmpl: "{{.P.Name}}", wantErr: true},
		{name: "with arguments", mode: "text", tmpl: "{{.I.Name 1}}", wantErr: true},
		{name: "map element", mode: "text", tmpl: "{{.M.k.Name}}", wantErr: true},
		{name: "variable", mode: "text", tmpl: "{{$x := .I}}{{$x.Name}}", wantErr: true},
		{name: "html field", mode: "html", tmpl: "<p>{{.I.Name}}</p>", wantErr: true},
		{name: "with guards nil", mode: "text", tmpl: "{{with .I}}{{.Name}}{{end}}", wantErr: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.wantErr {
				var std string
				var err error
				if tc.mode == "html" {
					std, err = stdHTML(tc.tmpl, data)
				} else {
					std, err = stdTextString(tc.tmpl, data)
				}
				if err != nil || std != "" {
					t.Fatalf("std(%q) = %q, %v; want empty output and no error", tc.tmpl, std, err)
				}
				var got string
				if tc.mode == "html" {
					got = execHTML(t, tc.name, tc.tmpl, data)
				} else {
					got = execText(t, tc.name, tc.tmpl, data)
				}
				if got != "" {
					t.Errorf("htt(%q) = %q, want empty output", tc.tmpl, got)
				}
				return
			}
			var stdErr error
			if tc.mode == "html" {
				_, stdErr = stdHTML(tc.tmpl, data)
			} else {
				_, stdErr = stdTextString(tc.tmpl, data)
			}
			if stdErr == nil || !strings.Contains(stdErr.Error(), "nil pointer evaluating") {
				t.Fatalf("std(%q) error = %v, want nil pointer evaluating", tc.tmpl, stdErr)
			}
			var gotErr error
			if tc.mode == "html" {
				gotErr = execHTMLErr(t, tc.name, tc.tmpl, data)
			} else {
				gotErr = execTextErr(t, tc.name, tc.tmpl, data)
			}
			if gotErr == nil || !strings.Contains(gotErr.Error(), "nil pointer evaluating") {
				t.Errorf("htt(%q) error = %v, want nil pointer evaluating", tc.tmpl, gotErr)
			}
		})
	}
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
