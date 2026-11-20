// Package template puts html/template's API over package htt, so that code
// written for html/template can run on trees that keep positions and count
// coverage.
//
// Execute escapes with a copy of html/template's escaper, run over a
// text-mode parse of the same source. The header of esc_escape.go names
// the Go release the copy came from: the output and the escaping errors
// are that html/template's, whatever Go builds the program, and a fix Go
// makes later reaches this package only when the copy is updated. The copy
// is checked against the standard library's escaping and error tables and
// by a differential fuzz, and nothing more: do not serve its output.
//
// The safe-content types are this package's own. Values of html/template's
// content types are recognized as well.
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
	"fmt"
	"io"
	"io/fs"
	"maps"
	"reflect"
	"sort"
	"strings"
	texttemplate "text/template"
	"unicode"

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
	// template name. A template added from a tree with HTML structure is nil:
	// it has no text form to escape.
	shadow map[string]*htt.Tree
	// bodies is the parse tree of each name in the set, as text/template keeps
	// its bodies in the set. A template made again with New takes its body
	// from here, so an empty Parse leaves the body in place.
	bodies map[string]*htt.Tree
	// esc holds the plans made from shadow. Anything that changes the
	// templates replaces it.
	esc *escapeSet
	// executed is set by the first execution, under esc's lock. From then on
	// the set refuses Parse, AddParseTree, ParseFiles, ParseGlob, ParseFS
	// and Clone, as html/template's does, because the plans in esc are made
	// from the trees as they were.
	executed bool
}

// New returns an undefined template named name, in a set of its own.
func New(name string) *Template {
	t := &Template{
		name: name,
	}
	t.common = &common{
		tmpl:   map[string]*Template{name: t},
		shadow: make(map[string]*htt.Tree),
		bodies: make(map[string]*htt.Tree),
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
	// As html/template does, a template already named name is reset to an
	// empty template in a set of its own: it is orphaned, executing it
	// fails, and parsing it leaves t's set alone. The set's body for name
	// stays. t may be the orphan itself, so the set is taken before any reset.
	set := t.common
	if existing := set.tmpl[name]; existing != nil {
		*existing = *New(existing.name)
	}
	nt := &Template{
		name:   name,
		common: set,
	}
	set.tmpl[name] = nt
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
// "; defined templates are: ", or "" when there are none. No execution error
// lists them, as html/template's do not.
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
		// As text/template's addValueFuncs does, a bad name, value or result
		// signature panics.
		if !goodFuncName(k) {
			panic(fmt.Errorf("function name %q is not a valid identifier", k))
		}
		if reflect.ValueOf(v).Kind() != reflect.Func {
			panic("value for " + k + " not a function")
		}
		if err := goodFunc(k, reflect.TypeOf(v)); err != nil {
			panic(err)
		}
		t.common.funcMap[k] = v
	}
	t.common.esc = new(escapeSet)
	return t
}

// goodFunc reports whether a function named name has a result shape that
// text/template can call: one result, or two with the second an error.
func goodFunc(name string, typ reflect.Type) error {
	switch numOut := typ.NumOut(); {
	case numOut == 1:
		return nil
	case numOut == 2 && typ.Out(1) == reflect.TypeFor[error]():
		return nil
	case numOut == 2:
		return fmt.Errorf("invalid function signature for %s: second return value should be error; is %s", name, typ.Out(1))
	default:
		return fmt.Errorf("function %s has %d return values; should be 1 or 2", name, typ.NumOut())
	}
}

// goodFuncName reports whether name is an identifier, as text/template
// requires of a function name: a letter or underscore, then letters,
// digits and underscores.
func goodFuncName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		switch {
		case r == '_':
		case i == 0 && !unicode.IsLetter(r):
			return false
		case !unicode.IsLetter(r) && !unicode.IsDigit(r):
			return false
		}
	}
	return true
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
	if err := t.checkCanParse(); err != nil {
		return nil, err
	}
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
		htt.AnnotateHTMLContexts(tr.Root)
		_, ok := t.addTree(name, tr)
		replaced[name] = ok
	}
	if err := t.parseShadow(text, replaced); err != nil {
		return nil, err
	}
	return t, nil
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
// A tree parsed without HTML structure is escaped as html/template escapes a
// parse tree added that way. A tree with HTML structure that is the tree of
// a template in t's set runs with that template's text form. Any other tree
// with HTML structure is kept for inspection and annotated in place;
// executing it is an error. An empty tree
// does not replace a definition.
func (t *Template) AddParseTree(name string, tree *htt.Tree) (*Template, error) {
	if err := t.checkCanParse(); err != nil {
		return nil, err
	}
	shadow := tree
	if tree.HTMLMode {
		htt.AnnotateHTMLContexts(tree.Root)
		shadow = t.shadowOf(tree)
	}
	tmpl, replaced := t.addTree(name, tree)
	if replaced {
		t.common.shadow[name] = shadow
	}
	t.common.esc = new(escapeSet)
	return tmpl, nil
}

