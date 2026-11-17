package htt_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/typelate/htt/template"
)

// A tag with an action in an attribute name is written as text, and the
// executor follows that text to know where it stands in the tag. These
// are the errors for what it cannot follow there.
func TestErrorsInATagWrittenAsText(t *testing.T) {
	for _, tt := range []struct {
		name string
		text string
		want string
	}{
		{
			name: "an element starts",
			text: `{{define "t"}}<b>x</b>{{end}}<p data-{{.K}}=1 {{template "t"}}>`,
			want: `<b> in a tag that htt cannot follow`,
		},
		{
			name: "an element ends",
			text: `<b>{{if .K}}<i data-{{.K}}=1 {{end}}</b>`,
			want: `</b> in a tag that htt cannot follow`,
		},
		{
			name: "an action after a quote in an unquoted value",
			text: `<p data-{{.K}}=a"b{{.V}}>`,
			want: `an action in a tag that htt cannot follow`,
		},
		{
			name: "a template call among attributes",
			text: `{{define "t"}}id=x{{end}}<p {{template "t"}}>`,
			want: `{{template "t"}} in a tag is not supported by htt`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := template.New("page").Parse(tt.text)
			require.NoError(t, err)
			err = tmpl.Execute(new(bytes.Buffer), map[string]string{"K": "k", "V": "v"})
			require.EqualError(t, err, tt.want, "Execute(%q)", tt.text)
		})
	}
}
