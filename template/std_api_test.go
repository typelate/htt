package template_test

import (
	"io"
	"strings"
	"testing"
	texttemplate "text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/htt"
	"github.com/typelate/htt/template"
)

func TestExecErrorIsTextTemplates(t *testing.T) {
	tmpl := template.Must(template.New("t").Parse(`{{.X}}`))
	err := tmpl.Execute(io.Discard, 1)

	var execErr template.ExecError
	require.ErrorAs(t, err, &execErr)
	assert.Equal(t, "t", execErr.Name)
	var std texttemplate.ExecError = execErr
	assert.Equal(t, err.Error(), std.Error())
}

// text/template's Template embeds its parse tree, so the tree's fields and
// methods can be selected on the template.
func TestTemplatePromotesItsTree(t *testing.T) {
	const text = "a\n{{ .X }}"
	std := texttemplate.Must(texttemplate.New("t").Parse(text))
	wantLocation, wantContext := std.ErrorContext(std.Root.Nodes[1])

	tmpl := template.Must(template.New("t").Parse(text))
	require.NotNil(t, tmpl.Tree)
	assert.Same(t, tmpl.Tree.Root, tmpl.Root)
	assert.Equal(t, std.ParseName, tmpl.ParseName)
	assert.Equal(t, tmpl.Tree.Mode, tmpl.Mode)
	location, context := tmpl.ErrorContext(firstAction(t, tmpl.Root))
	assert.Equal(t, wantLocation, location)
	assert.Equal(t, wantContext, context)
	assert.Equal(t, "t", tmpl.Name(), "Name is the template's method, not the tree's field")

	cp := tmpl.Copy()
	require.NotNil(t, cp)
	assert.NotSame(t, tmpl.Tree, cp)
	assert.Equal(t, tmpl.Root.String(), cp.Root.String())
}

// A copy of a template's tree runs in another set as the template runs in
// its own.
func TestACopiedTreeRunsInAnotherSet(t *testing.T) {
	const value = `<b>'+"x"/&`
	other := template.Must(template.New("other").Parse(`<a href="/x?q={{.}}" title="{{.}}">{{.}}</a>`))
	set := template.New("main")
	_, err := set.AddParseTree("added", other.Copy())
	require.NoError(t, err)

	var want, got strings.Builder
	require.NoError(t, other.Execute(&want, value))
	require.NoError(t, set.ExecuteTemplate(&got, "added", value))
	assert.Equal(t, want.String(), got.String())
}

// firstAction returns the first action under n.
func firstAction(t *testing.T, n htt.Node) *htt.ActionNode {
	t.Helper()
	var found *htt.ActionNode
	htt.Walk(&htt.Tree{Root: n.(*htt.ListNode)}, func(_ *htt.Tree, n htt.Node) {
		if action, ok := n.(*htt.ActionNode); ok && found == nil {
			found = action
		}
	})
	require.NotNil(t, found)
	return found
}
