package htt

import (
	"bytes"
	"cmp"
	"fmt"
	"go/token"
	"path"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/typelate/htt/lex"
)

// A Mode value is a set of flags (or 0), as text/template/parse's Mode is.
// A tree's Mode records how it was parsed; the parser is told what to do
// with lex.Options, so changing a Mode changes nothing.
type Mode uint

const (
	ParseComments Mode = 1 << iota // comments were parsed and added to the tree
	SkipFuncCheck                  // functions were not checked to be defined
)

// Tree is the representation of a single parsed template.
type Tree struct {
	Name      string    // name of the template represented by the tree.
	ParseName string    // name of the top-level template during parsing, for error messages.
	Filepath  string    // absolute file path, if known.
	Root      *ListNode // top-level root of the tree.
	Mode      Mode      // parsing mode.
	// HTMLMode says the tree executes with contextual escaping, whatever
	// its root holds.
	HTMLMode bool
	// Diagnostics holds the structural findings recorded while parsing.
	Diagnostics Diagnostics
	// RootTrimFirst and RootTrimLast are the trim markers of the {{define}}
	// or {{block}} whose body is this tree's root: -}} on the opener and
	// {{- on the end.
	RootTrimFirst, RootTrimLast bool
	text                        string // text parsed to create the template (or its parent)
	leftDelim, rightDelim       string // action delimiters text was parsed with
	define                      bool   // registered by a define clause; DefinedTrees installs Extract's tree for it
	linesOnce                   sync.Once
	lines                       []int // offset of the start of each line of text; see lineStarts
	// Parsing only; cleared after parse.
	scanner         *lex.Scanner
	token           [3]lex.Token // three-token lookahead for parser.
	peekCount       int
	vars            []string // variables defined at the moment.
	treeSet         map[string]*Tree
	actionLine      int     // line of left delim starting action
	actionPos       lex.Pos // position of left delim starting action
	actionTrimLeft  bool    // left trim marker on current action's left delimiter
	actionTrimRight bool    // right trim marker on current action's right delimiter
	rangeDepth      int
	stackDepth      int             // depth of nested parenthesized expressions
	htmlAncestors   []string        // stack of open HTML element tag names (lowercase)
	htmlDegraded    []string        // names of start tags read as text whose end tag is text too (see lex.Token.DegradedTag)
	htmlDegradedMin int             // htmlDegraded entries below this index belong to elements enclosing the one being parsed
	functions       map[string]bool // lex.Options.Functions: the registered function names, or nil for no check
	maxHTMLDepth    int             // maximum nesting depth for HTML elements
	htmlDepth       int             // current HTML element nesting depth
}

// maxStackDepth is the maximum depth permitted for nested
// parenthesized expressions.
var maxStackDepth = 10000

// init reduces maxStackDepth for WebAssembly due to its smaller stack size.
func init() {
	if runtime.GOARCH == "wasm" {
		maxStackDepth = 1000
	}
}

// ParseTree parses text and returns the tree of the template named
// templateName. It adds that tree, and one for each {{define}} and
// {{block}} in text, to treeSet, which may be nil. fileName is a
// slash-separated path used in positions and error messages.
func ParseTree(fileName, templateName, text string, treeSet map[string]*Tree, options lex.Options) (*Tree, error) {
	if treeSet == nil {
		treeSet = make(map[string]*Tree)
	}
	t := &Tree{
		Name:      templateName,
		ParseName: path.Base(fileName),
		Filepath:  fileName,
	}
	t.text = text
	t.leftDelim, t.rightDelim = options.LeftDelim, options.RightDelim
	t.functions = options.Functions
	if options.EmitComment {
		t.Mode |= ParseComments
	}
	if options.Functions == nil {
		t.Mode |= SkipFuncCheck
	}
	t.maxHTMLDepth = options.MaxHTMLDepth
	if t.maxHTMLDepth == 0 {
		t.maxHTMLDepth = 1024
	}
	lexer := lex.New(text, options)
	t.startParse(lexer, treeSet)
	t.text = text
	if options.ParseHTML {
		t.HTMLMode = true
		if err := t.htmlParse(); err != nil {
			t.stopParse()
			return nil, err
		}
	} else {
		if err := t.parse(); err != nil {
			t.stopParse()
			return nil, err
		}
	}
	if err := t.add(); err != nil {
		t.stopParse()
		return nil, err
	}
	t.stopParse()
	t.setParents()
	return t, nil
}

func (t *Tree) next() lex.Token {
	if t.peekCount > 0 {
		t.peekCount--
	} else {
		t.token[0] = t.scanner.Next()
	}
	return t.token[t.peekCount]
}

func (t *Tree) backup() {
	t.peekCount++
}

func (t *Tree) backup2(t1 lex.Token) {
	t.token[1] = t1
	t.peekCount = 2
}

func (t *Tree) backup3(t2, t1 lex.Token) { // Reverse order: we're pushing back.
	t.token[1] = t1
	t.token[2] = t2
	t.peekCount = 3
}

