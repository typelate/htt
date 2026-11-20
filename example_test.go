package htt_test

import (
	"fmt"
	"io"
	"os"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

func ExampleParseTree() {
	const page = `<ul>{{range .Items}}<li class="item">{{.}}</li>{{end}}</ul>`
	tree, err := htt.ParseTree("page.gohtml", "page", page, nil, lex.NewOptions())
	if err != nil {
		fmt.Println(err)
		return
	}
	htt.Walk(tree, func(t *htt.Tree, n htt.Node) {
		switch n := n.(type) {
		case *htt.HTMLElementNode:
			fmt.Printf("%s <%s>\n", t.Position(n), n.TagName)
		case *htt.RangeNode:
			fmt.Printf("%s range %s\n", t.Position(n), n.Pipe.Commands[0])
		case *htt.ActionNode:
			fmt.Printf("%s %s\n", t.Position(n), n)
		}
	})
	// Output:
	// page.gohtml:1:1 <ul>
	// page.gohtml:1:5 range .Items
	// page.gohtml:1:21 <li>
	// page.gohtml:1:38 {{.}}
}

func ExampleTree_diagnostics() {
	tree, err := htt.ParseTree("page.gohtml", "page", `<p>one</b> two</p>`, nil, lex.NewOptions())
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, d := range tree.Diagnostics {
		fmt.Println(tree.PositionAt(d.Pos), d.Code, d.Message)
	}
	// Output:
	// page.gohtml:1:7 orphan-end-tag unmatched </b> is ignored by browsers
}

func ExampleTree_unescapedAmpersand() {
	tree, err := htt.ParseTree("page.gohtml", "page", `<a href="/search?x=1&{{.Key}}={{.Value}}">go</a>`, nil, lex.NewOptions())
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, d := range tree.Diagnostics {
		fmt.Println(tree.PositionAt(d.Pos), d.Code, d.Message)
	}
	// Output:
	// page.gohtml:1:21 unescaped-ampersand & before an action starts a character reference its value can complete; write &amp;
}

func ExampleExecuteTemplate() {
	tree, err := htt.ParseTree("page.gohtml", "page", `<a href="{{.URL}}">{{.Title}}</a>`, nil, lex.NewOptions())
	if err != nil {
		fmt.Println(err)
		return
	}
	htt.AnnotateHTMLContexts(tree.Root)
	data := struct{ URL, Title string }{URL: "javascript:alert(1)", Title: "<Home>"}
	if err := htt.ExecuteTemplate(os.Stdout, tree, data); err != nil {
		fmt.Println(err)
	}
	// Output:
	// <a href="#ZgotmplZ">&lt;Home&gt;</a>
}

func ExampleCoverProfile() {
	tree, err := htt.ParseTree("page.gohtml", "page", `{{if .OK}}<p>yes</p>{{else}}<p>no</p>{{end}}`, nil, lex.NewOptions())
	if err != nil {
		fmt.Println(err)
		return
	}
	profile := new(htt.CoverProfile)
	data := struct{ OK bool }{OK: true}
	if err := htt.ExecuteTemplate(io.Discard, tree, data, htt.WithCoverProfile(profile)); err != nil {
		fmt.Println(err)
		return
	}
	if _, err := profile.WriteTo(os.Stdout); err != nil {
		fmt.Println(err)
	}
	// Output:
	// mode: set
	// page.gohtml:1.1,1.21 5 1
	// page.gohtml:1.21,1.38 5 0
	// page.gohtml:1.38,1.45 1 1
}
