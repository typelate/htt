// Package template puts html/template's API over package htt, so that code
// written for html/template can run on trees that keep positions and count
// coverage.
//
// Its output must not be served: the engine's escaping is weaker than
// html/template's (see htt.ExecuteTemplate).
//
// The safe-content types are this package's own. Values of html/template's
// content types are trusted as well.
//
// When the environment variable named by CoverEnv holds a directory as the
// process starts, each Execute appends the blocks it reaches to a cover
// profile there.
//
// Parts of this package derive from the Go standard library; see
// NOTICE.txt.
package template

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"sort"
	"strings"
	texttemplate "text/template"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

// FuncMap is text/template's FuncMap, as it is in html/template.
type FuncMap = texttemplate.FuncMap

// Template is a parsed template and the set it belongs to. Once parsed it
// is safe for concurrent use.
type Template struct {
	// Tree is the parse tree, nil until the template is parsed.
	Tree *htt.Tree

	name   string
	common *common
}

type common struct {
	tmpl       map[string]*Template
	funcMap    htt.FuncMap
	missingKey htt.MissingKeyMode
	leftDelim  string
	rightDelim string

	// shadow is the same source as the trees of tmpl, parsed as text, by
	// template name. A template added with AddParseTree has none.
	shadow map[string]*htt.Tree
	// esc holds the plans made from shadow. Anything that changes the
	// templates replaces it.
	esc *escapeSet
}

// New returns an undefined template named name, in a set of its own.
func New(name string) *Template {
	t := &Template{
		name: name,
	}
	t.common = &common{
		tmpl:   map[string]*Template{name: t},
		shadow: make(map[string]*htt.Tree),
		esc:    new(escapeSet),
	}
	return t
}

// Must returns t, and panics if err is not nil.
func Must(t *Template, err error) *Template {
	if err != nil {
		panic(err)
	}
	return t
}

// Name returns the name of the template.
func (t *Template) Name() string {
	return t.name
}

// New returns an undefined template named name in t's set.
func (t *Template) New(name string) *Template {
	nt := &Template{
		name:   name,
		common: t.common,
	}
	t.common.tmpl[name] = nt
	return nt
}

// Lookup returns the template named name in t's set, or nil. A template
// made with New and not yet parsed has a nil Tree.
func (t *Template) Lookup(name string) *Template {
	return t.common.tmpl[name]
}

// Templates returns the templates in t's set, parsed or not.
func (t *Template) Templates() []*Template {
	result := make([]*Template, 0, len(t.common.tmpl))
	for _, tmpl := range t.common.tmpl {
		result = append(result, tmpl)
	}
	return result
}

