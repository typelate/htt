# htt

htt parses Go templates into trees that keep source positions and HTML structure, and executes those trees. It accepts the templates `text/template` and `html/template` accept.

It was factored out of [muxt](https://github.com/typelate/muxt), which uses it to type check templates, format them, and record which template nodes a package's tests reach.

## Status: do not serve its output yet

Render pages for a browser with the standard library's `html/template`. htt runs templates so that tests and tools can see what a template produces and which parts of it ran.

`htt/template` escapes with a copy of `html/template`'s escaper. The header of [`template/esc_escape.go`](template/esc_escape.go) names the Go release it was copied from. It has that release's escaping fixes whatever Go builds your program, and it does not get a later Go's fixes until the copy is updated. On the standard library's own escaping and error test tables its output and its errors are that `html/template`'s, and a fuzz target compares the two on generated templates. Nothing more has checked it, so treat its output as test data. The aim is a `htt/template` you can serve from.

The root package's [`ExecuteTemplate`](https://pkg.go.dev/github.com/typelate/htt#ExecuteTemplate) is a different engine when its tree has HTML structure. It escapes less than `html/template` does, and its documentation lists the known differences, for example:

- An action in attribute-name position, such as `<div {{.Name}}="x">`, is written as escaped text instead of a tag with a filtered attribute name.
- A text template called from an HTML template runs without the caller's escaping context.
- The value of a `style` or `srcdoc` attribute is not escaped for the CSS or HTML document it holds.

The API carries no compatibility promise; any release may change it.

## Packages

| Import path | Provides |
|---|---|
| `github.com/typelate/htt` | The parser, the tree (actions, HTML elements, attributes and comments as nodes with positions), and the execution engine. |
| `github.com/typelate/htt/template` | The API of `html/template` over that engine. |
| `github.com/typelate/htt/lex` | The lexer both use. |

```bash
go get github.com/typelate/htt
```

## Execute a template

`htt/template` has the names `html/template` has:

```go
package main

import (
	"os"

	"github.com/typelate/htt/template"
)

func main() {
	t := template.Must(template.New("greeting.gohtml").Parse(`<p>Hello, {{.}}!</p>` + "\n"))
	if err := t.Execute(os.Stdout, "<World>"); err != nil {
		panic(err)
	}
}
```

```text
<p>Hello, &lt;World&gt;!</p>
```

## Read a template's structure

`ParseTree` returns a tree in which HTML elements are nodes beside the actions, each with its position in the source:

```go
package main

import (
	"fmt"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

func main() {
	const src = "<ul>\n{{range .}}<li>{{.}}</li>{{end}}\n</ul>\n"
	tree, err := htt.ParseTree("list.gohtml", "list.gohtml", src, map[string]*htt.Tree{}, lex.NewOptions())
	if err != nil {
		panic(err)
	}
	htt.Walk(tree, func(t *htt.Tree, n htt.Node) {
		if el, ok := n.(*htt.HTMLElementNode); ok {
			p := t.Position(n)
			fmt.Printf("%s:%d:%d <%s>\n", p.ParseName, p.Line, p.Column, el.TagName)
		}
	})
}
```

```text
list.gohtml:1:1 <ul>
list.gohtml:2:12 <li>
```

## Template coverage

When the environment variable `MUXT_TEMPLATE_COVER` names a directory as the process starts, each execution through `htt/template` appends the template blocks it reaches to `<dir>/<pid>.cov`, in the format of a Go cover profile. `muxt test-template-mutations` sets it for its coverage pre-pass. Unset, nothing is written.

## License

MIT; see [LICENSE.md](LICENSE.md). Parts derive from the Go standard library's `text/template` and `html/template`, which carry Go's BSD-style license.