func (t *Tree) peek() lex.Token {
	if t.peekCount > 0 {
		return t.token[t.peekCount-1]
	}
	t.peekCount = 1
	t.token[0] = t.scanner.Next()
	return t.token[0]
}

func (t *Tree) nextNonSpace() (token lex.Token) {
	for {
		token = t.next()
		if token.Type != lex.TokenTypeSpace {
			break
		}
	}
	return token
}

func (t *Tree) peekNonSpace() lex.Token {
	token := t.nextNonSpace()
	t.backup()
	return token
}

// Position is a 1-based line and column in a template's source.
type Position struct {
	ParseName    string
	Line, Column int
	Source       string // the text of the node, when the position is a node's
}

// String formats p as name:line:column.
func (p Position) String() string {
	return fmt.Sprintf("%s:%d:%d", p.ParseName, p.Line, p.Column)
}

// RootStart returns the offset at which text/template would start this
// tree's root list. The two differ after {{define ... -}} and
// {{block ... -}}: text/template drops the trimmed space while lexing, and
// this parser keeps it as text.
func (t *Tree) RootStart() lex.Pos {
	root := t.Root
	if root == nil {
		return 0
	}
	if !t.RootTrimFirst || len(root.Nodes) == 0 {
		return root.Start()
	}
	text, ok := root.Nodes[0].(*TextNode)
	if !ok {
		return root.Start()
	}
	if rest := bytes.TrimLeft(text.Text, trimCutset); len(rest) > 0 {
		return text.Start() + lex.Pos(len(text.Text)-len(rest))
	}
	if len(root.Nodes) > 1 {
		return root.Nodes[1].Start()
	}
	return root.End()
}

// Source returns the whole text the tree was parsed from, which may define
// other templates too.
func (t *Tree) Source() string { return t.text }

// Delims returns the action delimiters the tree was parsed with.
func (t *Tree) Delims() (left, right string) {
	return cmp.Or(t.leftDelim, "{{"), cmp.Or(t.rightDelim, "}}")
}

// PositionAt returns the line and column of a byte offset in the source.
// An offset outside the source counts as its nearer end.
func (t *Tree) PositionAt(pos lex.Pos) Position {
	p := min(max(int(pos), 0), len(t.text))
	starts := t.lineStarts()
	// p is on the last line that starts at or before it.
	line := sort.Search(len(starts), func(i int) bool { return starts[i] > p })
	return Position{
		ParseName: t.ParseName,
		Line:      line,
		Column:    p - starts[line-1] + 1,
	}
}

// lineStarts returns the offset at which each line of the source starts.
// The table is built on first use: coverage asks for two positions per
// block, and counting newlines from the start each time is quadratic.
func (t *Tree) lineStarts() []int {
	t.linesOnce.Do(func() {
		t.lines = []int{0}
		for i := 0; ; {
			n := strings.IndexByte(t.text[i:], '\n')
			if n < 0 {
				return
			}
			i += n + 1
			t.lines = append(t.lines, i)
		}
	})
	return t.lines
}

// Position returns the position of n's start, with n's source text.
func (t *Tree) Position(n Node) Position {
	tree := n.tree()
	if tree == nil {
		tree = t
	}
	p := tree.PositionAt(n.Start())
	p.ParseName = t.ParseName
	p.Source = n.String()
	return p
}

// ParseError is a parse failure with its place in the template text. Error
// keeps text/template's "template: name:line: message" form.
type ParseError struct {
	err                error  // the bare cause; Error prefixes it with the location
	name               string // ParseName, as it appears in the Error string
	file               string // file path when known, "" for an in-memory template
	line, column       int
	endLine, endColumn int
	start, end         lex.Pos
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("template: %s:%d: %s", e.name, e.line, e.err.Error())
}

// Unwrap returns the cause, whose message has no location prefix.
func (e *ParseError) Unwrap() error { return e.err }

// Position returns where the failing token starts.
func (e *ParseError) Position() token.Position {
	return token.Position{Filename: e.file, Offset: int(e.start), Line: e.line, Column: e.column}
}

// EndPosition returns the position just past the failing token.
func (e *ParseError) EndPosition() token.Position {
	return token.Position{Filename: e.file, Offset: int(e.end), Line: e.endLine, Column: e.endColumn}
}

// errorf formats a parse error, clears the root, and returns the error.
func (t *Tree) errorf(format string, args ...any) error {
	t.Root = nil
	tok := t.token[0]
	end := t.PositionAt(tok.End)
	return &ParseError{
		err:       fmt.Errorf(format, args...),
		name:      t.ParseName,
		file:      t.Filepath,
		line:      tok.Line,
		column:    t.PositionAt(tok.Pos).Column,
		endLine:   end.Line,
		endColumn: end.Column,
		start:     tok.Pos,
		end:       tok.End,
	}
}

func (t *Tree) error(err error) error {
	return t.errorf("%s", err)
}

func (t *Tree) expect(expected lex.TokenType, context string) (lex.Token, error) {
	token := t.nextNonSpace()
	if token.Type != expected {
		return token, t.unexpected(token, context)
	}
	return token, nil
}

