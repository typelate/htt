package htt_test

import (
	"bytes"
	"testing"
	texttemplate "text/template"

	"github.com/stretchr/testify/require"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

func TestNilDataUnderMissingKeyError(t *testing.T) {
	const text = `{{.foo}}`

	t.Run("text", func(t *testing.T) {
		root, err := htt.ParseTree("t", "t", text, make(map[string]*htt.Tree), lex.Options{})
		require.NoError(t, err)

		var b bytes.Buffer
		err = htt.ExecuteTemplate(&b, root, nil, htt.WithMissingKey(htt.MissingKeyError))

		require.ErrorContains(t, err, `nil data; no entry for key "foo"`)
	})

	t.Run("html", func(t *testing.T) {
		root, err := htt.ParseTree("t", "t", text, make(map[string]*htt.Tree), lex.NewOptions())
		require.NoError(t, err)

		var b bytes.Buffer
		err = htt.ExecuteTemplate(&b, root, nil, htt.WithMissingKey(htt.MissingKeyError))

		require.ErrorContains(t, err, `nil data; no entry for key "foo"`)
	})
}

func TestNilDataUnderDefaultMissingKey(t *testing.T) {
	const text = `{{.foo}}`

	t.Run("text", func(t *testing.T) {
		tmpl := texttemplate.Must(texttemplate.New("t").Parse(text))
		var want bytes.Buffer
		require.NoError(t, tmpl.Execute(&want, nil))

		root, err := htt.ParseTree("t", "t", text, make(map[string]*htt.Tree), lex.Options{})
		require.NoError(t, err)
		var got bytes.Buffer
		require.NoError(t, htt.ExecuteTemplate(&got, root, nil))

		require.Equal(t, want.String(), got.String())
	})
}
