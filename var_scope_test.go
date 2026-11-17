package htt_test

import "testing"

// A variable declared in a range, if or with body is gone once the body ends,
// so the next iteration and the code after the body see the earlier bindings.
func TestVariableScopeEndsWithItsBody(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		data any
		html bool
		want string
	}{
		{
			name: "declaration in range body",
			tmpl: "{{range $x := .}}<{{$foo := $x}}{{$x}}>{{end}}",
			data: []int{21, 22, 23},
			want: "<21><22><23>",
		},
		{
			name: "declaration in range body in html",
			tmpl: "<p>{{range $x := .}}[{{$foo := $x}}{{$x}}]{{end}}</p>",
			data: []int{21, 22, 23},
			html: true,
			want: "<p>[21][22][23]</p>",
		},
		{
			name: "declaration in if body shadows outer variable",
			tmpl: "{{$x := 1}}{{if true}}{{$x := 2}}{{$x}}{{end}}{{$x}}",
			data: nil,
			want: "21",
		},
		{
			name: "declaration in with body shadows outer variable",
			tmpl: "{{$v := 1}}{{with .}}{{$v := 2}}{{$v}}{{end}}{{$v}}",
			data: 3,
			want: "21",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got string
			if test.html {
				got = execHTML(t, test.name, test.tmpl, test.data)
			} else {
				got = execText(t, test.name, test.tmpl, test.data)
			}
			if got != test.want {
				t.Errorf("execute(%q) = %q, want %q", test.tmpl, got, test.want)
			}
		})
	}
}