// shadowOf returns the text tree of the template of t's set whose tree is
// tree, or nil when no such template has one. A tree with HTML structure
// from outside the set has no text form.
func (t *Template) shadowOf(tree *htt.Tree) *htt.Tree {
	for name, body := range t.common.bodies {
		if body == tree && t.common.shadow[name] != nil {
			return t.common.shadow[name]
		}
	}
	return nil
}

// addTree adds tree to t's set under name, without a shadow, and reports
// whether it replaced the template's tree. An empty tree does not replace a
// definition.
func (t *Template) addTree(name string, tree *htt.Tree) (*Template, bool) {
	tmpl := t.common.tmpl[name]
	if tmpl == nil {
		tmpl = &Template{
			name:   name,
			common: t.common,
		}
		t.common.tmpl[name] = tmpl
	}
	replaced := t.common.bodies[name] == nil || !htt.IsEmptyTree(tree.Root)
	if replaced {
		t.common.bodies[name] = tree
		// A nil shadow marks a template defined with no text form to escape.
		t.common.shadow[name] = nil
	}
	tmpl.Tree = t.common.bodies[name]
	return tmpl, replaced
}

// Clone returns a copy of t and its set. The trees are shared; templates
// parsed into the copy do not reach the original.
func (t *Template) Clone() (*Template, error) {
	if t.hasExecuted() {
		return nil, fmt.Errorf("html/template: cannot Clone %q after it has executed", t.name)
	}
	nt := &Template{
		name: t.name,
	}
	nt.common = &common{
		tmpl:       make(map[string]*Template, len(t.common.tmpl)),
		missingKey: t.common.missingKey,
		leftDelim:  t.common.leftDelim,
		rightDelim: t.common.rightDelim,
		shadow:     maps.Clone(t.common.shadow),
		bodies:     maps.Clone(t.common.bodies),
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
	t.markExecuted()
	if t.Tree == nil {
		return fmt.Errorf("template: %q is an incomplete or empty template", t.name)
	}
	return t.execute(wr, t.name, data)
}

// ExecuteTemplate applies the template named name in t's set to data and
// writes the output to wr. The output must not be served.
func (t *Template) ExecuteTemplate(wr io.Writer, name string, data any) error {
	t.markExecuted()
	tmpl := t.Lookup(name)
	if tmpl == nil {
		return fmt.Errorf("html/template: %q is undefined", name)
	}
	if tmpl.Tree == nil {
		return fmt.Errorf("html/template: %q is an incomplete template", name)
	}
	return tmpl.execute(wr, name, data)
}

// execute runs the template named name, which is parsed.
func (t *Template) execute(wr io.Writer, name string, data any) error {
	plan, err := t.common.esc.escapeTemplate(t.common.shadow, name)
	if err != nil {
		return err
	}
	text := t.common.shadow[name]
	opts := t.shadowOptions(plan, name)
	if processCover != nil {
		return processCover.execute(wr, text, data, opts, t.htmlTreeOf)
	}
	return htt.ExecuteTemplate(wr, text, data, opts...)
}

// markExecuted freezes t's set: it is called by every execution, whether or
// not the execution succeeds, as html/template's escape is.
func (t *Template) markExecuted() {
	t.common.esc.mu.Lock()
	defer t.common.esc.mu.Unlock()
	t.common.executed = true
}

// hasExecuted reports whether t's set has been executed.
func (t *Template) hasExecuted() bool {
	t.common.esc.mu.Lock()
	defer t.common.esc.mu.Unlock()
	return t.common.executed
}

// checkCanParse returns html/template's error for parsing into a set that
// has executed, and nil for a nil t, as html/template's does.
func (t *Template) checkCanParse() error {
	if t == nil {
		return nil
	}
	if t.hasExecuted() {
		return fmt.Errorf("html/template: cannot Parse after Execute")
	}
	return nil
}

// htmlTreeOf returns the tree with HTML structure that the shadow text tree
// text stands for, or nil when no template has text as its shadow.
func (t *Template) htmlTreeOf(text *htt.Tree) *htt.Tree {
	for name, shadow := range t.common.shadow {
		if shadow != text {
			continue
		}
		if tmpl := t.common.tmpl[name]; tmpl != nil {
			return tmpl.Tree
		}
	}
	return nil
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

// shadowOptions are the options to run the shadow tree of the template
// named name under plan. The plan writes what html/template writes, so
// nothing is trusted here.
func (t *Template) shadowOptions(plan *escapePlan, name string) []htt.Option {
	finder := htt.FindTreeFunc(func(name string) (*htt.Tree, bool) {
		tree := t.common.shadow[name]
		return tree, tree != nil
	})
	return t.runOptions([]htt.Option{htt.WithPlan(plan, name)}, finder)
}

// runOptions appends to first the options every run of t shares: its funcs,
// its missing-key policy, and finder, which names the trees a run may call.
func (t *Template) runOptions(first []htt.Option, finder htt.FindTreeFunc) []htt.Option {
	opts := first
	if t.common.funcMap != nil {
		opts = append(opts, htt.WithFuncs(t.common.funcMap))
	}
	if t.common.missingKey != htt.MissingKeyInvalid {
		opts = append(opts, htt.WithMissingKey(t.common.missingKey))
	}
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
