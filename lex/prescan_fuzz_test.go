package lex

import (
	"strings"
	"testing"
)

// tagLexerErrors are the structural errors of the tag lexer. In HTML mode the
// pre-scan sends markup that would raise one down the text path instead.
var tagLexerErrors = []string{
	"expected tag name",
	"unclosed HTML tag",
	"unexpected / in HTML tag",
	"expected attribute name",
	"unterminated attribute value",
	"expected attribute value",
	"expected > to close end tag",
	"unclosed HTML comment",
	"unclosed DOCTYPE",
}

// scanAll scans input to EOF or the first error and returns the tokens. It
// reports false when more than limit tokens come out.
func scanAll(input string, options Options, limit int) (tokens []Token, ok bool) {
	l := New(input, options)
	for {
		tok := l.Next()
		tokens = append(tokens, tok)
		if tok.Type == TokenTypeEOF || tok.Type == TokenTypeError {
			return tokens, true
		}
		if len(tokens) > limit {
			return tokens, false
		}
	}
}

func firstError(tokens []Token) (Token, bool) {
	for _, tok := range tokens {
		if tok.Type == TokenTypeError {
			return tok, true
		}
	}
	return Token{}, false
}

var prescanSeeds = []string{
	"",
	"<",
	"a < b",
	"<div>text</div>",
	"<div class=a id='b' hidden>x</div>",
	`<a href="/x/{{.ID}}">l</a>`,
	`<a href='{{.A}}' title="{{if .T}}x{{end}}">l</a>`,
	"<div class={{.C}}>x</div>",
	"<div class=a{{.C}}b>x</div>",
	"<div class={{.C}} id=x>x</div>",
	"<h{{.N}}>x</h{{.N}}>",
	"<{{.Tag}}>x</{{.Tag}}>",
	"<div {{.Attr}}>x</div>",
	"<div {{.Attr}}=x>x</div>",
	"<div {{.Attr}} = x>x</div>",
	"<div data-{{.K}}=v>x</div>",
	"<div data-{{.K}}>x</div>",
	"<li{{if .X}} hidden{{end}}>x</li>",
	"<li{{if .X}}hidden{{end}}>x</li>",
	"<li{{if .X -}} hidden{{end}}>x</li>",
	"<input hidden{{if .X}} disabled{{end}}>",
	"<input {{if .X}}disabled{{else}}readonly{{end}}>",
	"<input {{if .X}}disabled>",
	"<input {{end}}>",
	"<input {{else}}>",
	`<input {{define "x"}}>`,
	"<input {{/* c */}} hidden>",
	"<input {{/* c */ -}} hidden>",
	"<input {{/* unclosed",
	"<input {{- if .X -}} hidden {{- end -}}>",
	"<input {{ if",
	`<input {{"}}"}} a>`,
	"<input {{`}}`}} a>",
	`<input a="{{.X}}" b='{{.Y}}'>`,
	`<input a="{{if .X}}1{{end}}">`,
	`<input a="{{if .X}}1">`,
	`<input a="{{end}}">`,
	`<input a="{{.X`,
	`<input a="unterminated`,
	"<input a=",
	"<input a=>",
	"<input a= >",
	"<input a='b",
	"<input /",
	"<input / >",
	"<input =a>",
	"<input a=b/>",
	"<br/>",
	"<br />",
	"</div>",
	"</div >",
	"</div x>",
	"</div",
	"</ div>",
	"</{{.T}}>",
	"</d{{.T}}>",
	"</div {{if .X}}>{{end}}",
	"</>",
	"</",
	"<!doctype html>",
	"<!DOCTYPE html {{.X}}>",
	"<!DOCTYPE html",
	"<?xml version=1.0?>",
	"<?xml {{.X}}?>",
	"<!-- c -->",
	"<!-- {{.X}} -->",
	"<!-->",
	"<!--->",
	"<!-- unclosed",
	"<!-- {{if .X}} -->{{end}}",
	"<!-- {{if .X}} --> -->{{end}}",
	"{{if .X}}<!-- {{end}} -->",
	"<!-- {{if .X}}a{{end}} -->",
	"<!-- {{else}} -->",
	"<!-- {{end}} -->",
	`<!-- {{define "x"}} -->`,
	"<!-- {{/* c */}} -->",
	"<!-- {{.X",
	"<!-- {{- .X -}} -->",
	"{{if .X}}<div>{{else}}<p>{{end}}",
	"{{if .X}}<div {{end}}>",
	"{{range .}}<li>{{.}}</li>{{end}}",
	"{{with .}}<a href=\"{{.}}\">{{end}}",
	`{{block "b" .}}<b>{{end}}`,
	"<script>if (a < b) {}</script>",
	"<style>a>b{}</style>",
	"<textarea><b></textarea>",
	"<title>{{.T}}</title>",
	"<a\nhref=\"x\"\n>y</a>",
	"<a\thref=x\t>y</a>",
	"<div<div>",
	"<1>",
	"<é>",
	"<a b=\"c\"d>",
	`<a b="c" d='e' f=g h>`,
	"<a b = 'c' >",
	"{{.}}<{{.}}",
	"<a{{.}}",
	"<a {{.}}",
	"<a b{{.}}",
	"<a b={{.}}",
	"<a b='{{.}}",
	"<a b={{if .}}c{{end}}>",
	"<a b={{if .}}c>",
	"<a b= {{end}}>",
}

// FuzzHTMLPreScanAgreesWithTagLexer checks that the pre-scan sends to the
// text path every markup the tag lexer would reject, and so that every
// template text/template accepts also lexes in HTML mode.
func FuzzHTMLPreScanAgreesWithTagLexer(f *testing.F) {
	for _, seed := range prescanSeeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		limit := 4*len(input) + 16
		base := Options{EmitComment: true, BreakOK: true, ContinueOK: true}
		textTokens, ok := scanAll(input, base, limit)
		if !ok {
			t.Fatalf("text mode scan of %q exceeded %d tokens", input, limit)
		}
		html := base
		html.ParseHTML = true
		htmlTokens, ok := scanAll(input, html, limit)
		if !ok {
			t.Fatalf("HTML mode scan of %q exceeded %d tokens", input, limit)
		}

		if _, failed := firstError(textTokens); !failed {
			if tok, failed := firstError(htmlTokens); failed {
				t.Errorf("text mode lexes %q, HTML mode fails: %q", input, tok.Value)
			}
		}

		if tok, failed := firstError(htmlTokens); failed {
			for _, msg := range tagLexerErrors {
				if strings.HasPrefix(tok.Value, msg) {
					t.Errorf("HTML mode on %q reports the tag lexer error %q; the pre-scan should have degraded it", input, tok.Value)
				}
			}
		}

		var prevEnd Pos
		for n, tok := range htmlTokens {
			if tok.Pos > tok.End {
				t.Errorf("%q token %d %v: Pos %d > End %d", input, n, tok.Type, tok.Pos, tok.End)
			}
			if tok.Pos < prevEnd {
				t.Errorf("%q token %d %v: Pos %d < previous End %d", input, n, tok.Type, tok.Pos, prevEnd)
			}
			if int(tok.End) > len(input) {
				t.Errorf("%q token %d %v: End %d > len %d", input, n, tok.Type, tok.End, len(input))
			}
			prevEnd = tok.End
		}
	})
}
