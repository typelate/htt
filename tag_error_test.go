package htt_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/typelate/htt"
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
			want: `template: page:1:14: executing "t" at <<b>x</b>>: <b> in a tag that htt cannot follow`,
		},
		{
			name: "an element ends",
			text: `<b>{{if .K}}<i data-{{.K}}=1 {{end}}</b>`,
			want: `template: page:1:0: executing "page" at <<b>{{if .K}}<i data-{{.K}}=1 {{end}}</b>>: </b> in a tag that htt cannot follow`,
		},
		{
			name: "an action after a quote in an unquoted value",
			text: `<p data-{{.K}}=a"b{{.V}}>`,
			want: `template: page:1:20: executing "page" at <{{.V}}>: an action in a tag that htt cannot follow`,
		},
		{
			name: "a template call among attributes",
			text: `{{define "t"}}id=x{{end}}<p {{template "t"}}>`,
			want: `template: page:1:25: executing "page" at <<p {{template "t"}}>>: {{template "t"}} in a tag is not supported by htt`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			set, err := htt.New[any]("page").Parse(tt.text)
			require.NoError(t, err)
			tree, ok := set.FindTree("page")
			require.True(t, ok, "no tree named page")
			err = htt.ExecuteTemplate(new(bytes.Buffer), tree, map[string]string{"K": "k", "V": "v"}, htt.WithFinder(set))
			require.EqualError(t, err, tt.want, "Execute(%q)", tt.text)
		})
	}
}
