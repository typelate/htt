package template_test

import (
	htmltemplate "html/template"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/htt/template"
)

// sameNameFiles holds two files with one base name, each defining a
// template of its own, and a third with another name.
var sameNameFiles = fstest.MapFS{
	"index.gohtml":     {Data: []byte(`top {{upper "x"}}{{define "a"}}{{if .}}a{{end}}{{end}}`)},
	"sub/index.gohtml": {Data: []byte(`sub {{upper "y"}}{{define "b"}}{{if .}}b{{end}}{{end}}`)},
	"other.gohtml":     {Data: []byte(`other{{define "c"}}c{{end}}`)},
}

func upper(s string) string { return strings.ToUpper(s) }

// A file with the name of the template it is parsed into becomes that
// template's contents, and what an earlier file of that name defined stays
// in the set, as in html/template.
func TestFilesWithOneBaseNameMatchHTMLTemplate(t *testing.T) {
	for _, tt := range []struct {
		name string
		std  func(fsys fs.FS) (*htmltemplate.Template, error)
		htt  func(fsys fs.FS) (*template.Template, error)
	}{
		{
			name: "one call matching both files",
			std: func(fsys fs.FS) (*htmltemplate.Template, error) {
				return htmltemplate.New("index.gohtml").Funcs(htmltemplate.FuncMap{"upper": upper}).ParseFS(fsys, "*.gohtml", "sub/*.gohtml")
			},
			htt: func(fsys fs.FS) (*template.Template, error) {
				return template.New("index.gohtml").Funcs(template.FuncMap{"upper": upper}).ParseFS(fsys, "*.gohtml", "sub/*.gohtml")
			},
		},
		{
			name: "a second call on the result of the first",
			std: func(fsys fs.FS) (*htmltemplate.Template, error) {
				first, err := htmltemplate.New("index.gohtml").Funcs(htmltemplate.FuncMap{"upper": upper}).ParseFS(fsys, "index.gohtml")
				if err != nil {
					return nil, err
				}
				return first.ParseFS(fsys, "sub/*.gohtml")
			},
			htt: func(fsys fs.FS) (*template.Template, error) {
				first, err := template.New("index.gohtml").Funcs(template.FuncMap{"upper": upper}).ParseFS(fsys, "index.gohtml")
				if err != nil {
					return nil, err
				}
				return first.ParseFS(fsys, "sub/*.gohtml")
			},
		},
		{
			name: "into a template of another name",
			std: func(fsys fs.FS) (*htmltemplate.Template, error) {
				return htmltemplate.New("root").Funcs(htmltemplate.FuncMap{"upper": upper}).ParseFS(fsys, "index.gohtml", "sub/index.gohtml", "other.gohtml")
			},
			htt: func(fsys fs.FS) (*template.Template, error) {
				return template.New("root").Funcs(template.FuncMap{"upper": upper}).ParseFS(fsys, "index.gohtml", "sub/index.gohtml", "other.gohtml")
			},
		},
		{
			name: "the file named as the template, then another",
			std: func(fsys fs.FS) (*htmltemplate.Template, error) {
				return htmltemplate.New("other.gohtml").Funcs(htmltemplate.FuncMap{"upper": upper}).ParseFS(fsys, "other.gohtml", "index.gohtml")
			},
			htt: func(fsys fs.FS) (*template.Template, error) {
				return template.New("other.gohtml").Funcs(template.FuncMap{"upper": upper}).ParseFS(fsys, "other.gohtml", "index.gohtml")
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			std, err := tt.std(sameNameFiles)
			require.NoError(t, err)
			got, err := tt.htt(sameNameFiles)
			require.NoError(t, err)

			var wantNames, gotNames []string
			for _, tmpl := range std.Templates() {
				wantNames = append(wantNames, tmpl.Name())
			}
			for _, tmpl := range got.Templates() {
				gotNames = append(gotNames, tmpl.Name())
			}
			slices.Sort(wantNames)
			slices.Sort(gotNames)
			assert.Equal(t, wantNames, gotNames, "the names in the set")
			assert.Equal(t, std.Name(), got.Name(), "the name of the result")

			for _, name := range append(wantNames, "missing") {
				var want, have strings.Builder
				wantErr := std.ExecuteTemplate(&want, name, true)
				gotErr := got.ExecuteTemplate(&have, name, true)
				if wantErr != nil {
					require.Error(t, gotErr, name)
					assert.Equal(t, wantErr.Error(), gotErr.Error(), name)
				} else {
					require.NoError(t, gotErr, name)
				}
				assert.Equal(t, want.String(), have.String(), name)
			}
		})
	}
}

// Each tree keeps the path of the file that made it, so two files with one
// base name can be told apart: the second file does not take over the
// trees of the first.
func TestFilesWithOneBaseNameKeepTheirPaths(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "index.gohtml")
	second := filepath.Join(dir, "sub", "index.gohtml")
	require.NoError(t, os.MkdirAll(filepath.Dir(second), 0o755))
	require.NoError(t, os.WriteFile(first, []byte(`top{{define "a"}}a{{end}}`), 0o644))
	// No text of its own: the first file's body stays the template's.
	require.NoError(t, os.WriteFile(second, []byte(`{{define "b"}}b{{end}}`), 0o644))

	tmpl, err := template.ParseFiles(first, second)
	require.NoError(t, err)

	assert.Equal(t, first, tmpl.Lookup("a").Tree.Filepath, `"a" is the first file's`)
	assert.Equal(t, second, tmpl.Lookup("b").Tree.Filepath, `"b" is the second file's`)
	assert.Equal(t, first, tmpl.Lookup("index.gohtml").Tree.Filepath, "the body is still the first file's")

	var out strings.Builder
	require.NoError(t, tmpl.Execute(&out, nil))
	assert.Equal(t, "top", out.String())
}
