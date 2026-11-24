package htt_test

import (
	"strings"
	"testing"
)

type noValueData struct {
	E  any
	M  map[string]any
	MI map[string]int
}

// A value with nothing in it reaches html/template's escapers as a nil
// argument, whether it is a nil interface or a missing map entry, and they
// write nothing for it in text and null in a script.
func TestNoValueInHTMLMatchesHTMLTemplate(t *testing.T) {
	data := noValueData{M: map[string]any{}, MI: map[string]int{}}
	for _, tmpl := range []string{
		`<p>{{.E}}</p>`,
		`<p>{{.M.missing}}</p>`,
		`<p>{{.MI.missing}}</p>`,
		`<a title="{{.E}}">x</a>`,
		`<a title="{{.M.missing}}">x</a>`,
		`<script>var x = {{.E}};</script>`,
		`<script>var x = {{.M.missing}};</script>`,
		`<p>{{.E | print}}</p>`,
		`<p>{{.M.missing | print}}</p>`,
	} {
		t.Run(tmpl, func(t *testing.T) {
			want, err := stdHTML(tmpl, data)
			if err != nil {
				t.Fatalf("html/template: %v", err)
			}
			got := execHTML(t, "t", tmpl, data)
			if strings.ReplaceAll(got, " ", "") != strings.ReplaceAll(want, " ", "") {
				t.Errorf("htt = %q, html/template = %q (spaces removed for the comparison)", got, want)
			}
		})
	}
}
