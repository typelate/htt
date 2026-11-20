package htt_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

func TestUnescapedAmpersandDiagnostic(t *testing.T) {
	const wantMessage = `& before an action starts a character reference its value can complete; write &amp;`
	for _, test := range []struct {
		name, input string
		wantPos     []string // line:column of each expected warning
	}{
		{"attribute text", `<a title="&{{.}}">x</a>`, []string{"1:11"}},
		{"attribute inside call argument", `<a onclick="f('&{{.}}')">x</a>`, []string{"1:16"}},
		{"query string", `<a href="/s?a=1&{{.K}}={{.V}}">go</a>`, []string{"1:16"}},
		{"numeric reference start", `<a title="&#{{.}};">x</a>`, []string{"1:11"}},
		{"hex reference start", `<a title="&#x{{.}};">x</a>`, []string{"1:11"}},
		{"named reference start", `<a title="&am{{.}}">x</a>`, []string{"1:11"}},
		{"letters after text", `<a title="AT&T{{.}}">x</a>`, []string{"1:13"}},
		{"element text", `<p>&{{.}}</p>`, []string{"1:4"}},
		{"trimmed space before action", `<p>a & {{- .}}</p>`, []string{"1:6"}},
		{"textarea text", `<textarea>&{{.}}</textarea>`, []string{"1:11"}},
		{"title text", `<title>&{{.}}</title>`, []string{"1:8"}},
		{"if action", `<a title="&{{if .}}x{{end}}">x</a>`, []string{"1:11"}},
		{"template call", `{{define "t"}}t{{end}}<a title="&{{template "t"}}">x</a>`, []string{"1:33"}},
		{"template comment is skipped", `<a title="&{{/* c */}}{{.}}">x</a>`, []string{"1:11"}},
		{"unquoted attribute value", `<a title=&{{.}}>x</a>`, []string{"1:10"}},
		{"two references", `<a onclick="f('&{{.A}}&{{.B}}')">x</a>`, []string{"1:16", "1:23"}},

		{"query string without ampersand before action", `<a href="/s?a=1&q={{.}}">x</a>`, nil},
		{"encoded ampersand", `<a title="&amp;{{.}}">x</a>`, nil},
		{"complete numeric reference", `<a title="&#39;{{.}}">x</a>`, nil},
		{"space after ampersand", `<p>a & {{.}}</p>`, nil},
		{"ampersand at end of value", `<a title="&">x</a>`, nil},
		{"ampersand before element end", `<p>&</p>{{.}}`, nil},
		{"script text", `<script>var a = b &{{.}};</script>`, nil},
		{"style text", `<style>a{b:c}&{{.}}</style>`, nil},
		{"html comment", `<!-- &{{.}} -->`, nil},
		{"action before ampersand", `<p>{{.}}&</p>`, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			tree, err := htt.ParseTree("test.gohtml", "test", test.input, nil, lex.NewOptions())
			require.NoError(t, err)
			var got []string
			for _, d := range tree.Diagnostics {
				if d.Code != htt.DiagUnescapedAmpersand {
					continue
				}
				p := tree.PositionAt(d.Pos)
				got = append(got, fmt.Sprintf("%d:%d", p.Line, p.Column))
				assert.Equal(t, htt.SeverityWarning, d.Severity)
				assert.Equal(t, wantMessage, d.Message)
				assert.Equal(t, byte('&'), test.input[d.Pos], "byte at Pos")
				assert.Equal(t, "{{", test.input[d.End:d.End+2], "End is where the action starts")
			}
			assert.Equal(t, test.wantPos, got)
		})
	}
}

func TestUnescapedAmpersandDiagnosticOnlyWithHTMLStructure(t *testing.T) {
	tree, err := htt.ParseTree("test.gohtml", "test", `<a title="&{{.}}">x</a>`, nil, lex.Options{})
	require.NoError(t, err)
	for _, d := range tree.Diagnostics {
		assert.NotEqual(t, htt.DiagUnescapedAmpersand, d.Code)
	}
}

func TestUnescapedAmpersandDiagnosticOnDefine(t *testing.T) {
	tree, err := htt.ParseTree("test.gohtml", "test", `{{define "t"}}<a title="&{{.}}">x</a>{{end}}`, nil, lex.NewOptions())
	require.NoError(t, err)
	trees := make(map[string]*htt.Tree)
	for name, dt := range htt.Extract(tree) {
		trees[name] = dt
	}
	require.Contains(t, trees, "t")
	var codes []htt.DiagnosticCode
	for _, d := range trees["t"].Diagnostics {
		codes = append(codes, d.Code)
	}
	assert.Equal(t, []htt.DiagnosticCode{htt.DiagUnescapedAmpersand}, codes)
}
