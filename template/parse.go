package template

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

// ParseFiles parses the named files into a new set and returns the
// template of the first, named for its base name. There must be a file.
func ParseFiles(filenames ...string) (*Template, error) {
	return parseFiles(nil, readFileOS, filenames...)
}

// ParseGlob is ParseFiles for the files that match pattern, as
// filepath.Glob reads it. It must match a file.
func ParseGlob(pattern string) (*Template, error) {
	return parseGlob(nil, pattern)
}

// ParseFS is ParseFiles for the files in fsys that match patterns.
func ParseFS(fsys fs.FS, patterns ...string) (*Template, error) {
	return parseFS(nil, fsys, patterns...)
}

func parseFiles(t *Template, readFile func(string) (string, string, []byte, error), filenames ...string) (*Template, error) {
	if err := t.checkCanParse(); err != nil {
		return nil, err
	}
	if len(filenames) == 0 {
		return nil, fmt.Errorf("html/template: no files named in call to ParseFiles")
	}
	for _, filename := range filenames {
		name, fullPath, b, err := readFile(filename)
		if err != nil {
			return nil, err
		}
		s := string(b)
		// As in html/template: the first file names the result when there
		// is none, and a file with the result's name becomes its contents,
		// so that New(name).Funcs(f).ParseFiles(name) works. What the
		// result already defines stays in the set. Any other file is a new
		// template of the set.
		var tmpl *Template
		if t == nil {
			t = New(name)
		}
		if name == t.Name() {
			tmpl = t
		} else {
			tmpl = t.New(name)
		}
		_, err = tmpl.Parse(s)
		if err != nil {
			return nil, err
		}
		if fullPath != "" {
			// The trees this file made have its name and no path yet. A
			// tree an earlier file of the same name made keeps its own.
			for _, tr := range t.common.tmpl {
				if tr.Tree != nil && tr.Tree.Filepath == "" && tr.Tree.ParseName == name {
					tr.Tree.Filepath = fullPath
				}
			}
		}
	}
	return t, nil
}

func parseGlob(t *Template, pattern string) (*Template, error) {
	if err := t.checkCanParse(); err != nil {
		return nil, err
	}
	filenames, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	if len(filenames) == 0 {
		return nil, fmt.Errorf("html/template: pattern matches no files: %#q", pattern)
	}
	return parseFiles(t, readFileOS, filenames...)
}

func parseFS(t *Template, fsys fs.FS, patterns ...string) (*Template, error) {
	var filenames []string
	for _, pattern := range patterns {
		list, err := fs.Glob(fsys, pattern)
		if err != nil {
			return nil, err
		}
		if len(list) == 0 {
			return nil, fmt.Errorf("html/template: pattern matches no files: %#q", pattern)
		}
		filenames = append(filenames, list...)
	}
	return parseFiles(t, func(file string) (string, string, []byte, error) {
		name := path.Base(file)
		b, err := fs.ReadFile(fsys, file)
		return name, "", b, err
	}, filenames...)
}

func readFileOS(file string) (string, string, []byte, error) {
	name := filepath.Base(file)
	abs, err := filepath.Abs(file)
	if err != nil {
		return "", "", nil, err
	}
	b, err := os.ReadFile(file)
	return name, abs, b, err
}