func (t *Tree) expectOneOf(expected1, expected2 lex.TokenType, context string) (lex.Token, error) {
	token := t.nextNonSpace()
	if token.Type != expected1 && token.Type != expected2 {
		return token, t.unexpected(token, context)
	}
	return token, nil
}

func (t *Tree) unexpected(token lex.Token, context string) error {
	if token.Type == lex.TokenTypeError {
		extra := ""
		if t.actionLine != 0 && t.actionLine != token.Line {
			extra = fmt.Sprintf(" in action started at %s:%d", t.ParseName, t.actionLine)
			if strings.HasSuffix(token.Value, " action") {
				extra = extra[len(" in action"):] // avoid "action in action"
			}
		}
		return t.errorf("%s%s", token, extra)
	}
	return t.errorf("unexpected %s in %s", token, context)
}

// hasFunction reports whether a template may call name: a registered
// function or a builtin, or any name when none were registered.
func (t *Tree) hasFunction(name string) bool {
	if t.functions == nil || t.functions[name] {
		return true
	}
	_, ok := builtins()[name]
	return ok
}

func (t *Tree) startParse(scanner *lex.Scanner, treeSet map[string]*Tree) {
	t.Root = nil
	t.scanner = scanner
	t.vars = []string{"$"}
	t.treeSet = treeSet
	t.stackDepth = 0
}

func (t *Tree) stopParse() {
	t.scanner = nil
	t.vars = nil
	t.treeSet = nil
}

func (t *Tree) add() error {
	tree := t.treeSet[t.Name]
	if tree == nil || IsEmptyTree(tree.Root) {
		t.treeSet[t.Name] = t
		return nil
	}
	if !IsEmptyTree(t.Root) {
		return t.errorf("template: multiple definition of template %q", t.Name)
	}
	return nil
}

// addDefinition registers the tree of a define or block as text/template
// does: it replaces an empty tree of the same name, and two that are not
// empty are an error.
func (t *Tree) addDefinition(tree *Tree) error {
	existing := t.treeSet[tree.Name]
	if existing == nil || IsEmptyTree(existing.Root) {
		t.treeSet[tree.Name] = tree
		return nil
	}
	if !IsEmptyTree(tree.Root) {
		return t.errorf("template: multiple definition of template %q", tree.Name)
	}
	return nil
}

// IsEmptyTree reports whether n holds nothing but space and comments.
func IsEmptyTree(n Node) bool {
	switch n := n.(type) {
	case nil:
		return true
	case *ActionNode:
	case *CommentNode:
		return true
	case *IfNode:
	case *ListNode:
		for _, node := range n.Nodes {
			if !IsEmptyTree(node) {
				return false
			}
		}
		return true
	case *RangeNode:
	case *TemplateNode:
	case *TextNode:
		return len(bytes.TrimSpace(n.Text)) == 0
	case *WithNode:
	case *HTMLDocumentFragmentNode:
		for _, child := range n.Children {
			if !IsEmptyTree(child) {
				return false
			}
		}
		return true
	case *HTMLElementNode:
	case *HTMLAttributeNode:
	case *HTMLCommentNode:
		return true
	case *HTMLDoctypeNode:
	case *DefineNode:
		return true
	case *BlockNode:
		return IsEmptyTree(n.Body)
	case *BreakNode, *ContinueNode:
	default:
		panic("unknown node: " + n.String())
	}
	return false
}

// parse is the top-level parser for a template, essentially the same
// as itemList except it also parses {{define}} actions.
// It runs to EOF.
func (t *Tree) parse() error {
	t.Root = t.newList(0)
	// trimNext is set when the define before the next node has a right
	// trim marker on its end, which removes the space that follows it.
	trimNext := false
	for t.peek().Type != lex.TokenTypeEOF {
		if t.peek().Type == lex.TokenTypeLeftDelim {
			delim := t.next()
			if t.nextNonSpace().Type == lex.TokenTypeDefine {
				newT := &Tree{
					Name: "definition",
				}
				newT.text = t.text
				newT.leftDelim, newT.rightDelim = t.leftDelim, t.rightDelim
				newT.functions = t.functions
				newT.Mode = t.Mode
				newT.ParseName = t.ParseName
				newT.Filepath = t.Filepath
				newT.startParse(t.scanner, t.treeSet)
				endTrimRight, err := newT.parseDefinition()
				if err != nil {
					return err
				}
				if delim.TrimLeft {
					t.trimLastText()
				}
				trimNext = endTrimRight
				continue
			}
			t.backup2(delim)
		}
		n, err := t.textOrAction()
		if err != nil {
			return err
		}
		if text, ok := n.(*TextNode); ok && trimNext {
			text.Text = bytes.TrimLeft(text.Text, trimCutset)
		}
		trimNext = false
		switch n.(type) {
		case *endNode, *elseNode:
			return t.errorf("unexpected %s", n)
		default:
			t.Root.Nodes = append(t.Root.Nodes, n)
		}
	}
	t.Root.end = lex.Pos(len(t.text))
	return nil
}

