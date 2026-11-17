package htt_test

import (
	htmltemplate "html/template"
	"strings"
	"testing"
	texttemplate "text/template"
)

type namedThing interface {
	Name() string
}

type namedStruct struct{ label string }

func (n namedStruct) Name() string { return n.label }

// TestMethodThroughPointerToInterface: a template calls a method on a
// pointer to an interface, and the method of the value inside is found.
func TestMethodThroughPointerToInterface(t *testing.T) {
	var inner namedThing = namedStruct{label: "ada"}
	data := &inner
	tests := []struct {
		name string
		mode string
		tmpl string
		want string
	}{
		{name: "text", mode: "text", tmpl: "{{.Name}}", want: "ada"},
		{name: "html", mode: "html", tmpl: "<p>{{.Name}}</p>", want: "<p>ada</p>"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var std string
			var err error
			if tc.mode == "html" {
				std, err = stdHTML(tc.tmpl, data)
			} else {
				std, err = stdTextString(tc.tmpl, data)
			}
			if err != nil {
				t.Fatalf("std(%q) error: %v", tc.tmpl, err)
			}
			if std != tc.want {
				t.Fatalf("std(%q) = %q, want %q", tc.tmpl, std, tc.want)
			}
			var got string
			if tc.mode == "html" {
				got = execHTML(t, tc.name, tc.tmpl, data)
			} else {
				got = execText(t, tc.name, tc.tmpl, data)
			}
			if got != tc.want {
				t.Errorf("htt(%q) = %q, want %q", tc.tmpl, got, tc.want)
			}
		})
	}
}

func stdTextString(tmpl string, data any) (string, error) {
	t, err := texttemplate.New("std").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

func stdHTML(tmpl string, data any) (string, error) {
	t, err := htmltemplate.New("std").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}
