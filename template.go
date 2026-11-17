package htt

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/typelate/htt/lex"
)

// Template is a set of named trees parsed together, and one tree in it.
// Signature is the caller's representation of a function; the set only
// records which names are registered.
type Template[Signature any] struct {
	shared                *common[Signature] // nil until a method stores set state
	name                  string
	tree                  *Tree
	leftDelim, rightDelim string
	text                  bool // a text/template set: parsed without HTML structure
}

// common is the state that every template of a set shares, as
// text/template's common is.
type common[Signature any] struct {
	templates     map[string]*Template[Signature]
	functions     map[string]Signature
	missingKey    string // "invalid", "zero", "error", or "" (default)
	skipFuncCheck bool   // Parse does not check that called functions are defined
}

// state returns the set's shared state, allocating it for a zero Template.
func (t *Template[S]) state() *common[S] {
	if t.shared == nil {
		t.shared = &common[S]{
			templates: make(map[string]*Template[S]),
			functions: make(map[string]S),
		}
	}
	return t.shared
}

// read returns the set's shared state, or an empty state for a zero
// Template, without storing it.
func (t *Template[S]) read() common[S] {
	if t.shared == nil {
		return common[S]{}
	}
	return *t.shared
}

// New returns a set that parses in HTML mode, as html/template does.
func New[Signature any](name string) *Template[Signature] {
	t := &Template[Signature]{name: name}
	t.state().templates[name] = t
	return t
}

// NewText returns a set that parses as text/template does, with no HTML
// structure.
func NewText[Signature any](name string) *Template[Signature] {
	t := New[Signature](name)
	t.text = true
	return t
}

// SkipFuncCheck makes Parse accept calls of functions that are not
// registered. It returns t.
func (t *Template[S]) SkipFuncCheck() *Template[S] {
	t.state().skipFuncCheck = true
	return t
}

// IsText reports whether t came from NewText.
func (t *Template[S]) IsText() bool {
	return t.text
}

// ParseOptions returns the lexer options Parse uses.
func (t *Template[S]) ParseOptions() lex.Options {
	shared := t.read()
	_, hasBreak := shared.functions["break"]
	_, hasContinue := shared.functions["continue"]
	var functions map[string]bool
	if !shared.skipFuncCheck {
		functions = make(map[string]bool, len(shared.functions))
		for name := range shared.functions {
			functions[name] = true
		}
	}
	return lex.Options{
		Functions:   functions,
		EmitComment: true,
		ParseHTML:   !t.text,
		LeftDelim:   cmp.Or(t.leftDelim, "{{"),
		RightDelim:  cmp.Or(t.rightDelim, "}}"),
		BreakOK:     !hasBreak,
		ContinueOK:  !hasContinue,
	}
}

// Names returns the sorted names of the templates in the set.
func (t *Template[S]) Names() []string {
	templates := t.read().templates
	names := make([]string, 0, len(templates))
	for name := range templates {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Name returns the template's name.
func (t *Template[S]) Name() string {
	return t.name
}

// Tree returns the template's tree, which is nil until it is parsed.
func (t *Template[S]) Tree() *Tree {
	return t.tree
}

// Functions registers functions by name. It returns t.
func (t *Template[S]) Functions(functions map[string]S) *Template[S] {
	shared := t.state()
	for k, v := range functions {
		shared.functions[k] = v
	}
	return t
}

// Option sets options as text/template's Option does. The only option is
// missingkey.
func (t *Template[S]) Option(opt ...string) (*Template[S], error) {
	shared := t.state()
	for _, o := range opt {
		switch {
		case strings.HasPrefix(o, "missingkey="):
			value := o[len("missingkey="):]
			switch value {
			case "invalid", "default":
				shared.missingKey = "invalid"
			case "zero", "error":
				shared.missingKey = value
			default:
				return nil, fmt.Errorf("template: unknown missing key option %q", o)
			}
		default:
			return nil, fmt.Errorf("template: unknown option %q", o)
		}
	}
	return t, nil
}

// Delimiters sets the action delimiters for later calls to Parse. It
// returns t.
func (t *Template[S]) Delimiters(left, right string) *Template[S] {
	t.leftDelim = left
	t.rightDelim = right
	return t
}

// Lookup returns the template named name, or nil.
func (t *Template[S]) Lookup(name string) *Template[S] {
	return t.read().templates[name]
}

// FindTree implements TreeFinder.
func (t *Template[S]) FindTree(name string) (*Tree, bool) {
	if tmpl := t.Lookup(name); tmpl != nil && tmpl.tree != nil {
		return tmpl.tree, true
	}
	return nil, false
}

// Parse parses text as the body of t and adds the templates it defines to
// the set.
func (t *Template[S]) Parse(text string) (*Template[S], error) {
	options := t.ParseOptions()
	treeSet := make(map[string]*Tree)
	tree, err := ParseTree(t.name, t.name, text, treeSet, options)
	if err != nil {
		return nil, err
	}
	// ParseTree took the name for a file path; text/template records it
	// as given, with no file behind it.
	tree.ParseName, tree.Filepath = t.name, ""
	for _, tr := range treeSet {
		tr.ParseName, tr.Filepath = t.name, ""
	}
	for name, tr := range DefinedTrees(tree, treeSet) {
		if _, err := t.AddParseTree(name, tr); err != nil {
			return nil, err
		}
	}
	return t, nil
}

// AddParseTree adds tree to the set under name and returns its template.
// An empty tree does not replace one already there.
func (t *Template[S]) AddParseTree(name string, tree *Tree) (*Template[S], error) {
	if !t.text {
		tree.HTMLMode = true
		AnnotateHTMLContexts(tree.Root)
	}
	shared := t.state()
	tmpl := shared.templates[name]
	if tmpl == nil {
		tmpl = &Template[S]{
			name:       name,
			shared:     shared,
			leftDelim:  t.leftDelim,
			rightDelim: t.rightDelim,
			text:       t.text,
		}
		shared.templates[name] = tmpl
	}
	if tmpl.tree == nil || !IsEmptyTree(tree.Root) {
		tmpl.tree = tree
	}
	return tmpl, nil
}