// trimLastText removes the space at the end of the text node that ends
// t.Root, as a {{- marker on the define that follows it does.
func (t *Tree) trimLastText() {
	nodes := t.Root.Nodes
	if len(nodes) == 0 {
		return
	}
	if text, ok := nodes[len(nodes)-1].(*TextNode); ok {
		text.Text = bytes.TrimRight(text.Text, trimCutset)
	}
}

// parseDefinition parses a {{define}} ...  {{end}} template definition and
// installs the definition in t.treeSet. The "define" keyword has already
// been scanned. It returns the -}} marker on the definition's end, which
// trims the space after the define.
func (t *Tree) parseDefinition() (endTrimRight bool, err error) {
	const context = "define clause"
	name, err := t.expectOneOf(lex.TokenTypeString, lex.TokenTypeRawString, context)
	if err != nil {
		return false, err
	}
	t.Name, err = strconv.Unquote(name.Value)
	if err != nil {
		return false, t.error(err)
	}
	rightDelim, err := t.expect(lex.TokenTypeRightDelim, context)
	if err != nil {
		return false, err
	}
	t.RootTrimFirst = rightDelim.TrimRight
	var end Node
	t.Root, end, err = t.itemList()
	if err != nil {
		return false, err
	}
	en, ok := end.(*endNode)
	if !ok {
		return false, t.errorf("unexpected %s in %s", end, context)
	}
	t.RootTrimLast = en.TrimLeft
	if err := t.add(); err != nil {
		return false, err
	}
	t.stopParse()
	return en.TrimRight, nil
}

// itemList:
//
//	textOrAction*
//
// Terminates at {{end}} or {{else}}, returned separately.
func (t *Tree) itemList() (*ListNode, Node, error) {
	list := t.newList(t.peekNonSpace().Pos)
	for t.peekNonSpace().Type != lex.TokenTypeEOF {
		n, err := t.textOrAction()
		if err != nil {
			return nil, nil, err
		}
		switch n.(type) {
		case *endNode, *elseNode:
			list.end = n.Start()
			return list, n, nil
		default:
			list.Nodes = append(list.Nodes, n)
		}
	}
	return nil, nil, t.errorf("unexpected EOF")
}

// textOrAction:
//
//	text | comment | action
func (t *Tree) textOrAction() (Node, error) {
	switch token := t.nextNonSpace(); token.Type {
	case lex.TokenTypeText:
		return t.newText(token.Pos, token.End, token.Value), nil
	case lex.TokenTypeLeftDelim:
		t.actionLine = token.Line
		t.actionPos = token.Pos
		t.actionTrimLeft = token.TrimLeft
		defer t.clearActionLine()
		return t.action()
	case lex.TokenTypeComment:
		c := t.newComment(token.Pos, token.End, token.Value)
		c.TrimLeft = token.TrimLeft
		c.TrimRight = token.TrimRight
		return c, nil
	default:
		// In HTML mode, delegate HTML tokens to the HTML parser.
		if t.scanner.Options.ParseHTML {
			t.backup()
			return t.htmlTextOrAction()
		}
		return nil, t.unexpected(token, "input")
	}
}

func (t *Tree) clearActionLine() {
	t.actionLine = 0
	t.actionPos = 0
	t.actionTrimLeft = false
	t.actionTrimRight = false
}

// Action:
//
//	control
//	command ("|" command)*
//
// Left delim is past. Now get actions.
// First word could be a keyword such as range.
func (t *Tree) action() (Node, error) {
	switch token := t.nextNonSpace(); token.Type {
	case lex.TokenTypeBlock:
		return t.blockControl()
	case lex.TokenTypeBreak:
		return t.breakControl(t.actionPos, token.Pos, token.Line)
	case lex.TokenTypeContinue:
		return t.continueControl(t.actionPos, token.Pos, token.Line)
	case lex.TokenTypeElse:
		return t.elseControl()
	case lex.TokenTypeEnd:
		return t.endControl()
	case lex.TokenTypeIf:
		return t.ifControl()
	case lex.TokenTypeRange:
		return t.rangeControl()
	case lex.TokenTypeTemplate:
		return t.templateControl()
	case lex.TokenTypeWith:
		return t.withControl()
	default:
		t.backup()
		token := t.peek()
		// Do not pop variables; they persist until "end".
		pipe, err := t.pipeline("command", lex.TokenTypeRightDelim)
		if err != nil {
			return nil, err
		}
		a := t.newAction(t.actionPos, pipe.end, token.Line, pipe)
		a.TrimLeft = t.actionTrimLeft
		a.TrimRight = t.actionTrimRight
		return a, nil
	}
}

// Break:
//
//	{{break}}
//
// Break keyword is past.
func (t *Tree) breakControl(pos, keyword lex.Pos, line int) (Node, error) {
	token := t.nextNonSpace()
	if token.Type != lex.TokenTypeRightDelim {
		return nil, t.unexpected(token, "{{break}}")
	}
	if t.rangeDepth == 0 {
		return nil, t.errorf("{{break}} outside {{range}}")
	}
	n := t.newBreak(pos, token.End, line)
	n.keyword = keyword
	n.TrimLeft = t.actionTrimLeft
	n.TrimRight = token.TrimRight
	return n, nil
}

