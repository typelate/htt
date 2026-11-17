package lex

import "testing"

func prescanner(input string, emitComment bool) *Scanner {
	return New(input, Options{ParseHTML: true, EmitComment: emitComment})
}

func TestHTMLActionAt(t *testing.T) {
	tests := []struct {
		input string
		i     int
		kind  int
		end   int
	}{
		{"{{.X}}", 0, htmlActionOther, 6},
		{"ab{{end}}", 2, htmlActionEnd, 9},
		{"{{if .X}}", 0, htmlActionOpen, 9},
		{"{{with .}}", 0, htmlActionOpen, 10},
		{"{{range .}}", 0, htmlActionOpen, 11},
		{`{{block "b" .}}`, 0, htmlActionOpen, 15},
		{"{{else}}", 0, htmlActionElse, 8},
		{"{{else if .X}}", 0, htmlActionElse, 14},
		{"{{end}}", 0, htmlActionEnd, 7},
		{"{{ end }}", 0, htmlActionEnd, 9},
		{`{{define "x"}}`, 0, htmlActionDefine, 14},
		{"{{- if .X -}}", 0, htmlActionOpen, 13},
		{"{{-if .X}}", 0, htmlActionOther, 10}, // not a trim marker: no space
		{"{{iffy}}", 0, htmlActionOther, 8},
		{"{{/* c */}}", 0, htmlActionComment, 11},
		{"{{- /* c */ -}}", 0, htmlActionComment, 15},
		{"{{/* c */ }}", 0, htmlActionComment, -1}, // a comment must touch the delimiter
		{"{{/* c", 0, htmlActionComment, -1},
		{"{{.X", 0, htmlActionOther, -1},
		{"{{if .X", 0, htmlActionOpen, -1},
		{`{{"}}"}}`, 0, htmlActionOther, 8},
		{"{{`}}`}}", 0, htmlActionOther, 8},
		{`{{"a\"}}"}}`, 0, htmlActionOther, 11},
		{"{{`a\\`}}", 0, htmlActionOther, 8}, // no escapes in a raw string
		{`{{'}}'}}`, 0, htmlActionOther, 8},
		{`{{"unterminated}}`, 0, htmlActionOther, -1},
		{"{{/**/}}", 0, htmlActionComment, 8}, // empty comment: "*/" at offset 0
		{"{{ ", 0, htmlActionOther, -1},
		{"{{", 0, htmlActionOther, -1},
		{"{{endz}}", 0, htmlActionOther, 8}, // z ends a keyword-like word last
		{"{{enda}}", 0, htmlActionOther, 8},
	}
	for _, tt := range tests {
		l := prescanner(tt.input, true)
		kind, end := l.htmlActionAt(tt.i)
		if kind != tt.kind || end != tt.end {
			t.Errorf("htmlActionAt(%q, %d) = (%d, %d), want (%d, %d)", tt.input, tt.i, kind, end, tt.kind, tt.end)
		}
	}

	l := New("<<end>>", Options{ParseHTML: true, LeftDelim: "<<", RightDelim: ">>"})
	if kind, end := l.htmlActionAt(0); kind != htmlActionEnd || end != 7 {
		t.Errorf("htmlActionAt(%q, 0) with <<>> = (%d, %d), want (%d, %d)", "<<end>>", kind, end, htmlActionEnd, 7)
	}
}

func TestHTMLActionOpensAttrs(t *testing.T) {
	tests := []struct {
		input string
		i     int
		want  bool
	}{
		{"<li{{if .X}} hidden{{end}}>", 3, true},
		{"{{if .X}}\n a", 0, true},
		{"{{if .X}}\ta", 0, true},
		{"{{range .}} a", 0, true},
		{"{{with .}} a", 0, true},
		{"{{- if .X}} a", 0, true},
		{"{{if .X-}} a", 0, true}, // "X-" is no trim marker
		{"{{if .X}}hidden{{end}}", 0, false},
		{"{{if .X}}", 0, false}, // action ends the input
		{"{{if .X -}} a", 0, false},
		{"{{if .X", 0, false},
		{"{{.X}} a", 0, false},
		{"{{else}} a", 0, false},
		{"{{end}} a", 0, false},
		{"{{/* c */}} a", 0, false},
	}
	for _, tt := range tests {
		l := prescanner(tt.input, true)
		if got := l.htmlActionOpensAttrs(tt.i); got != tt.want {
			t.Errorf("htmlActionOpensAttrs(%q, %d) = %t, want %t", tt.input, tt.i, got, tt.want)
		}
	}
}

