package htt_test

import (
	"bytes"
	"testing"
	texttemplate "text/template"

	"github.com/stretchr/testify/require"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

func TestNilIsNotACommand(t *testing.T) {
	for _, tt := range []struct {
		name    string
		text    string
		wantErr string
	}{
		{name: "bare nil action", text: `{{nil}}`, wantErr: "nil is not a command"},
		{name: "if nil", text: `{{if nil}}x{{end}}`, wantErr: "nil is not a command"},
		{name: "variable assigned nil", text: `{{$x := nil}}`, wantErr: "nil is not a command"},
		{name: "field of nil", text: `{{(nil).True}}`, wantErr: "nil is not a command"},
		{name: "nil as printf argument", text: `{{printf "%v" nil}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root, err := htt.ParseTree("t", "t", tt.text, make(map[string]*htt.Tree), lex.Options{})
			require.NoError(t, err)

			var b bytes.Buffer
			err = htt.ExecuteTemplate(&b, root, map[string]any{})
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, referenceText(t, tt.text), b.String())
		})
	}
}

// referenceText executes text through text/template and returns its output.
func referenceText(t *testing.T, text string) string {
	t.Helper()
	tmpl, err := texttemplate.New("t").Parse(text)
	require.NoError(t, err)
	var b bytes.Buffer
	require.NoError(t, tmpl.Execute(&b, map[string]any{}))
	return b.String()
}