// Continue:
//
//	{{continue}}
//
// Continue keyword is past.
func (t *Tree) continueControl(pos, keyword lex.Pos, line int) (Node, error) {
	token := t.nextNonSpace()
	if token.Type != lex.TokenTypeRightDelim {
		return nil, t.unexpected(token, "{{continue}}")
	}
	if t.rangeDepth == 0 {
		return nil, t.errorf("{{continue}} outside {{range}}")
	}
	n := t.newContinue(pos, token.End, line)
	n.keyword = keyword
	n.TrimLeft = t.actionTrimLeft
	n.TrimRight = token.TrimRight
	return n, nil
}

// Pipeline:
//
//	declarations? command ('|' command)*
func (t *Tree) pipeline(context string, end lex.TokenType) (*PipeNode, error) {
	token := t.peekNonSpace()
	pipe := t.newPipeline(token.Pos, token.Line, nil)
	// Are there declarations or assignments?
decls:
	if v := t.peekNonSpace(); v.Type == lex.TokenTypeVariable {
		t.next()
		// Since space is a token, we need 3-token look-ahead here in the worst case:
		// in "$x foo" we need to read "foo" (as opposed to ":=") to know that $x is an
		// argument variable rather than a declaration. So remember the token
		// adjacent to the variable so we can push it back if necessary.
		tokenAfterVariable := t.peek()
		next := t.peekNonSpace()
		switch {
		case next.Type == lex.TokenTypeAssign, next.Type == lex.TokenTypeDeclare:
			pipe.IsAssign = next.Type == lex.TokenTypeAssign
			t.nextNonSpace()
			pipe.Decl = append(pipe.Decl, t.newVariable(v.Pos, v.End, v.Value))
			t.vars = append(t.vars, v.Value)
		case next.Type == lex.TokenTypeChar && next.Value == ",":
			t.nextNonSpace()
			pipe.Decl = append(pipe.Decl, t.newVariable(v.Pos, v.End, v.Value))
			t.vars = append(t.vars, v.Value)
			if context == "range" && len(pipe.Decl) < 2 {
				switch t.peekNonSpace().Type {
				case lex.TokenTypeVariable, lex.TokenTypeRightDelim, lex.TokenTypeRightParen:
					// second initialized variable in a range pipeline
					goto decls
				default:
					return nil, t.errorf("range can only initialize variables")
				}
			}
			return nil, t.errorf("too many declarations in %s", context)
		case tokenAfterVariable.Type == lex.TokenTypeSpace:
			t.backup3(v, tokenAfterVariable)
		default:
			t.backup2(v)
		}
	}
	for {
		switch token := t.nextNonSpace(); token.Type {
		case end:
			// At this point, the pipeline is complete
			pipe.end = token.End
			if token.Type == lex.TokenTypeRightDelim {
				t.actionTrimRight = token.TrimRight
			}
			if err := t.checkPipeline(pipe, context); err != nil {
				return nil, err
			}
			return pipe, nil
		case lex.TokenTypeBool, lex.TokenTypeCharConstant, lex.TokenTypeComplex, lex.TokenTypeDot, lex.TokenTypeField, lex.TokenTypeIdentifier,
			lex.TokenTypeNumber, lex.TokenTypeNil, lex.TokenTypeRawString, lex.TokenTypeString, lex.TokenTypeVariable, lex.TokenTypeLeftParen:
			t.backup()
			cmd, err := t.command()
			if err != nil {
				return nil, err
			}
			pipe.Commands = append(pipe.Commands, cmd)
		default:
			return nil, t.unexpected(token, context)
		}
	}
}

func (t *Tree) checkPipeline(pipe *PipeNode, context string) error {
	// Reject empty pipelines
	if len(pipe.Commands) == 0 {
		return t.errorf("missing value for %s", context)
	}
	// Only the first command of a pipeline can start with a non-executable operand
	for i, c := range pipe.Commands[1:] {
		switch c.Args[0].(type) {
		case *BoolNode, *DotNode, *NilNode, *NumberNode, *StringNode:
			// With A|B|C, pipeline stage 2 is B
			return t.errorf("non executable command in pipeline stage %d", i+2)
		default:
		}
	}
	return nil
}

// branchTrim carries trim marker information for a branch node's opener, else, and end actions.
type branchTrim struct {
	StartTrimLeft, StartTrimRight bool
	ElseTrimLeft, ElseTrimRight   bool
	EndTrimLeft, EndTrimRight     bool
	ElseChained                   bool // else written {{else if}} / {{else with}}
}