func TestHTMLCommentAt(t *testing.T) {
	tests := []struct {
		input   string
		isTag   bool
		textEnd int
	}{
		{"<!---->", true, 7},
		{"<!-->", true, 0},
		{"<!--->", true, 0},
		{"<!-- c -->", true, 10},
		{"<!-- c", false, 4},
		{"<!-- {{.X}} -->", true, 15},
		{"<!-- {{- .X -}} -->", true, 19},
		{"<!-- {{/* c */}} -->", true, 20},
		{"<!-- {{if .X}}a{{end}} -->", true, 26},
		{"<!-- {{if .X}}{{else}}{{end}} -->", true, 33},
		{"<!-- {{if .X}} -->{{end}}", false, 18},
		{"<!-- {{end}} -->", false, 5},
		{"<!-- {{else}} -->", false, 5},
		{`<!-- {{define "x"}} -->`, false, 5},
		{"<!-- {{.X -->", true, 0},
		{"<!-- {{.X}}", false, 4}, // no "-->" at all
	}
	for _, tt := range tests {
		l := prescanner(tt.input, true)
		isTag, textEnd, name := l.htmlCommentAt(4)
		if isTag != tt.isTag || textEnd != tt.textEnd || name != "" {
			t.Errorf("htmlCommentAt(%q, 4) = (%t, %d, %q), want (%t, %d, %q)", tt.input, isTag, textEnd, name, tt.isTag, tt.textEnd, "")
		}
	}
}

func TestHTMLStartTagAt(t *testing.T) {
	tests := []struct {
		input       string
		emitComment bool
		isTag       bool
		textEnd     int
		name        string
	}{
		{"<div>", true, true, 0, ""},
		{"<br/>", true, true, 0, ""},
		{"<br />", true, true, 0, ""},
		{"<a b=c>", true, true, 0, ""},
		{"<a b='c' d=\"e\" f>", true, true, 0, ""},
		{`<a b="{{.X}}">`, true, true, 0, ""},
		{"<a b=c{{.X}}>", true, true, 0, ""},
		{"<a b={{.X}}>", true, true, 0, ""},
		{"<a {{.X}}>", true, true, 0, ""},
		{"<a b {{.X}}>", true, true, 0, ""},
		{"<li{{if .X}} hidden{{end}}>", true, true, 0, ""},
		{"<a b{{if .X}} c{{end}}>", true, true, 0, ""},
		{"<a {{if .X}} b{{end}}>", true, true, 0, ""},
		{"<a {{if .X}}b{{else}}c{{end}}>", true, true, 0, ""},
		{"<a {{.X", true, true, 0, ""}, // the action lexer reports it
		{`<a b="{{.X">`, true, true, 0, ""},
		{"<a b=c{{.X>", true, true, 0, ""},
		{"<a {{/* c */}}>", false, true, 0, ""},
		{"<a {{/* c */}}>", true, false, 15, "a"},

		{"<a", true, false, 2, "a"},
		{"<a ", true, false, 3, "a"},
		{"<a b=", true, false, 5, "a"},
		{"<a b='c", true, false, 7, "a"},
		{"<a b=>", true, false, 6, "a"},
		{"<a =b>", true, false, 6, "a"},
		{"<a / b>", true, false, 7, "a"},
		{"<h{{.N}}>", true, false, 9, "h"},
		{"<li{{if .X}}hidden{{end}}>", true, false, 26, "li"},
		{"<a {{.X}}=v>", true, false, 12, "a"},
		{"<a {{end}}>", true, false, 3, "a"},
		{"<a {{else}}>", true, false, 3, "a"},
		{`<a {{define "x"}}>`, true, false, 3, "a"},
		{"<a {{if .X}}>", true, false, 13, "a"},
		{"<a b{{.X}}>", true, false, 11, "a"},
		{"<a b{{end}}>", true, false, 4, "a"},
		{"<a b=c{{end}}>", true, false, 6, "a"},
		{`<a b="{{end}}">`, true, false, 6, "a"},
		{`<a b="xx{{end}}">`, true, false, 8, "a"},
		{`<a b=">" c="yyyyyy{{end}}">`, true, false, 18, "a"},
		{`<a b="xx{{.X}}yy">`, true, true, 0, ""},
		{`<a b="xx{{.X">`, true, true, 0, ""},
		{`<a b="{{if .X}}c">`, true, false, 18, "a"},
		{"<a b= {{end}}>", true, false, 6, "a"},
	}
	for _, tt := range tests {
		l := prescanner(tt.input, tt.emitComment)
		isTag, textEnd, name := l.htmlStartTagAt(0)
		if isTag != tt.isTag || textEnd != tt.textEnd || name != tt.name {
			t.Errorf("htmlStartTagAt(%q, 0) [EmitComment=%t] = (%t, %d, %q), want (%t, %d, %q)",
				tt.input, tt.emitComment, isTag, textEnd, name, tt.isTag, tt.textEnd, tt.name)
		}
	}
}

