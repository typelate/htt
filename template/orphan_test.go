package template_test

import (
	"fmt"
	htmltemplate "html/template"
	"slices"
	"strings"
	"testing"

	"github.com/typelate/htt/template"
)

// orphanRow is one step taken from the state after New orphans t1. Each
// side builds that state itself, so a step cannot see another's changes.
type orphanRow struct {
	name string
	std  func() string
	htt  func() string
}

func orphanOutcome(err error, out string) string {
	return fmt.Sprintf("err=%v out=%q", err, out)
}

func stdOrphanSet() (*htmltemplate.Template, *htmltemplate.Template) {
	t1 := htmltemplate.Must(htmltemplate.New("foo").Parse("<p>one {{.}}</p>"))
	t2 := t1.New("foo")
	htmltemplate.Must(t2.Parse("<p>two {{.}}</p>"))
	return t1, t2
}

func httOrphanSet() (*template.Template, *template.Template) {
	t1 := template.Must(template.New("foo").Parse("<p>one {{.}}</p>"))
	t2 := t1.New("foo")
	template.Must(t2.Parse("<p>two {{.}}</p>"))
	return t1, t2
}

func stdExecuteOrphan(tm *htmltemplate.Template) string {
	var b strings.Builder
	err := tm.Execute(&b, "<b>")
	return orphanOutcome(err, b.String())
}

func httExecuteOrphan(tm *template.Template) string {
	var b strings.Builder
	err := tm.Execute(&b, "<b>")
	return orphanOutcome(err, b.String())
}

func stdLookupOrphan(t *htmltemplate.Template, name string) string {
	found := t.Lookup(name)
	if found == nil {
		return "nil"
	}
	return fmt.Sprintf("name=%q", found.Name())
}

func httLookupOrphan(t *template.Template, name string) string {
	found := t.Lookup(name)
	if found == nil {
		return "nil"
	}
	return fmt.Sprintf("name=%q", found.Name())
}

func stdNamesOrphan(t *htmltemplate.Template) string {
	var names []string
	for _, tm := range t.Templates() {
		names = append(names, tm.Name())
	}
	slices.Sort(names)
	return fmt.Sprintf("%q", names)
}

func httNamesOrphan(t *template.Template) string {
	var names []string
	for _, tm := range t.Templates() {
		names = append(names, tm.Name())
	}
	slices.Sort(names)
	return fmt.Sprintf("%q", names)
}

var orphanRows = []orphanRow{
	{
		name: "execute t2",
		std: func() string {
			_, t2 := stdOrphanSet()
			return stdExecuteOrphan(t2)
		},
		htt: func() string {
			_, t2 := httOrphanSet()
			return httExecuteOrphan(t2)
		},
	},
	{
		name: "execute t1",
		std: func() string {
			t1, _ := stdOrphanSet()
			return stdExecuteOrphan(t1)
		},
		htt: func() string {
			t1, _ := httOrphanSet()
			return httExecuteOrphan(t1)
		},
	},
	{
		name: "parse t1 then execute t1 and t2",
		std: func() string {
			t1, t2 := stdOrphanSet()
			_, err := t1.Parse("<p>three {{.}}</p>")
			return fmt.Sprintf("parse %v; t1 %s; t2 %s", err, stdExecuteOrphan(t1), stdExecuteOrphan(t2))
		},
		htt: func() string {
			t1, t2 := httOrphanSet()
			_, err := t1.Parse("<p>three {{.}}</p>")
			return fmt.Sprintf("parse %v; t1 %s; t2 %s", err, httExecuteOrphan(t1), httExecuteOrphan(t2))
		},
	},
	{
		name: "lookup foo from t1",
		std: func() string {
			t1, _ := stdOrphanSet()
			return stdLookupOrphan(t1, "foo")
		},
		htt: func() string {
			t1, _ := httOrphanSet()
			return httLookupOrphan(t1, "foo")
		},
	},
	{
		name: "templates of t1",
		std: func() string {
			t1, _ := stdOrphanSet()
			return stdNamesOrphan(t1)
		},
		htt: func() string {
			t1, _ := httOrphanSet()
			return httNamesOrphan(t1)
		},
	},
	{
		name: "templates of t2",
		std: func() string {
			_, t2 := stdOrphanSet()
			return stdNamesOrphan(t2)
		},
		htt: func() string {
			_, t2 := httOrphanSet()
			return httNamesOrphan(t2)
		},
	},
	{
		name: "new bar in t1 then lookup bar from t2",
		std: func() string {
			t1, t2 := stdOrphanSet()
			_, err := t1.New("bar").Parse("{{.}}")
			return fmt.Sprintf("parse %v; t2 bar %s", err, stdLookupOrphan(t2, "bar"))
		},
		htt: func() string {
			t1, t2 := httOrphanSet()
			_, err := t1.New("bar").Parse("{{.}}")
			return fmt.Sprintf("parse %v; t2 bar %s", err, httLookupOrphan(t2, "bar"))
		},
	},
	{
		name: "name of t1",
		std: func() string {
			t1, _ := stdOrphanSet()
			return t1.Name()
		},
		htt: func() string {
			t1, _ := httOrphanSet()
			return t1.Name()
		},
	},
}

// TestOrphanIsDetachedFromItsSet runs the same steps after New orphans a
// template, on html/template and on this package, and compares each result.
func TestOrphanIsDetachedFromItsSet(t *testing.T) {
	for _, row := range orphanRows {
		t.Run(row.name, func(t *testing.T) {
			want := row.std()
			got := row.htt()
			if got != want {
				t.Errorf("this package = %s\nhtml/template   = %s", got, want)
			}
		})
	}
}