func (t *Tree) parseControl(context string) (pos lex.Pos, endPos lex.Pos, line int, pipe *PipeNode, list, elseList *ListNode, trim branchTrim, err error) {
	actionPos := t.actionPos
	trim.StartTrimLeft = t.actionTrimLeft
	defer t.popVars(len(t.vars))
	pipe, err = t.pipeline(context, lex.TokenTypeRightDelim)
	if err != nil {
		return
	}
	trim.StartTrimRight = t.actionTrimRight
	if context == "range" {
		t.rangeDepth++
	}
	var next Node
	list, next, err = t.itemList()
	if context == "range" {
		t.rangeDepth--
	}
	if err != nil {
		return
	}
	switch n := next.(type) {
	case *endNode: // done
		endPos = n.End()
		trim.EndTrimLeft = n.TrimLeft
		trim.EndTrimRight = n.TrimRight
	case *elseNode:
		trim.ElseTrimLeft = n.TrimLeft
		trim.ElseTrimRight = n.TrimRight
		// Special case for "else if" and "else with".
		// If the "else" is followed immediately by an "if" or "with",
		// the elseControl will have left the "if" or "with" token pending. Treat
		//	{{if a}}_{{else if b}}_{{end}}
		//  {{with a}}_{{else with b}}_{{end}}
		// as
		//	{{if a}}_{{else}}{{if b}}_{{end}}{{end}}
		//  {{with a}}_{{else}}{{with b}}_{{end}}{{end}}.
		// To do this, parse the "if" or "with" as usual and stop at it {{end}};
		// the subsequent{{end}} is assumed. This technique works even for long if-else-if chains.
		if context == "if" && t.peek().Type == lex.TokenTypeIf {
			t.next() // Consume the "if" token.
			// The chained if shares the else's {{- and starts at the
			// {{else if}} action; textOrAction's defer cleared both.
			t.actionTrimLeft = n.TrimLeft
			t.actionPos = next.Start()
			elseList = t.newList(next.Start())
			var ifNode Node
			ifNode, err = t.ifControl()
			if err != nil {
				return
			}
			elseList.Nodes = append(elseList.Nodes, ifNode)
			endPos = ifNode.End()
			elseList.end = endPos
			trim.ElseChained = true
			// The chain's {{end}} trim lives on the innermost node.
			inner := ifNode.(*IfNode)
			trim.EndTrimLeft = inner.EndTrimLeft
			trim.EndTrimRight = inner.EndTrimRight
		} else if context == "with" && t.peek().Type == lex.TokenTypeWith {
			t.next()
			t.actionTrimLeft = n.TrimLeft
			t.actionPos = next.Start()
			elseList = t.newList(next.Start())
			var withNode Node
			withNode, err = t.withControl()
			if err != nil {
				return
			}
			elseList.Nodes = append(elseList.Nodes, withNode)
			endPos = withNode.End()
			elseList.end = endPos
			trim.ElseChained = true
			inner := withNode.(*WithNode)
			trim.EndTrimLeft = inner.EndTrimLeft
			trim.EndTrimRight = inner.EndTrimRight
		} else {
			elseList, next, err = t.itemList()
			if err != nil {
				return
			}
			if en, ok := next.(*endNode); !ok {
				err = t.errorf("expected end; found %s", next)
				return
			} else {
				trim.EndTrimLeft = en.TrimLeft
				trim.EndTrimRight = en.TrimRight
			}
			endPos = next.End()
		}
	default:
	}
	return actionPos, endPos, pipe.Line, pipe, list, elseList, trim, nil
}

// If:
//
//	{{if pipeline}} itemList {{end}}
//	{{if pipeline}} itemList {{else}} itemList {{end}}
//
// If keyword is past.
func (t *Tree) ifControl() (Node, error) {
	pos, endPos, line, pipe, list, elseList, trim, err := t.parseControl("if")
	if err != nil {
		return nil, err
	}
	n := t.newIf(pos, endPos, line, pipe, list, elseList)
	n.StartTrimLeft = trim.StartTrimLeft
	n.StartTrimRight = trim.StartTrimRight
	n.ElseTrimLeft = trim.ElseTrimLeft
	n.ElseTrimRight = trim.ElseTrimRight
	n.EndTrimLeft = trim.EndTrimLeft
	n.EndTrimRight = trim.EndTrimRight
	n.elseChained = trim.ElseChained
	return n, nil
}

// Range:
//
//	{{range pipeline}} itemList {{end}}
//	{{range pipeline}} itemList {{else}} itemList {{end}}
//
// Range keyword is past.
func (t *Tree) rangeControl() (Node, error) {
	pos, endPos, line, pipe, list, elseList, trim, err := t.parseControl("range")
	if err != nil {
		return nil, err
	}
	n := t.newRange(pos, endPos, line, pipe, list, elseList)
	n.StartTrimLeft = trim.StartTrimLeft
	n.StartTrimRight = trim.StartTrimRight
	n.ElseTrimLeft = trim.ElseTrimLeft
	n.ElseTrimRight = trim.ElseTrimRight
	n.EndTrimLeft = trim.EndTrimLeft
	n.EndTrimRight = trim.EndTrimRight
	n.elseChained = trim.ElseChained
	return n, nil
}

