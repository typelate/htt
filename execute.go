package htt

import (
	"io"
	"reflect"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// FuncMap maps names to functions. Each function returns one value, or a
// value and an error.
type FuncMap map[string]any

// TreeFinder finds the tree a {{template}} call names.
type TreeFinder interface {
	FindTree(name string) (*Tree, bool)
}

// FindTreeFunc adapts a function to TreeFinder.
type FindTreeFunc func(name string) (*Tree, bool)

// FindTree calls f.
func (f FindTreeFunc) FindTree(name string) (*Tree, bool) {
	return f(name)
}

// Option configures an execution.
type Option func(*config)

type config struct {
	funcs      FuncMap
	finder     TreeFinder
	missingKey MissingKeyMode
	cp         *CoverProfile
	trust      TrustFunc
	plan       Plan
	instance   string
}

// MissingKeyMode says what a map lookup with a missing key evaluates to.
// A missing struct field is an error in every mode.
type MissingKeyMode int

const (
	// MissingKeyInvalid is the default: no value, printed as "<no value>".
	MissingKeyInvalid MissingKeyMode = iota
	// MissingKeyZero is the zero value of the map's element type.
	MissingKeyZero
	// MissingKeyError stops execution with an error.
	MissingKeyError
)

// WithFuncs sets the functions templates may call besides the builtins.
func WithFuncs(funcs FuncMap) Option {
	return func(c *config) {
		c.funcs = funcs
	}
}

// WithFinder sets how {{template}} calls find their trees. Without one,
// every call fails.
func WithFinder(finder TreeFinder) Option {
	return func(c *config) {
		c.finder = finder
	}
}

// WithMissingKey sets the MissingKeyMode.
func WithMissingKey(mode MissingKeyMode) Option {
	return func(c *config) {
		c.missingKey = mode
	}
}

// WithCoverProfile records in cp which nodes run.
func WithCoverProfile(cp *CoverProfile) Option {
	return func(c *config) {
		c.cp = cp
	}
}

// WithTrust sets the function that says which values skip contextual
// escaping in HTML mode.
func WithTrust(f TrustFunc) Option {
	return func(c *config) {
		c.trust = f
	}
}

// maxExecDepth is the maximum nesting of {{template}} calls.
const maxExecDepth = 100000

// ExecuteTemplate executes tree with data as dot and writes the output to
// w. A text tree is written as text/template writes it. A tree parsed in
// HTML mode is escaped by the contexts AnnotateHTMLContexts set on it, and
// as HTML text where it set none.
//
// Execution is for analysis, tests and coverage. Its output must not be
// served: the escaping falls short of html/template's, in these places
// among others.
//
//   - A style attribute's value is not filtered as CSS.
//   - A URL attribute's value is checked for its scheme but is not
//     percent-encoded.
//   - A text tree called from an HTML tree is not escaped for the place
//     it is called from.
func ExecuteTemplate(w io.Writer, tree *Tree, data any, opts ...Option) error {
	cfg := config{}
	for _, o := range opts {
		o(&cfg)
	}
	s := &state{
		wr:         w,
		markup:     new(markup),
		tree:       tree,
		cp:         cfg.cp,
		funcs:      cfg.funcs,
		finder:     cfg.finder,
		missingKey: cfg.missingKey,
		trust:      cfg.trust,
		plan:       cfg.plan,
		instance:   cfg.instance,
	}
	s.vars = []variable{{"$", dataValue(data)}}
	return s.execute(dataValue(data))
}

// ExecuteDocument executes the top-level tree of an HTML-mode parse and
// returns the document it builds, unrendered.
func ExecuteDocument(tree *Tree, data any, opts ...Option) (*html.Node, error) {
	cfg := config{}
	for _, o := range opts {
		o(&cfg)
	}
	s := &state{
		tree:       tree,
		cp:         cfg.cp,
		funcs:      cfg.funcs,
		finder:     cfg.finder,
		missingKey: cfg.missingKey,
		trust:      cfg.trust,
		plan:       cfg.plan,
		instance:   cfg.instance,
	}
	dot := dataValue(data)
	s.vars = []variable{{"$", dot}}
	root := tree.Root
	if root == nil {
		return &html.Node{Type: html.DocumentNode}, nil
	}
	if len(root.Nodes) == 1 {
		if frag, ok := root.Nodes[0].(*HTMLDocumentFragmentNode); ok {
			return s.buildDocument(dot, frag)
		}
	}
	return nil, errorf(tree, nil, "tree is not an HTML template")
}

// ExecuteElement executes one element of tree and returns its node.
func ExecuteElement(tree *Tree, elem *HTMLElementNode, data any, opts ...Option) (*html.Node, error) {
	cfg := config{}
	for _, o := range opts {
		o(&cfg)
	}
	s := &state{
		tree:       tree,
		cp:         cfg.cp,
		funcs:      cfg.funcs,
		finder:     cfg.finder,
		missingKey: cfg.missingKey,
		trust:      cfg.trust,
		plan:       cfg.plan,
		instance:   cfg.instance,
	}
	s.vars = []variable{{"$", dataValue(data)}}
	return s.walkHTMLElement(dataValue(data), elem)
}

// dataValue returns the value an execution starts from. A reflect.Value given
// as data is the value it wraps, as in text/template.
func dataValue(data any) reflect.Value {
	if v, ok := data.(reflect.Value); ok {
		return v
	}
	return reflect.ValueOf(data)
}

func htmlAtom(tagName string) atom.Atom {
	return atom.Lookup([]byte(tagName))
}
