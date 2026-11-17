package htt_test

import (
	"iter"
	"testing"
)

// pairSeq yields a=1, b=2, c=3.
func pairSeq(pairs ...string) iter.Seq2[string, int] {
	return func(yield func(string, int) bool) {
		for i, k := range pairs {
			if !yield(k, i+1) {
				return
			}
		}
	}
}

func TestRangeOverPairIterator(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		data iter.Seq2[string, int]
		html bool
		want string
	}{
		{
			name: "two variables are key and value",
			tmpl: `{{range $k, $v := .}}{{$k}}={{$v}},{{end}}`,
			data: pairSeq("a", "b", "c"),
			want: "a=1,b=2,c=3,",
		},
		{
			name: "one variable is the key",
			tmpl: `{{range $k := .}}{{$k}},{{end}}`,
			data: pairSeq("a", "b", "c"),
			want: "a,b,c,",
		},
		{
			name: "no variable makes dot the key",
			tmpl: `{{range .}}{{.}},{{end}}`,
			data: pairSeq("a", "b", "c"),
			want: "a,b,c,",
		},
		{
			name: "empty iterator runs else",
			tmpl: `{{range $k, $v := .}}x{{else}}empty{{end}}`,
			data: pairSeq(),
			want: "empty",
		},
		{
			name: "break stops after first pair",
			tmpl: `{{range $k, $v := .}}{{$k}}{{break}}{{end}}`,
			data: pairSeq("a", "b", "c"),
			want: "a",
		},
		{
			name: "html two variables are key and value",
			tmpl: `{{range $k, $v := .}}<b>{{$k}}={{$v}}</b>{{end}}`,
			data: pairSeq("a", "b", "c"),
			html: true,
			want: "<b>a=1</b><b>b=2</b><b>c=3</b>",
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
