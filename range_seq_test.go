package htt_test

import (
	"iter"
	"testing"
)

// singleValueSeq yields 0 through n-1.
func singleValueSeq(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for v := range n {
			if !yield(v) {
				return
			}
		}
	}
}

func TestRangeOverSingleValueIterator(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		data iter.Seq[int]
		html bool
		want string
	}{
		{
			name: "dot is each value",
			tmpl: `{{range .}}{{.}},{{end}}`,
			data: singleValueSeq(3),
			want: "0,1,2,",
		},
		{
			name: "one variable is each value",
			tmpl: `{{range $i := .}}{{$i}},{{end}}`,
			data: singleValueSeq(3),
			want: "0,1,2,",
		},
		{
			name: "empty iterator runs else",
			tmpl: `{{range .}}{{.}}{{else}}empty{{end}}`,
			data: singleValueSeq(0),
			want: "empty",
		},
		{
			name: "break stops after first value",
			tmpl: `{{range .}}{{.}}{{break}}{{end}}`,
			data: singleValueSeq(3),
			want: "0",
		},
		{
			name: "html dot is each value",
			tmpl: `{{range .}}<b>{{.}}</b>{{end}}`,
			data: singleValueSeq(2),
			html: true,
			want: "<b>0</b><b>1</b>",
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