func TestHTMLDegradedTagEnd(t *testing.T) {
	tests := []struct {
		input string
		j     int
		depth int
		want  int
	}{
		{"a>b", 0, 0, 2},
		{"abc", 0, 0, 3},
		{"<a>", 2, 0, 3},
		{"a=b>", 0, 0, 4},
		{`a="x>y">`, 0, 0, 8},
		{`a='x>'>`, 0, 0, 7},
		{`a "x>y">`, 0, 0, 5},        // a quote not after = is no quote
		{`a= "x>">`, 0, 0, 8},        // space keeps afterEquals
		{`a="b"c">`, 0, 0, 8},        // closed quote, then c clears afterEquals
		{`a= {{.X}}"x>">`, 0, 0, 14}, // an action keeps afterEquals
		{"a{{end}}>", 0, 0, 1},
		{"a{{end}}>", 0, 1, 9},
		{"a{{else}}>", 0, 0, 1},
		{"a{{else}}>", 0, 1, 10},
		{`a{{define "x"}}>`, 0, 0, 1},
		{`a{{define "x"}}>`, 0, 1, 1},
		{"a{{if .X}}>", 0, 0, 11},
		{"a{{if .X}}{{end}}{{end}}>", 0, 0, 17},
		{"a{{.X}}>", 0, 0, 8},
		{"a{{.X", 0, 0, 1},
		{"a{{/* c */}}>", 0, 0, 13},
	}
	for _, tt := range tests {
		l := prescanner(tt.input, true)
		if got := l.htmlDegradedTagEnd(tt.j, tt.depth); got != tt.want {
			t.Errorf("htmlDegradedTagEnd(%q, %d, %d) = %d, want %d", tt.input, tt.j, tt.depth, got, tt.want)
		}
	}
}

func TestNextHTMLTag(t *testing.T) {
	tests := []struct {
		input       string
		pos         int
		textEnd     int // value of htmlTextEnd before the call
		limit       int
		want        int
		wantName    string
		wantTextEnd int
	}{
		{"<div>", 0, 0, 0, -1, "", 0}, // limit 0: nothing before the delimiter
		{"<div>", 0, 0, -1, 0, "", 0},
		{"a<div>", 0, 0, 1, -1, "", 0},
		{"a<div>", 0, 0, 2, 1, "", 0},
		{"<a =b> <c>", 0, 0, -1, 7, "a", 6},
		{"xx<a =b> <c>", 2, 0, -1, 7, "a", 8},
		{"ab<c><d>", 2, 5, -1, 3, "", 5},       // resumes from htmlTextEnd
		{"<a =b>x<c =d>", 0, 0, -1, 7, "a", 6}, // a second degraded tag stops the scan at its '<'
		{"<a =b>x", 0, 0, -1, -1, "a", 6},
	}
	for _, tt := range tests {
		l := prescanner(tt.input, true)
		l.pos = Pos(tt.pos)
		l.htmlTextEnd = Pos(tt.textEnd)
		got, name := l.nextHTMLTag(tt.input[tt.pos:], tt.limit)
		if got != tt.want || name != tt.wantName {
			t.Errorf("nextHTMLTag(%q [pos %d, htmlTextEnd %d], %d) = (%d, %q), want (%d, %q)",
				tt.input, tt.pos, tt.textEnd, tt.limit, got, name, tt.want, tt.wantName)
		}
		if tt.wantTextEnd != 0 && int(l.htmlTextEnd) != tt.wantTextEnd {
			t.Errorf("nextHTMLTag(%q [pos %d]) left htmlTextEnd = %d, want %d", tt.input, tt.pos, l.htmlTextEnd, tt.wantTextEnd)
		}
	}
}

func TestHTMLTagAt(t *testing.T) {
	tests := []struct {
		input   string
		i       int
		isTag   bool
		textEnd int
		name    string
	}{
		{"<", 0, false, 1, ""},
		{"< a", 0, false, 1, ""},
		{"<1>", 0, false, 1, ""},
		{"<é>", 0, false, 1, ""},
		{"a <b>", 2, true, 0, ""},
		{"<a", 0, false, 2, "a"},

		{"<!-- c -->", 0, true, 10, ""},
		{"<!--", 0, false, 4, ""},
		{"<!doctype html>", 0, true, 0, ""},
		{"<!doctype {{.X}}>", 0, false, 2, ""},
		{"<!doctype", 0, false, 2, ""},
		{"<?xml?>", 0, true, 0, ""},
		{"<?xml", 0, false, 2, ""},
		{"<?xml {{.X}}?>", 0, false, 2, ""},

		{"</div>", 0, true, 0, ""},
		{"x</y>", 1, true, 0, ""},
		{"</div >", 0, true, 0, ""},
		{"</div x>", 0, false, 8, ""},
		{"</div", 0, false, 5, ""},
		{"</", 0, false, 2, ""},
		{"</1>", 0, false, 2, ""},
		{"</ div>", 0, false, 2, ""},
		{"</{{.T}}>", 0, false, 2, ""},
		{"</d{{.T}}>", 0, false, 10, ""},
		{"</d{{end}}>", 0, false, 3, ""},
	}
	for _, tt := range tests {
		l := prescanner(tt.input, true)
		isTag, textEnd, name := l.htmlTagAt(tt.i)
		if isTag != tt.isTag || textEnd != tt.textEnd || name != tt.name {
			t.Errorf("htmlTagAt(%q, %d) = (%t, %d, %q), want (%t, %d, %q)",
				tt.input, tt.i, isTag, textEnd, name, tt.isTag, tt.textEnd, tt.name)
		}
	}
}
