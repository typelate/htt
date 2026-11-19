// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Copied from html/template/escape_test.go of go1.27.3, with the changes marked below.

package template_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/typelate/htt/template"
)

func TestIssue81821(t *testing.T) {
	// Changed: package-qualified; this package's API is template.New and template.Must.
	tmpl := template.Must(template.New("test").Parse("<script>const s = `${1}${/{{.}}/g}`</script>"))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, `x/.exec(alert(1))}`); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := "<script>const s = `${1}${/x\\/\\.exec\\(alert\\(1\\)\\)\\}/g}`</script>"
	if got := buf.String(); got != want {
		t.Errorf("got:  %s\nwant: %s", got, want)
	}
}

func TestIssue81823(t *testing.T) {
	tests := []struct {
		name  string
		tmpl  string
		input string
		want  string
	}{
		{
			name:  "yield",
			tmpl:  `<script>function* rules(){yield /{{.}}/g}</script>`,
			input: `/;alert(1)//`,
			want:  `<script>function* rules(){yield /\/;alert\(1\)\/\//g}</script>`,
		},
		{
			name:  "property yield",
			tmpl:  "<script>const pct = bond.yield / 100;\nconst user = {{.}}; // state\n</script>",
			input: `1;pwned=1;0`,
			want:  "<script>const pct = bond.yield / 100;\nconst user = \"1;pwned=1;0\"; \n</script>",
		},
		{
			name:  "private yield",
			tmpl:  "<script>class Bond { #yield = 4; pct() { return this.#yield / 100; } }\nconst user = {{.}}; // state\n</script>",
			input: `1;pwned=1;0`,
			want:  "<script>class Bond { #yield = 4; pct() { return this.#yield / 100; } }\nconst user = \"1;pwned=1;0\"; \n</script>",
		},
		{
			name:  "property in",
			tmpl:  "<script>const kb = traffic.in / 1024;\nconst user = {{.}}; // state\n</script>",
			input: `1;pwned=1;0`,
			want:  "<script>const kb = traffic.in / 1024;\nconst user = \"1;pwned=1;0\"; \n</script>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Changed: package-qualified constructor; strings.Builder kept as in the original.
			tmpl := template.Must(template.New("test").Parse(tt.tmpl))
			var buf strings.Builder
			if err := tmpl.Execute(&buf, tt.input); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if got := buf.String(); got != tt.want {
				t.Errorf("got:  %s\nwant: %s", got, tt.want)
			}
		})
	}
}
