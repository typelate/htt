package template_test

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/htt"
	"github.com/typelate/htt/template"
)

// html/template's Error.Node is a parse.Node; this package's is the node
// type of its own trees.
func TestEscapingErrorHoldsItsNode(t *testing.T) {
	tmpl := template.Must(template.New("t").Parse(`<a href="{{if .C}}/path/{{else}}/search?q={{end}}{{.X}}">`))
	err := tmpl.Execute(io.Discard, nil)

	var escErr *template.Error
	require.ErrorAs(t, err, &escErr)
	var node htt.Node = escErr.Node
	action, ok := node.(*htt.ActionNode)
	require.True(t, ok, "the node is the action")
	assert.Equal(t, `{{.X}}`, action.String())
}
