package htt_test

import (
	"fmt"
	"strings"
	"testing"
	texttemplate "text/template"
	"unsafe"

	"github.com/typelate/htt"
)

// valueKindsStd executes input with text/template, the oracle for each row.
func valueKindsStd(input string, data any) (string, error) {
	tmpl, err := texttemplate.New("case").Parse(input)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	err = tmpl.Execute(&b, data)
	return b.String(), err
}

// valueKindsHTT executes input with htt in text mode.
func valueKindsHTT(input string, data any) (string, error) {
	set, err := htt.NewText[any]("case").Parse(input)
	if err != nil {
		return "", err
	}
	tree, ok := set.FindTree("case")
	if !ok {
		return "", fmt.Errorf("template tree %q not found", "case")
	}
	var b strings.Builder
	err = htt.ExecuteTemplate(&b, tree, data, htt.WithFinder(set))
	return b.String(), err
}

// checkValueKinds runs each row through text/template, which must give the
// row's want, and then through htt, which must give the same.
func checkValueKinds(t *testing.T, data any, rows []struct{ name, input, want string }) {
	t.Helper()
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			std, err := valueKindsStd(tc.input, data)
			if err != nil {
				t.Fatalf("text/template %q: %v", tc.input, err)
			}
			if std != tc.want {
				t.Fatalf("text/template %q = %q, want %q", tc.input, std, tc.want)
			}
			got, err := valueKindsHTT(tc.input, data)
			if err != nil {
				t.Fatalf("htt %q: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("htt %q = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestInterfaceHeldValues(t *testing.T) {
	type holder struct {
		Anything any
		Ptr      *any
	}
	inner := any([]string{"a", "b"})
	data := holder{Anything: []int{7, 8}, Ptr: &inner}
	checkValueKinds(t, data, []struct{ name, input, want string }{
		{"len of interface field", "{{len .Anything}}", "2"},
		{"range over interface field", "{{range .Anything}}-{{.}}-{{end}}", "-7--8-"},
		{"index through pointer to interface", "{{index .Ptr 1}}", "b"},
		{"range through pointer to interface", "{{range .Ptr}}-{{.}}-{{end}}", "-a--b-"},
		{"slice of interface field", "{{slice .Anything 1}}", "[8]"},
	})
}

func TestSliceOfPointerToArray(t *testing.T) {
	type holder struct {
		PA *[3]int
	}
	data := holder{PA: &[3]int{3, 4, 5}}
	checkValueKinds(t, data, []struct{ name, input, want string }{
		{"whole array", "{{slice .PA}}", "[3 4 5]"},
		{"from second element", "{{slice .PA 1}}", "[4 5]"},
		{"second element only", "{{slice .PA 1 2}}", "[4]"},
	})
}

func TestUnsafePointerTruth(t *testing.T) {
	type holder struct {
		Set, Nil unsafe.Pointer
	}
	n := 23
	data := holder{Set: unsafe.Pointer(&n)}
	checkValueKinds(t, data, []struct{ name, input, want string }{
		{"non-nil unsafe pointer is true", "{{if .Set}}NON-ZERO{{else}}ZERO{{end}}", "NON-ZERO"},
		{"nil unsafe pointer is false", "{{if .Nil}}NON-ZERO{{else}}ZERO{{end}}", "ZERO"},
	})
}