// With:
//
//	{{with pipeline}} itemList {{end}}
//	{{with pipeline}} itemList {{else}} itemList {{end}}
//
// If keyword is past.
func (t *Tree) withControl() (Node, error) {
	pos, endPos, line, pipe, list, elseList, trim, err := t.parseControl("with")
	if err != nil {
		return nil, err
	}
	n := t.newWith(pos, endPos, line, pipe, list, elseList)
	n.StartTrimLeft = trim.StartTrimLeft
	n.StartTrimRight = trim.StartTrimRight
	n.ElseTrimLeft = trim.ElseTrimLeft
	n.ElseTrimRight = trim.ElseTrimRight
	n.EndTrimLeft = trim.EndTrimLeft
	n.EndTrimRight = trim.EndTrimRight
	n.elseChained = trim.ElseChained
	return n, nil
}

// End:
//
//	{{end}}
//
// End keyword is past.
func (t *Tree) endControl() (Node, error) {
	token, err := t.expect(lex.TokenTypeRightDelim, "end")
	if err != nil {
		return nil, err
	}
	n := t.newEnd(t.actionPos, token.End)
	n.TrimLeft = t.actionTrimLeft
	n.TrimRight = token.TrimRight
	return n, nil
}

// Else:
//
//	{{else}}
//
// Else keyword is past.
func (t *Tree) elseControl() (Node, error) {
	peek := t.peekNonSpace()
	actionPos := t.actionPos
	trimLeft := t.actionTrimLeft
	// The "{{else if ... " and "{{else with ..." will be
	// treated as "{{else}}{{if ..." and "{{else}}{{with ...".
	// So return the else node here.
	if peek.Type == lex.TokenTypeIf || peek.Type == lex.TokenTypeWith {
		n := t.newElse(actionPos, peek.End, peek.Line)
		n.TrimLeft = trimLeft
		// TrimRight is not set here because the right delim belongs to the
		// inner if/with action that follows.
		return n, nil
	}
	token, err := t.expect(lex.TokenTypeRightDelim, "else")
	if err != nil {
		return nil, err
	}
	n := t.newElse(actionPos, token.End, token.Line)
	n.TrimLeft = trimLeft
	n.TrimRight = token.TrimRight
	return n, nil
}

// Block:
//
//	{{block stringValue pipeline}}
//
// Block keyword is past.
// The name must be something that can evaluate to a string.
// The pipeline is mandatory.
func (t *Tree) blockControl() (Node, error) {
	const context = "block clause"
	actionPos := t.actionPos
	startTrimLeft := t.actionTrimLeft

	token := t.nextNonSpace()
	name, err := t.parseTemplateName(token, context)
	if err != nil {
		return nil, err
	}
	pipe, err := t.pipeline(context, lex.TokenTypeRightDelim)
	if err != nil {
		return nil, err
	}
	// pipeline left the opener's -}} in t.actionTrimRight.
	startTrimRight := t.actionTrimRight

	var end Node
	var body *ListNode
	body, end, err = t.itemList()
	if err != nil {
		return nil, err
	}
	en, ok := end.(*endNode)
	if !ok {
		return nil, t.errorf("unexpected %s in %s", end, context)
	}

	// Register the body as the block's tree, as text/template does. It
	// carries the block's own trim markers: once the body is a tree of its
	// own, nothing else applies them.
	block := &Tree{Name: name}
	block.Root = body
	block.text = t.text
	block.leftDelim, block.rightDelim = t.leftDelim, t.rightDelim
	block.Mode = t.Mode
	block.ParseName = t.ParseName
	block.Filepath = t.Filepath
	block.HTMLMode = t.HTMLMode
	block.RootTrimFirst = startTrimRight
	block.RootTrimLast = en.TrimLeft
	if err := t.addDefinition(block); err != nil {
		return nil, err
	}

	b := t.newBlock(actionPos, end.End(), token.Line, name, pipe, body)
	b.call.nameStart = token.Pos
	b.StartTrimLeft = startTrimLeft
	b.StartTrimRight = startTrimRight
	b.EndTrimLeft = en.TrimLeft
	b.EndTrimRight = en.TrimRight
	return b, nil
}

// Template:
//
//	{{template stringValue pipeline}}
//
// Template keyword is past. The name must be something that can evaluate
// to a string.
func (t *Tree) templateControl() (Node, error) {
	const context = "template clause"
	actionPos := t.actionPos
	trimLeft := t.actionTrimLeft
	token := t.nextNonSpace()
	name, err := t.parseTemplateName(token, context)
	if err != nil {
		return nil, err
	}
	var pipe *PipeNode
	rightDelim := t.nextNonSpace()
	if rightDelim.Type != lex.TokenTypeRightDelim {
		t.backup()
		// Do not pop variables; they persist until "end".
		pipe, err = t.pipeline(context, lex.TokenTypeRightDelim)
		if err != nil {
			return nil, err
		}
		n := t.newTemplate(actionPos, pipe.end, token.Line, name, pipe)
		n.nameStart = token.Pos
		n.TrimLeft = trimLeft
		n.TrimRight = t.actionTrimRight
		return n, nil
	}
	n := t.newTemplate(actionPos, rightDelim.End, token.Line, name, pipe)
	n.nameStart = token.Pos
	n.TrimLeft = trimLeft
	n.TrimRight = rightDelim.TrimRight
	return n, nil
}

