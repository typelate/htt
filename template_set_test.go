package htt_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/typelate/htt"
)

func TestAssociatedTemplateParsesWithTheSetsFunctions(t *testing.T) {
	set := htt.New[int]("root").Functions(map[string]int{"f": 1})
	_, err := set.Parse(`{{define "a"}}x{{end}}`)
	require.NoError(t, err)

	_, err = set.Lookup("a").Parse("{{f}}")
	require.NoError(t, err, `Lookup("a").Parse("{{f}}")`)
}

func TestFunctionRegisteredOnAnAssociatedTemplateIsKnownToTheSet(t *testing.T) {
	set := htt.New[int]("root")
	_, err := set.Parse(`{{define "a"}}x{{end}}`)
	require.NoError(t, err)
	set.Lookup("a").Functions(map[string]int{"g": 2})

	_, err = set.Parse("{{g}}")
	require.NoError(t, err, `Parse("{{g}}") after Lookup("a").Functions`)
}

func TestAssociatedTemplateInheritsDelimiters(t *testing.T) {
	set := htt.New[int]("root").Delimiters("<<", ">>")
	_, err := set.Parse(`<<define "a">>x<<end>>`)
	require.NoError(t, err)

	a, err := set.Lookup("a").Parse("<<.>>")
	require.NoError(t, err, `Lookup("a").Parse("<<.>>")`)

	var out bytes.Buffer
	require.NoError(t, htt.ExecuteTemplate(&out, a.Tree(), 1))
	require.Equal(t, "1", out.String(), `Lookup("a") executed with data 1`)
}

func TestAssociatedTemplateSharesSkipFuncCheck(t *testing.T) {
	set := htt.New[int]("root").SkipFuncCheck()
	_, err := set.Parse(`{{define "a"}}x{{end}}`)
	require.NoError(t, err)

	_, err = set.Lookup("a").Parse("{{unknown}}")
	require.NoError(t, err, `Lookup("a").Parse("{{unknown}}") with SkipFuncCheck`)
}

func TestZeroTemplateLookupAndFunctions(t *testing.T) {
	var set htt.Template[int]
	require.Nil(t, set.Lookup("x"), `Lookup("x") of a zero Template`)
	require.Empty(t, set.Names(), "Names() of a zero Template")
	_, ok := set.FindTree("x")
	require.False(t, ok, `FindTree("x") of a zero Template`)
	require.Same(t, &set, set.Functions(map[string]int{"f": 1}), "Functions() returns the receiver")
}