// DefinedTemplates returns the names of the parsed templates after
// "; defined templates are: ", or "" when there are none. It is for error
// messages.
func (t *Template) DefinedTemplates() string {
	var names []string
	for _, tmpl := range t.common.tmpl {
		if tmpl.Tree != nil {
			names = append(names, tmpl.name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("; defined templates are: ")
	for i, name := range names {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%q", name)
	}
	return b.String()
}

// Funcs adds funcMap to the set's functions, replacing those of the same
// name. It returns t.
func (t *Template) Funcs(funcMap FuncMap) *Template {
	if t.common.funcMap == nil {
		t.common.funcMap = make(htt.FuncMap)
	}
	for k, v := range funcMap {
		t.common.funcMap[k] = v
	}
	t.common.esc = new(escapeSet)
	return t
}

// Delims sets the action delimiters for later parses in t's set. An empty
// string stands for the default, {{ or }}. It returns t.
func (t *Template) Delims(left, right string) *Template {
	t.common.leftDelim = left
	t.common.rightDelim = right
	return t
}

// Option sets options for t's set and panics on one it does not know. The
// only option is missingkey, which says what a map lookup with a missing
// key evaluates to:
//
//	"missingkey=default" or "missingkey=invalid"
//		No value, printed as "<no value>". The default.
//	"missingkey=zero"
//		The zero value of the map's element type.
//	"missingkey=error"
//		Execution stops with an error.
func (t *Template) Option(opt ...string) *Template {
	for _, o := range opt {
		switch {
		case strings.HasPrefix(o, "missingkey="):
			value := o[len("missingkey="):]
			switch value {
			case "default", "invalid":
				t.common.missingKey = htt.MissingKeyInvalid
			case "zero":
				t.common.missingKey = htt.MissingKeyZero
			case "error":
				t.common.missingKey = htt.MissingKeyError
			default:
				panic(fmt.Sprintf("template: unknown missing key option %q", o))
			}
		default:
			panic(fmt.Sprintf("template: unknown option %q", o))
		}
	}
	return t
}

// Parse parses text as the body of t. The templates text defines join t's
// set.
func (t *Template) Parse(text string) (*Template, error) {
	options := t.lexOptions()
	treeSet := make(map[string]*htt.Tree)
	tree, err := htt.ParseTree(t.name, t.name, text, treeSet, options)
	if err != nil {
		return nil, err
	}
	// ParseTree took the name for a file path; html/template records it
	// as given, with no file behind it.
	tree.ParseName, tree.Filepath = t.name, ""
	for _, tr := range treeSet {
		tr.ParseName, tr.Filepath = t.name, ""
	}
	replaced := make(map[string]bool)
	for name, tr := range htt.DefinedTrees(tree, treeSet) {
		tmpl, err := t.AddParseTree(name, tr)
		if err != nil {
			return nil, err
		}
		replaced[name] = tmpl.Tree == tr
	}
	// UNSUPPORTED: a text-mode parse keeps no node for a define, so it loses
	// the trim markers on one. Without a shadow, the old engine runs.
	if definesTrim(tree) {
		return t, nil
	}
	if err := t.parseShadow(text, replaced); err != nil {
		return nil, err
	}
	return t, nil
}

// definesTrim reports whether a {{define}} in tree has a trim marker.
func definesTrim(tree *htt.Tree) bool {
	found := false
	htt.Walk(tree, func(_ *htt.Tree, n htt.Node) {
		if d, ok := n.(*htt.DefineNode); ok {
			found = found || d.StartTrimLeft || d.StartTrimRight || d.EndTrimLeft || d.EndTrimRight
		}
	})
	return found
}

// parseShadow parses text as text, as Parse parsed it as HTML, and keeps
// the tree of each template in replaced that is true, since that template
// now has the HTML tree Parse made.
func (t *Template) parseShadow(text string, replaced map[string]bool) error {
	options := t.lexOptions()
	options.ParseHTML = false
	treeSet := make(map[string]*htt.Tree)
	tree, err := htt.ParseTree(t.name, t.name, text, treeSet, options)
	if err != nil {
		return err
	}
	tree.ParseName, tree.Filepath = t.name, ""
	for _, tr := range treeSet {
		tr.ParseName, tr.Filepath = t.name, ""
	}
	for name, tr := range htt.DefinedTrees(tree, treeSet) {
		if replaced[name] {
			t.common.shadow[name] = tr
		}
	}
	return nil
}

// AddParseTree adds tree to t's set under name and returns its template.
// An empty tree does not replace a definition.
func (t *Template) AddParseTree(name string, tree *htt.Tree) (*Template, error) {
	tree.HTMLMode = true
	htt.AnnotateHTMLContexts(tree.Root)
	tmpl := t.common.tmpl[name]
	if tmpl == nil {
		tmpl = &Template{
			name:   name,
			common: t.common,
		}
		t.common.tmpl[name] = tmpl
	}
	if tmpl.Tree == nil || !htt.IsEmptyTree(tree.Root) {
		tmpl.Tree = tree
		// There is no source to parse as text for this tree.
		delete(t.common.shadow, name)
	}
	t.common.esc = new(escapeSet)
	return tmpl, nil
}

// Clone returns a copy of t and its set. The trees are shared; templates
// parsed into the copy do not reach the original.
func (t *Template) Clone() (*Template, error) {
	nt := &Template{
		name: t.name,
	}
	nt.common = &common{
		tmpl:       make(map[string]*Template, len(t.common.tmpl)),
		missingKey: t.common.missingKey,
		leftDelim:  t.common.leftDelim,
		rightDelim: t.common.rightDelim,
		shadow:     maps.Clone(t.common.shadow),
		esc:        new(escapeSet),
	}
	if t.common.funcMap != nil {
		nt.common.funcMap = maps.Clone(t.common.funcMap)
	}
	for k, v := range t.common.tmpl {
		if k == t.name {
			nt.common.tmpl[k] = nt
			nt.Tree = v.Tree
		} else {
			nt.common.tmpl[k] = &Template{
				name:   v.name,
				Tree:   v.Tree,
				common: nt.common,
			}
		}
	}
	return nt, nil
}

// Execute applies t to data and writes the output to wr. The output must
// not be served; see htt.ExecuteTemplate.
func (t *Template) Execute(wr io.Writer, data any) error {
	return t.execute(wr, t.name, data)
}

// ExecuteTemplate applies the template named name in t's set to data and
// writes the output to wr. The output must not be served.
func (t *Template) ExecuteTemplate(wr io.Writer, name string, data any) error {
	tmpl := t.Lookup(name)
	if tmpl == nil || tmpl.Tree == nil {
		return fmt.Errorf("html/template: %q is an incomplete or empty template%s", name, t.DefinedTemplates())
	}
	return tmpl.execute(wr, name, data)
}

func (t *Template) execute(wr io.Writer, name string, data any) error {
	tmpl := t.common.tmpl[name]
	if tmpl == nil || tmpl.Tree == nil {
		return fmt.Errorf("html/template: %q is an incomplete or empty template%s", name, t.DefinedTemplates())
	}
	opts := t.execOptions()
	// UNSUPPORTED: coverage is counted on the HTML tree, so a run that
	// exports it uses the old engine.
	if processCover != nil {
		return processCover.execute(wr, tmpl.Tree, data, opts)
	}
	plan, err := t.common.esc.escapeTemplate(t.common.shadow, name)
	if err == nil {
		return htt.ExecuteTemplate(wr, t.common.shadow[name], data, t.shadowOptions(plan, name)...)
	}
	if !errors.Is(err, errUnsupported) {
		return err
	}
	return htt.ExecuteTemplate(wr, tmpl.Tree, data, opts...)
}

// ParseFiles parses the named files into t's set, each as a template named
// for its base name.
func (t *Template) ParseFiles(filenames ...string) (*Template, error) {
	return parseFiles(t, readFileOS, filenames...)
}

// ParseGlob is ParseFiles for the files that match pattern.
func (t *Template) ParseGlob(pattern string) (*Template, error) {
	return parseGlob(t, pattern)
}

// ParseFS is ParseFiles for the files in fsys that match patterns.
func (t *Template) ParseFS(fsys fs.FS, patterns ...string) (*Template, error) {
	return parseFS(t, fsys, patterns...)
}

func (t *Template) execOptions() []htt.Option {
	opts := []htt.Option{htt.WithTrust(trustOf)}
	if t.common.funcMap != nil {
		opts = append(opts, htt.WithFuncs(t.common.funcMap))
	}
	if t.common.missingKey != htt.MissingKeyInvalid {
		opts = append(opts, htt.WithMissingKey(t.common.missingKey))
	}
	finder := htt.FindTreeFunc(func(name string) (*htt.Tree, bool) {
		tmpl := t.common.tmpl[name]
		if tmpl != nil && tmpl.Tree != nil {
			return tmpl.Tree, true
		}
		return nil, false
	})
	opts = append(opts, htt.WithFinder(finder))
	return opts
}

// shadowOptions are the options to run the shadow tree of the template
// named name under plan. The plan writes what html/template writes, so
// nothing is trusted here.
func (t *Template) shadowOptions(plan *escapePlan, name string) []htt.Option {
	opts := []htt.Option{htt.WithPlan(plan, name)}
	if t.common.funcMap != nil {
		opts = append(opts, htt.WithFuncs(t.common.funcMap))
	}
	if t.common.missingKey != htt.MissingKeyInvalid {
		opts = append(opts, htt.WithMissingKey(t.common.missingKey))
	}
	finder := htt.FindTreeFunc(func(name string) (*htt.Tree, bool) {
		tree, ok := t.common.shadow[name]
		return tree, ok
	})
	return append(opts, htt.WithFinder(finder))
}

func (t *Template) lexOptions() lex.Options {
	var hasBreak, hasContinue bool
	if t.common.funcMap != nil {
		_, hasBreak = t.common.funcMap["break"]
		_, hasContinue = t.common.funcMap["continue"]
	}
	return lex.Options{
		EmitComment: true,
		ParseHTML:   true,
		LeftDelim:   cmp.Or(t.common.leftDelim, "{{"),
		RightDelim:  cmp.Or(t.common.rightDelim, "}}"),
		BreakOK:     !hasBreak,
		ContinueOK:  !hasContinue,
	}
}
