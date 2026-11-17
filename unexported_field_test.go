package htt_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type unexportedHolder struct {
	secret string
	Public string
}

type unexportedEmbedded struct {
	inner
}

type inner struct {
	Name string
}

func TestUnexportedFieldIsAnError(t *testing.T) {
	const wantTail = "is an unexported field of struct type"
	tests := []struct {
		name string
		tmpl string
		data any
		html bool
		// wantErr, when set, is a substring the htt error must contain.
		wantErr string
		// wantOut, when set, is the rendered output of a template that must succeed.
		wantOut string
	}{
		{
			name:    "text: unexported field of a struct value",
			tmpl:    `{{.secret}}`,
			data:    unexportedHolder{secret: "x"},
			wantErr: "secret " + wantTail + " htt_test.unexportedHolder",
		},
		{
			name:    "html: unexported field of a struct value",
			tmpl:    `<p>{{.secret}}</p>`,
			data:    unexportedHolder{secret: "x"},
			html:    true,
			wantErr: "secret " + wantTail + " htt_test.unexportedHolder",
		},
		{
			name:    "text: unexported field of a pointer to a struct names the pointer type",
			tmpl:    `{{.secret}}`,
			data:    &unexportedHolder{secret: "x"},
			wantErr: "secret " + wantTail + " *htt_test.unexportedHolder",
		},
		{
			name:    "text: exported field promoted through an unexported embedded struct",
			tmpl:    `{{.Name}}`,
			data:    unexportedEmbedded{inner: inner{Name: "ok"}},
			wantOut: "ok",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantErr != "" {
				var err error
				if tc.html {
					err = execHTMLErr(t, "x", tc.tmpl, tc.data)
				} else {
					err = execTextErr(t, "x", tc.tmpl, tc.data)
				}
				require.Error(t, err, "execute %q", tc.tmpl)
				assert.ErrorContains(t, err, tc.wantErr, "execute %q error = %v, want it to contain %q", tc.tmpl, err, tc.wantErr)
				return
			}
			got := execText(t, "x", tc.tmpl, tc.data)
			assert.Equal(t, tc.wantOut, got, "execute %q", tc.tmpl)
		})
	}
}
