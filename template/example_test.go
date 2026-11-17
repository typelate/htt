package template_test

import (
	"fmt"
	"os"

	"github.com/typelate/htt/template"
)

func Example() {
	page := template.Must(template.New("page").Parse(`<p>{{.}}</p>`))
	if err := page.Execute(os.Stdout, "<b>"); err != nil {
		fmt.Println(err)
	}
	// Output:
	// <p>&lt;b&gt;</p>
}