func (t *Tree) parseTemplateName(token lex.Token, context string) (string, error) {
	switch token.Type {
	case lex.TokenTypeString, lex.TokenTypeRawString:
		s, err := strconv.Unquote(token.Value)
		if err != nil {
			return "", t.error(err)
		}
		return s, nil
	default:
		return "", t.unexpected(token, context)
	}
}

// command:
//
//	operand (space operand)*
//
// space-separated arguments up to a pipeline character or right delimiter.
// we consume the pipe character but leave the right delim to terminate the action.
func (t *Tree) command() (*CommandNode, error) {
	cmd := t.newCommand(t.peekNonSpace().Pos)
	for {
		t.peekNonSpace() // skip leading spaces.
		operand, err := t.operand()
		if err != nil {
			return nil, err
		}
		if operand != nil {
			cmd.Args = append(cmd.Args, operand)
		}
		switch token := t.next(); token.Type {
		case lex.TokenTypeSpace:
			continue
		case lex.TokenTypeRightDelim, lex.TokenTypeRightParen:
			t.backup()
		case lex.TokenTypePipe:
		default:
			return nil, t.unexpected(token, "operand")
		}
		break
	}
	if len(cmd.Args) == 0 {
		return nil, t.errorf("empty command")
	}
	if last := cmd.Args[len(cmd.Args)-1]; last != nil {
		cmd.end = last.End()
	}
	return &cmd, nil
}

// operand:
//
//	term .Field*
//
// An operand is a space-separated component of a command,
// a term possibly followed by field accesses.
// A nil return means the next item is not an operand.
func (t *Tree) operand() (Node, error) {
	// A chain starts where its term does, which for a parenthesized
	// pipeline is at the '(' rather than at the pipeline inside it.
	start := t.peekNonSpace().Pos
	node, err := t.term()
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, nil
	}
	if t.peek().Type == lex.TokenTypeField {
		chain := t.newChain(start, node)
		for t.peek().Type == lex.TokenTypeField {
			fieldToken := t.next()
			chain.Add(fieldToken.Value)
			chain.end = fieldToken.End
		}
		// Compatibility with original API: If the term is of type NodeField
		// or NodeVariable, just put more fields on the original.
		// Otherwise, keep the Chain node.
		// Obvious parsing errors involving literal values are detected here.
		// More complex error cases will have to be handled at execution time.
		switch node.(type) {
		case *FieldNode:
			node = t.newField(chain.Start(), chain.end, chain.String())
		case *VariableNode:
			node = t.newVariable(chain.Start(), chain.end, chain.String())
		case *BoolNode, *StringNode, *NumberNode, *NilNode, *DotNode:
			return nil, t.errorf("unexpected . after term %q", node.String())
		default:
			node = chain
		}
	}
	return node, nil
}

// term:
//
//	literal (number, string, nil, boolean)
//	function (identifier)
//	.
//	.Field
//	$
//	'(' pipeline ')'
//
// A term is a simple "expression".
// A nil return means the next item is not a term.
func (t *Tree) term() (Node, error) {
	switch token := t.nextNonSpace(); token.Type {
	case lex.TokenTypeIdentifier:
		if !t.hasFunction(token.Value) {
			return nil, t.errorf("function %q not defined", token.Value)
		}
		return NewIdentifier(t, token.Pos, token.End, token.Value), nil
	case lex.TokenTypeDot:
		return t.newDot(token.Pos, token.End), nil
	case lex.TokenTypeNil:
		return t.newNil(token.Pos, token.End), nil
	case lex.TokenTypeVariable:
		return t.useVar(token.Pos, token.End, token.Value)
	case lex.TokenTypeField:
		return t.newField(token.Pos, token.End, token.Value), nil
	case lex.TokenTypeBool:
		return t.newBool(token.Pos, token.End, token.Value == "true"), nil
	case lex.TokenTypeCharConstant, lex.TokenTypeComplex, lex.TokenTypeNumber:
		number, err := t.newNumber(token.Pos, token.End, token.Value, token.Type)
		if err != nil {
			return nil, t.error(err)
		}
		return number, nil
	case lex.TokenTypeLeftParen:
		if t.stackDepth >= maxStackDepth {
			return nil, t.errorf("max expression depth exceeded")
		}
		t.stackDepth++
		defer func() { t.stackDepth-- }()
		return t.pipeline("parenthesized pipeline", lex.TokenTypeRightParen)
	case lex.TokenTypeString, lex.TokenTypeRawString:
		s, err := strconv.Unquote(token.Value)
		if err != nil {
			return nil, t.error(err)
		}
		return t.newString(token.Pos, token.End, token.Value, s), nil
	default:
		t.backup()
		return nil, nil
	}
}

func (t *Tree) popVars(n int) {
	t.vars = t.vars[:n]
}

// useVar returns a node for a variable reference. It errors if the
// variable is not defined.
func (t *Tree) useVar(pos, end lex.Pos, name string) (Node, error) {
	v := t.newVariable(pos, end, name)
	for _, varName := range t.vars {
		if varName == v.Ident[0] {
			return v, nil
		}
	}
	return nil, t.errorf("undefined variable %q", v.Ident[0])
}
