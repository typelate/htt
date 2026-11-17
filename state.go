package htt

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"strings"

	"golang.org/x/net/html"
)

type variable struct {
	name  string
	value reflect.Value
}

// state tracks the execution state of a single template.
type state struct {
	wr         io.Writer
	tree       *Tree
	cp         *CoverProfile
	vars       []variable
	depth      int
	funcs      FuncMap
	finder     TreeFinder
	missingKey MissingKeyMode
	trust      TrustFunc
	markup     *markup // what the source writes that html.Node cannot hold; nil records nothing
	// attrSource is set while a control structure in an attribute value
	// is walked. It holds the text nodes made from the template's own
	// text, which alone are written unescaped.
	attrSource map[*html.Node]bool
	// scan follows the text written so far through tags the parser left
	// as text; called templates share it. rawText marks script, style and
	// a template called from an attribute value, where text is not
	// followed. escapableText marks textarea and title, where each '<' is
	// escaped.
	scan          *tagScan
	rawText       bool
	escapableText bool
}

// tagAt returns the scan of the text written so far, or nil where text is
// not followed.
func (s *state) tagAt() *tagScan {
	if s.markup == nil || s.rawText || s.escapableText || s.attrSource != nil {
		return nil
	}
	if s.scan == nil {
		s.scan = new(tagScan)
	}
	return s.scan
}

// writeText returns text as html/template writes it: a '<' that starts no
// tag is escaped, so that an action after it cannot name one.
func (s *state) writeText(text string) string {
	if at := s.tagAt(); at != nil {
		text, *at = at.write(text)
		return text
	}
	if s.escapableText && s.markup != nil && !s.rawText && s.attrSource == nil {
		return strings.ReplaceAll(text, "<", "&lt;")
	}
	return text
}

// trustOf returns v's Trust, which is TrustNone without a TrustFunc.
func (s *state) trustOf(v reflect.Value) Trust {
	if s.trust == nil {
		return TrustNone
	}
	return s.trust(v)
}

func (s *state) execute(dot reflect.Value) error {
	s.cp.AddTree(s.tree)
	root := s.tree.Root
	if root == nil {
		return nil
	}
	if len(root.Nodes) == 1 {
		if frag, ok := root.Nodes[0].(*HTMLDocumentFragmentNode); ok {
			return s.executeHTML(dot, frag)
		}
	}
	// The tree of a define or block has no fragment, and still needs
	// contextual escaping.
	if s.tree.HTMLMode || rootHasHTMLNodes(root) {
		return s.executeHTMLList(dot, root)
	}
	return s.walk(dot, root)
}

func (s *state) executeHTML(dot reflect.Value, frag *HTMLDocumentFragmentNode) error {
	doc, err := s.buildDocument(dot, frag)
	if err != nil {
		return err
	}
	return render(s.wr, doc, s.markup)
}

// executeHTMLList executes a root list that holds HTML nodes with no
// fragment around them, as the tree of a define or block does.
func (s *state) executeHTMLList(dot reflect.Value, list *ListNode) error {
	nodes, err := s.walkHTMLList(dot, list, s.tree.RootTrimFirst, s.tree.RootTrimLast)
	if err != nil {
		return err
	}
	doc := &html.Node{Type: html.DocumentNode}
	for _, n := range nodes {
		doc.AppendChild(n)
	}
	return render(s.wr, doc, s.markup)
}

// rootHasHTMLNodes reports whether list holds an element, comment or
// doctype.
func rootHasHTMLNodes(list *ListNode) bool {
	for _, n := range list.Nodes {
		switch n.(type) {
		case *HTMLElementNode, *HTMLCommentNode, *HTMLDoctypeNode:
			return true
		}
	}
	return false
}

// buildDocument builds an *html.Node (DocumentNode) from an HTML fragment.
func (s *state) buildDocument(dot reflect.Value, frag *HTMLDocumentFragmentNode) (*html.Node, error) {
	doc := &html.Node{
		Type: html.DocumentNode,
	}
	nodes, err := s.walkHTMLNodes(dot, frag.Children, false, false)
	if err != nil {
		return nil, err
	}
	for _, n := range nodes {
		doc.AppendChild(n)
	}
	return doc, nil
}

func (s *state) walk(dot reflect.Value, node Node) error {
	switch n := node.(type) {
	case *ListNode:
		return s.walkList(dot, n, false, false)
	case *TextNode:
		n.count.Add(1)
		if _, err := s.wr.Write(n.Text); err != nil {
			return s.writeError(err)
		}
	case *ActionNode:
		return s.walkAction(dot, n)
	case *IfNode:
		return s.walkIfNode(dot, n)
	case *RangeNode:
		return s.walkRangeNode(dot, n)
	case *WithNode:
		return s.walkWithNode(dot, n)
	case *TemplateNode:
		return s.walkTemplateNode(dot, n)
	case *BreakNode:
		n.count.Add(1)
		return breakError{}
	case *ContinueNode:
		n.count.Add(1)
		return continueError{}
	case *CommentNode:
		// A template comment writes nothing.
	case *DefineNode:
		// A definition writes nothing where it stands.
	case *BlockNode:
		// A block executes as a call of the template it defines.
		return s.walkTemplateNode(dot, s.tree.newTemplate(n.Start(), n.End(), n.Line, n.Name, n.Pipe))
	default:
		return s.errorf("unknown node: %s", node)
	}
	return nil
}

func (s *state) walkList(dot reflect.Value, list *ListNode, trimFirst, trimLast bool) error {
	nodes := list.Nodes
	for i, node := range nodes {
		if tn, ok := node.(*TextNode); ok {
			text := tn.Text
			if (i == 0 && trimFirst) || (i > 0 && nodeRightTrim(nodes[i-1])) {
				text = bytes.TrimLeft(text, trimCutset)
			}
			if (i == len(nodes)-1 && trimLast) || (i < len(nodes)-1 && nodeLeftTrim(nodes[i+1])) {
				text = bytes.TrimRight(text, trimCutset)
			}
			tn.count.Add(1)
			if len(text) > 0 {
				if _, err := s.wr.Write(text); err != nil {
					return s.writeError(err)
				}
			}
			continue
		}
		if err := s.walk(dot, node); err != nil {
			return err
		}
	}
	return nil
}

const trimCutset = " \t\r\n"

// nodeLeftTrim reports whether a node trims whitespace from preceding text.
func nodeLeftTrim(n Node) bool {
	switch n := n.(type) {
	case *ActionNode:
		return n.TrimLeft
	case *IfNode:
		return n.StartTrimLeft
	case *RangeNode:
		return n.StartTrimLeft
	case *WithNode:
		return n.StartTrimLeft
	case *TemplateNode:
		return n.TrimLeft
	case *CommentNode:
		return n.TrimLeft
	case *BreakNode:
		return n.TrimLeft
	case *ContinueNode:
		return n.TrimLeft
	case *DefineNode:
		return n.StartTrimLeft
	}
	return false
}

// nodeRightTrim reports whether a node trims whitespace from following text.
func nodeRightTrim(n Node) bool {
	switch n := n.(type) {
	case *ActionNode:
		return n.TrimRight
	case *IfNode:
		return n.EndTrimRight
	case *RangeNode:
		return n.EndTrimRight
	case *WithNode:
		return n.EndTrimRight
	case *TemplateNode:
		return n.TrimRight
	case *CommentNode:
		return n.TrimRight
	case *BreakNode:
		return n.TrimRight
	case *ContinueNode:
		return n.TrimRight
	case *DefineNode:
		return n.EndTrimRight
	}
	return false
}

func (s *state) walkAction(dot reflect.Value, action *ActionNode) error {
	action.count.Add(1)
	val, err := s.evalPipeline(dot, action.Pipe)
	if err != nil {
		return err
	}
	if len(action.Pipe.Decl) > 0 {
		// Variable assignment/declaration — no output.
		return nil
	}
	if _, err := s.printValue(s.wr, val); err != nil {
		return s.writeError(err)
	}
	return nil
}

func (s *state) walkIfNode(dot reflect.Value, n *IfNode) error {
	return s.walkBranch(dot, &n.BranchNode, "if")
}

func (s *state) walkWithNode(dot reflect.Value, n *WithNode) error {
	return s.walkBranch(dot, &n.BranchNode, "with")
}

// bodyLastTrim reports whether the body's last node trims the space after
// it. When the else list has nodes it sits next to the body, so its {{-
// decides; otherwise the {{- on the end does.
func bodyLastTrim(branch *BranchNode) bool {
	if branch.ElseList != nil && len(branch.ElseList.Nodes) > 0 {
		return branch.ElseTrimLeft
	}
	return branch.EndTrimLeft
}

func (s *state) walkBranch(dot reflect.Value, branch *BranchNode, name string) error {
	branch.openCount.Add(1)
	mark := len(s.vars)
	defer func() { s.vars = s.vars[:mark] }()
	pipe, err := s.evalPipeline(dot, branch.Pipe)
	if err != nil {
		return err
	}
	truth, ok := isTrue(indirectInterface(pipe))
	if !ok {
		return s.errorf("if/with can't use %v", pipe)
	}
	var walkErr error
	if truth {
		newDot := dot
		if name == "with" {
			newDot = pipe
		}
		lastTrim := bodyLastTrim(branch)
		walkErr = s.walkList(newDot, branch.List, branch.StartTrimRight, lastTrim)
	} else if branch.ElseList != nil {
		branch.elseCount.Add(1)
		walkErr = s.walkList(dot, branch.ElseList, branch.ElseTrimRight, branch.EndTrimLeft)
	}
	if walkErr != nil {
		return walkErr
	}
	branch.endCount.Add(1)
	return nil
}

// walkRangeNode handles {{range}} iteration.
func (s *state) walkRangeNode(dot reflect.Value, r *RangeNode) error {
	r.BranchNode.openCount.Add(1)
	val, err := s.evalPipeline(dot, r.Pipe)
	if err != nil {
		return err
	}
	val, _ = indirect(val)
	mark := len(s.vars)
	ran, err := s.rangeOver(val, len(r.Pipe.Decl), func(index, elem reflect.Value) (bool, error) {
		s.setRangeVars(r.Pipe, index, elem)
		lastTrim := bodyLastTrim(&r.BranchNode)
		err := s.walkList(elem, r.List, r.StartTrimRight, lastTrim)
		s.vars = s.vars[:mark]
		if isBreak(err) {
			return true, nil
		}
		if isContinue(err) {
			return false, nil
		}
		return false, err
	})
	if err != nil {
		return err
	}
	s.vars = s.vars[:mark]
	if !ran && r.ElseList != nil {
		r.BranchNode.elseCount.Add(1)
		if walkErr := s.walkList(dot, r.ElseList, r.ElseTrimRight, r.EndTrimLeft); walkErr != nil {
			return walkErr
		}
	}
	r.BranchNode.endCount.Add(1)
	return nil
}

func (s *state) walkTemplateNode(dot reflect.Value, t *TemplateNode) error {
	t.count.Add(1)
	if s.depth >= maxExecDepth {
		return s.errorf("exceeded maximum template depth (%d)", maxExecDepth)
	}
	if s.finder == nil {
		return s.errorf("template %q not defined", t.Name)
	}
	tree, ok := s.finder.FindTree(t.Name)
	if !ok {
		return s.errorf("template %q not defined", t.Name)
	}
	newDot := dot
	if t.Pipe != nil {
		var err error
		newDot, err = s.evalPipeline(dot, t.Pipe)
		if err != nil {
			return err
		}
	}
	newState := &state{
		wr:         s.wr,
		tree:       tree,
		cp:         s.cp,
		depth:      s.depth + 1,
		funcs:      s.funcs,
		finder:     s.finder,
		missingKey: s.missingKey,
		trust:      s.trust,
		vars:       []variable{{"$", newDot}},
	}
	return newState.execute(newDot)
}

// evalPipeline evaluates a pipeline and returns its final value.
func (s *state) evalPipeline(dot reflect.Value, pipe *PipeNode) (reflect.Value, error) {
	if pipe == nil {
		return missingVal, nil
	}
	var value reflect.Value
	for _, cmd := range pipe.Commands {
		var err error
		value, err = s.evalCommand(dot, cmd, value)
		if err != nil {
			return missingVal, err
		}
	}
	for _, vr := range pipe.Decl {
		if pipe.IsAssign {
			s.setVar(vr.Ident[0], value)
		} else {
			s.push(vr.Ident[0], value)
		}
	}
	return value, nil
}

// evalCommand evaluates a single command in a pipeline. The final argument is the
// result of the previous command (for pipe chaining), or invalid for the first command.
func (s *state) evalCommand(dot reflect.Value, cmd *CommandNode, final reflect.Value) (reflect.Value, error) {
	firstWord := cmd.Args[0]
	switch n := firstWord.(type) {
	case *FieldNode:
		return s.evalFieldNode(dot, n, cmd.Args, final)
	case *ChainNode:
		return s.evalChainNode(dot, n, cmd.Args, final)
	case *IdentifierNode:
		return s.evalFunction(dot, n, cmd, cmd.Args, final)
	case *PipeNode:
		if len(cmd.Args) > 1 || final.IsValid() {
			// Name the stage as text/template does (its commands, without the
			// parentheses) rather than by source span.
			var stage strings.Builder
			n.writeTo(&stage)
			return missingVal, s.errorf("can't give argument to non-function %s", stage.String())
		}
		return s.evalPipeline(dot, n)
	case *VariableNode:
		return s.evalVariableNode(dot, n, cmd.Args, final)
	}
	notAFunction := len(cmd.Args) > 1 || final.IsValid()
	if notAFunction {
		return missingVal, s.errorf("can't give argument to non-function %s", firstWord)
	}
	switch n := firstWord.(type) {
	case *BoolNode:
		return reflect.ValueOf(n.True), nil
	case *DotNode:
		return dot, nil
	case *NilNode:
		return missingVal, s.errorf("nil is not a command")
	case *NumberNode:
		return s.idealConstant(n)
	case *StringNode:
		return reflect.ValueOf(n.Text), nil
	}
	return missingVal, s.errorf("can't evaluate command %q", firstWord)
}

func (s *state) idealConstant(constant *NumberNode) (reflect.Value, error) {
	switch {
	case constant.IsComplex:
		return reflect.ValueOf(constant.Complex128), nil
	case constant.IsFloat &&
		!isHexInt(constant.Text) && !isRuneInt(constant.Text) &&
		strings.ContainsAny(constant.Text, ".eEpP"):
		return reflect.ValueOf(constant.Float64), nil
	case constant.IsInt:
		n := int(constant.Int64)
		if int64(n) != constant.Int64 {
			return missingVal, s.errorf("%s overflows int", constant.Text)
		}
		return reflect.ValueOf(n), nil
	case constant.IsUint:
		return missingVal, s.errorf("%s overflows int", constant.Text)
	}
	return missingVal, nil
}

func isHexInt(s string) bool {
	return len(s) > 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') && !strings.ContainsAny(s, "pP")
}

func isRuneInt(s string) bool {
	return len(s) > 0 && s[0] == '\''
}

func (s *state) evalFieldNode(dot reflect.Value, field *FieldNode, args []Node, final reflect.Value) (reflect.Value, error) {
	return s.evalFieldChain(dot, dot, field.Ident, args, final)
}

func (s *state) evalChainNode(dot reflect.Value, chain *ChainNode, args []Node, final reflect.Value) (reflect.Value, error) {
	if len(chain.Field) == 0 {
		return missingVal, s.errorf("internal error: no fields in evalChainNode")
	}
	pipe, err := s.evalArg(dot, nil, chain.Node)
	if err != nil {
		return missingVal, err
	}
	return s.evalFieldChain(dot, pipe, chain.Field, args, final)
}

func (s *state) evalVariableNode(dot reflect.Value, v *VariableNode, args []Node, final reflect.Value) (reflect.Value, error) {
	value := s.varValue(v.Ident[0])
	if len(v.Ident) == 1 {
		notAFunction := len(args) > 1 || final.IsValid()
		if notAFunction {
			return missingVal, s.errorf("can't give argument to non-function %s", v.Ident[0])
		}
		return value, nil
	}
	return s.evalFieldChain(dot, value, v.Ident[1:], args, final)
}

func (s *state) evalFieldChain(dot, receiver reflect.Value, ident []string, args []Node, final reflect.Value) (reflect.Value, error) {
	n := len(ident)
	for i := 0; i < n-1; i++ {
		var err error
		// zeroArgSentinel, not nil: evalCall counts len(args)-1
		// arguments, and nil would count -1 and reject a method that
		// takes none.
		receiver, err = s.evalField(dot, ident[i], zeroArgSentinel, missingVal, receiver)
		if err != nil {
			return missingVal, err
		}
	}
	return s.evalField(dot, ident[n-1], args, final, receiver)
}

// zeroArgSentinel is the args of a field or method access with no
// arguments. evalCall takes args[0] for the word itself and never reads it.
var zeroArgSentinel = []Node{nil}

func (s *state) evalFunction(dot reflect.Value, node *IdentifierNode, cmd *CommandNode, args []Node, final reflect.Value) (reflect.Value, error) {
	name := node.Ident
	// The builtin and/or evaluate their arguments lazily, as in
	// text/template. A user function of either name is an ordinary call,
	// and one with no arguments falls through to the arg-count error.
	if name == "and" || name == "or" {
		if _, overridden := s.funcs[name]; !overridden && (len(args) > 1 || final.IsValid()) {
			return s.evalAndOr(dot, name, args, final)
		}
	}
	fn, ok := s.findFunction(name)
	if !ok {
		return missingVal, s.errorf("function %q not defined", name)
	}
	return s.evalCall(dot, fn, cmd, name, args, final)
}

// evalAndOr evaluates the builtin and/or over args[1:] and then final, left
// to right, and stops at the operand that decides the result.
func (s *state) evalAndOr(dot reflect.Value, name string, args []Node, final reflect.Value) (reflect.Value, error) {
	returnOnTruth := name == "or"
	var result reflect.Value
	decided := func(v reflect.Value) bool {
		result = v
		return truth(v) == returnOnTruth
	}
	for i := 1; i < len(args); i++ {
		v, err := s.evalArg(dot, nil, args[i])
		if err != nil {
			return missingVal, err
		}
		if decided(v) {
			return result, nil
		}
	}
	if final.IsValid() && decided(final) {
		return result, nil
	}
	return result, nil
}

func (s *state) findFunction(name string) (reflect.Value, bool) {
	if s.funcs != nil {
		if fn, ok := s.funcs[name]; ok {
			return reflect.ValueOf(fn), true
		}
	}
	if fn, ok := builtins()[name]; ok {
		return reflect.ValueOf(fn), true
	}
	return missingVal, false
}

func (s *state) evalField(dot reflect.Value, fieldName string, args []Node, final reflect.Value, receiver reflect.Value) (reflect.Value, error) {
	if !receiver.IsValid() {
		// Nil data has no field; text/template yields no value unless the key is required.
		if s.missingKey == MissingKeyError {
			return missingVal, s.errorf("nil data; no entry for key %q", fieldName)
		}
		return reflect.Value{}, nil
	}
	typ := receiver.Type()
	receiver = indirectInterface(receiver)
	hasArgs := len(args) > 1 || final.IsValid()

	// Try methods first (on the pointer receiver too).
	ptr := receiver
	if ptr.Kind() != reflect.Interface && ptr.CanAddr() {
		ptr = ptr.Addr()
	}
	if method := ptr.MethodByName(fieldName); method.IsValid() {
		return s.evalCall(dot, method, nil, fieldName, args, final)
	}
	// Try methods on the value itself.
	if method := receiver.MethodByName(fieldName); method.IsValid() {
		return s.evalCall(dot, method, nil, fieldName, args, final)
	}
	if hasArgs {
		return missingVal, s.errorf("can't call method/function %q on %s", fieldName, typ)
	}
	// A field is reached through the pointer's Elem.
	structReceiver, isNil := indirect(receiver)
	if isNil {
		return missingVal, s.errorf("nil pointer evaluating field %s in type %s", fieldName, typ)
	}
	switch structReceiver.Kind() {
	case reflect.Struct:
		field, err := s.structField(structReceiver, fieldName, typ)
		if field.IsValid() || err != nil {
			return field, err
		}
	case reflect.Map:
		nameVal := reflect.ValueOf(fieldName)
		if nameVal.Type().AssignableTo(structReceiver.Type().Key()) {
			result := structReceiver.MapIndex(nameVal)
			if result.IsValid() {
				return result, nil
			}
			switch s.missingKey {
			case MissingKeyError:
				return missingVal, s.errorf("map has no entry for key %q", fieldName)
			case MissingKeyZero:
				return reflect.Zero(structReceiver.Type().Elem()), nil
			default: // MissingKeyInvalid
				return reflect.Value{}, nil
			}
		}
	default:
	}
	return missingVal, s.errorf("can't evaluate field %s in type %s", fieldName, typ)
}

// structField returns the field fieldName of the struct v. The value is
// invalid when v has no field of that name. The error is for a field that
// exists but is unexported: it is found and then refused, never read.
func (s *state) structField(v reflect.Value, fieldName string, typ reflect.Type) (reflect.Value, error) {
	if sf, ok := v.Type().FieldByName(fieldName); ok && !sf.IsExported() {
		return missingVal, s.errorf("%s is an unexported field of struct type %s", fieldName, typ)
	}
	return v.FieldByName(fieldName), nil
}

func (s *state) evalCall(dot reflect.Value, fun reflect.Value, cmd *CommandNode, name string, args []Node, final reflect.Value) (reflect.Value, error) {
	typ := fun.Type()
	numIn := typ.NumIn()
	numFixed := len(args) - 1 // first arg is the function name itself
	if final.IsValid() {
		numFixed++
	}
	// For non-variadic, numFixed must equal numIn.
	// For variadic, numFixed must be >= numIn-1.
	if !typ.IsVariadic() {
		if numFixed != numIn {
			return missingVal, s.errorf("wrong number of args for %s: want %d got %d", name, numIn, numFixed)
		}
	} else {
		if numFixed < numIn-1 {
			return missingVal, s.errorf("wrong number of args for %s: want at least %d got %d", name, numIn-1, numFixed)
		}
	}
	if !goodFunc(typ) {
		return missingVal, s.errorf("can't call %s: invalid return signature", name)
	}

	// Build the argument list.
	argv := make([]reflect.Value, 0, numFixed)
	// Evaluate the explicit arguments (skipping the first arg which is the function reference).
	for i := 1; i < len(args); i++ {
		var paramType reflect.Type
		if !typ.IsVariadic() || i-1 < numIn-1 {
			paramType = typ.In(i - 1)
		} else {
			paramType = typ.In(numIn - 1).Elem()
		}
		arg, err := s.evalArg(dot, paramType, args[i])
		if err != nil {
			return missingVal, err
		}
		arg, err = s.validateArg(paramType, arg)
		if err != nil {
			return missingVal, err
		}
		argv = append(argv, arg)
	}
	// Add the piped (final) argument.
	if final.IsValid() {
		var paramType reflect.Type
		idx := len(argv)
		if !typ.IsVariadic() || idx < numIn-1 {
			if idx < numIn {
				paramType = typ.In(idx)
			}
		} else {
			paramType = typ.In(numIn - 1).Elem()
		}
		piped, err := s.validateArg(paramType, final)
		if err != nil {
			return missingVal, err
		}
		argv = append(argv, piped)
	}

	result, err := safeCall(fun, argv)
	if err != nil {
		return missingVal, s.errorf("error calling %s: %w", name, err)
	}
	if result.Type() == reflectValueType {
		// A function returning a reflect.Value, as index and slice do,
		// returns the value it holds, as in text/template.
		result = result.Interface().(reflect.Value)
	}
	return result, nil
}

func (s *state) evalArg(dot reflect.Value, typ reflect.Type, n Node) (reflect.Value, error) {
	switch arg := n.(type) {
	case *DotNode:
		return dot, nil
	case *NilNode:
		if typ != nil && canBeNil(typ) {
			return reflect.Zero(typ), nil
		}
		if typ != nil && typ != reflectValueType {
			return missingVal, s.errorf("cannot assign nil to %s", typ)
		}
		return reflect.ValueOf(nil), nil
	case *FieldNode:
		// zeroArgSentinel, not nil: see evalFieldChain.
		return s.evalFieldNode(dot, arg, zeroArgSentinel, missingVal)
	case *VariableNode:
		return s.evalVariableNode(dot, arg, zeroArgSentinel, missingVal)
	case *PipeNode:
		return s.evalPipeline(dot, arg)
	case *IdentifierNode:
		return s.evalFunction(dot, arg, nil, []Node{arg}, missingVal)
	case *ChainNode:
		return s.evalChainNode(dot, arg, zeroArgSentinel, missingVal)
	case *BoolNode:
		return reflect.ValueOf(arg.True), nil
	case *NumberNode:
		return s.idealConstant(arg)
	case *StringNode:
		return reflect.ValueOf(arg.Text), nil
	}
	return missingVal, s.errorf("can't handle %s for arg", n)
}

// printValue writes the textual representation of the value to the writer.
func (s *state) printValue(wr io.Writer, v reflect.Value) (int, error) {
	// A nil interface holds no value, as indirectInterface reports it.
	if !v.IsValid() || (v.Kind() == reflect.Interface && v.IsNil()) {
		return wr.Write([]byte("<no value>"))
	}
	iface := printableValue(v)
	if iface == nil {
		return 0, s.errorf("can't print %s of type %s", v, v.Type())
	}
	text := fmt.Sprint(iface)
	return wr.Write([]byte(text))
}

// printableValue returns what fmt.Sprint should print for v. A pointer is
// dereferenced, so that a nil one prints "<nil>", unless it implements
// error or fmt.Stringer: a method on the pointer receiver would be lost.
func printableValue(v reflect.Value) any {
	if v.Type().Implements(errorType) || v.Type().Implements(stringerType) {
		if v.CanInterface() {
			return v.Interface()
		}
	}
	if v.Kind() == reflect.Pointer {
		v, _ = indirect(v)
		if !v.IsValid() {
			return "<nil>"
		}
	}
	if v.CanInterface() {
		return v.Interface()
	}
	return nil
}

var stringerType = reflect.TypeFor[fmt.Stringer]()

// htmlActionText returns the text an action with the value v writes in
// HTML, before escaping, and v's trust. A nil interface writes nothing, as
// in html/template.
func (s *state) htmlActionText(v reflect.Value) (string, Trust, error) {
	if !v.IsValid() {
		return "<no value>", TrustNone, nil
	}
	if v.Kind() == reflect.Interface && v.IsNil() {
		return "", TrustNone, nil
	}
	iface := printableValue(v)
	if iface == nil {
		return "", TrustNone, s.errorf("can't print %s of type %s", v, v.Type())
	}
	return fmt.Sprint(iface), s.trustOf(v), nil
}

// Variable management

func (s *state) push(name string, value reflect.Value) {
	s.vars = append(s.vars, variable{name, value})
}

func (s *state) setVar(name string, value reflect.Value) {
	for i := len(s.vars) - 1; i >= 0; i-- {
		if s.vars[i].name == name {
			s.vars[i].value = value
			return
		}
	}
	s.vars = append(s.vars, variable{name, value})
}

func (s *state) setTopVar(n int, value reflect.Value) {
	s.vars[len(s.vars)-n].value = value
}

func (s *state) varValue(name string) reflect.Value {
	for i := len(s.vars) - 1; i >= 0; i-- {
		if s.vars[i].name == name {
			return s.vars[i].value
		}
	}
	return missingVal
}

// HTML walk methods

// walkHTMLNode executes one node and returns the html.Nodes it makes.
func (s *state) walkHTMLNode(dot reflect.Value, node Node) ([]*html.Node, error) {
	switch n := node.(type) {
	case *HTMLElementNode:
		el, err := s.walkHTMLElement(dot, n)
		if err != nil {
			if el != nil {
				return []*html.Node{el}, err
			}
			return nil, err
		}
		return []*html.Node{el}, nil
	case *HTMLCommentNode:
		// Comments are stripped to match html/template behavior.
		n.count.Add(1)
		return nil, nil
	case *HTMLDoctypeNode:
		n.count.Add(1)
		dn := &html.Node{
			Type: html.DoctypeNode,
			Data: "html",
		}
		return []*html.Node{dn}, nil
	case *TextNode:
		n.count.Add(1)
		nodeType := html.TextNode
		if n.Verbatim || s.markup != nil {
			nodeType = html.RawNode
		}
		tn := &html.Node{
			Type: nodeType,
			Data: s.writeText(string(n.Text)),
		}
		return []*html.Node{tn}, nil
	case *ActionNode:
		return s.walkHTMLAction(dot, n)
	case *IfNode:
		return s.walkHTMLIf(dot, n)
	case *RangeNode:
		return s.walkHTMLRange(dot, n)
	case *WithNode:
		return s.walkHTMLWith(dot, n)
	case *TemplateNode:
		return s.walkHTMLTemplate(dot, n)
	case *DefineNode:
		// Skip define nodes during execution.
		return nil, nil
	case *BlockNode:
		// A block executes as a call of the template it defines.
		return s.walkHTMLTemplate(dot, s.tree.newTemplate(n.Start(), n.End(), n.Line, n.Name, n.Pipe))
	case *BreakNode:
		n.count.Add(1)
		return nil, breakError{}
	case *ContinueNode:
		n.count.Add(1)
		return nil, continueError{}
	case *CommentNode:
		// A template comment writes nothing.
		return nil, nil
	default:
		return nil, s.errorf("unsupported HTML node type: %T", node)
	}
}

func (s *state) walkHTMLElement(dot reflect.Value, elem *HTMLElementNode) (*html.Node, error) {
	if at := s.tagAt(); at != nil && at.part != tagNone {
		return nil, s.errorf("<%s> in a tag that htt cannot follow", elem.TagName)
	}
	elem.openCount.Add(1)
	n := &html.Node{
		Type:     html.ElementNode,
		Data:     elem.TagName,
		DataAtom: htmlAtom(elem.TagName),
	}
	// Process attributes.
	var attrs []sourceAttr
	for _, attrNode := range elem.Attr {
		switch a := attrNode.(type) {
		case *HTMLAttributeNode:
			a.count.Add(1)
			attr, err := s.evalHTMLAttribute(dot, a)
			if err != nil {
				return nil, err
			}
			attrs = append(attrs, attr)
		case *ActionNode:
			attr, ok, err := s.evalAttrNameAction(dot, a)
			if err != nil {
				return nil, err
			}
			if ok {
				attrs = append(attrs, attr)
			}
		case *IfNode:
			branch, err := s.walkHTMLAttrBranch(dot, &a.BranchNode, "if")
			if err != nil {
				return nil, err
			}
			attrs = append(attrs, branch...)
		case *RangeNode:
			each, err := s.walkHTMLAttrRange(dot, a)
			if err != nil {
				return nil, err
			}
			attrs = append(attrs, each...)
		case *WithNode:
			branch, err := s.walkHTMLAttrBranch(dot, &a.BranchNode, "with")
			if err != nil {
				return nil, err
			}
			attrs = append(attrs, branch...)
		case *TemplateNode:
			return nil, errTemplateInTag(s, a)
		default:
			return nil, s.errorf("unsupported attribute node type: %T", attrNode)
		}
	}
	for _, a := range attrs {
		n.Attr = append(n.Attr, a.Attribute)
	}
	s.markup.setAttrs(n, attrs)
	elem.attrCount.Add(1)
	// Process children.
	rawText, escapableText := s.rawText, s.escapableText
	s.rawText = rawText || isRawTextElement(strings.ToLower(elem.TagName))
	s.escapableText = escapableText || holdsEscapableText(elem.TagName)
	nodes, err := s.walkHTMLNodes(dot, elem.Children, false, false)
	s.rawText, s.escapableText = rawText, escapableText
	for _, cn := range nodes {
		n.AppendChild(cn)
	}
	if err != nil {
		if isBreak(err) || isContinue(err) {
			// Preserve the partially-built element. The caller
			// (walkHTMLList) collects nodes before propagating.
			return n, err
		}
		return nil, err
	}
	elem.closeCount.Add(1)
	if !elem.Closed && !elem.Void && !elem.Misnested && !changesTextContext(elem.TagName) {
		// html/template writes no end tag the source does not, as when
		// one template opens <main> and another closes it.
		s.markup.leaveOpen(n)
	} else if at := s.tagAt(); at != nil && at.part != tagNone {
		return nil, s.errorf("</%s> in a tag that htt cannot follow", elem.TagName)
	}
	return n, nil
}

// evalAttrNameAction evaluates an action written where an attribute is, as
// in <p {{.Name}}>. Its output is filtered as an attribute name. It reports
// false for a variable declaration, which writes nothing.
func (s *state) evalAttrNameAction(dot reflect.Value, action *ActionNode) (sourceAttr, bool, error) {
	action.count.Add(1)
	val, err := s.evalPipeline(dot, action.Pipe)
	if err != nil {
		return sourceAttr{}, false, err
	}
	if len(action.Pipe.Decl) > 0 {
		return sourceAttr{}, false, nil
	}
	text, trust, err := s.htmlActionText(val)
	if err != nil {
		return sourceAttr{}, false, err
	}
	return sourceAttr{Attribute: html.Attribute{Key: htmlNameFilter(text, trust)}}, true, nil
}

func (s *state) evalHTMLAttribute(dot reflect.Value, attr *HTMLAttributeNode) (sourceAttr, error) {
	ns := attr.Namespace

	// An action in the name would let data pick the attribute, onclick
	// say, past the escapers that go by the name.
	for _, k := range attr.Key {
		if _, ok := k.(*TextNode); !ok {
			return sourceAttr{}, s.errorf("dynamic attribute names are not permitted (template action found in attribute name)")
		}
	}

	var keyBuf bytes.Buffer
	for _, k := range attr.Key {
		if tn, ok := k.(*TextNode); ok {
			keyBuf.Write(tn.Text)
		}
	}

	key := keyBuf.String()
	if err := validateAttrName(key); err != nil {
		return sourceAttr{}, s.errorf("invalid attribute name %q: %v", key, err)
	}

	if !attr.HasValue {
		return sourceAttr{Attribute: html.Attribute{Namespace: ns, Key: key}}, nil
	}

	// The value's first text says whether it is quoted, and with what.
	var quote byte
	if len(attr.Value) > 0 {
		if tn, ok := attr.Value[0].(*TextNode); ok && len(tn.Text) > 0 && (tn.Text[0] == '"' || tn.Text[0] == '\'') {
			quote = tn.Text[0]
		}
	}
	var val, raw strings.Builder
	if err := s.evalHTMLAttrValueNodes(dot, attr.Value, quote, &val, &raw); err != nil {
		return sourceAttr{}, err
	}
	valStr, rawStr := val.String(), raw.String()
	if quote != 0 {
		valStr, rawStr = unquoteAttr(valStr, quote), unquoteAttr(rawStr, quote)
	}
	return sourceAttr{
		Attribute: html.Attribute{Namespace: ns, Key: key, Val: valStr},
		syntax:    &attrSyntax{quote: quote, raw: rawStr},
	}, nil
}

// unquoteAttr removes the quotes around an attribute value's text.
func unquoteAttr(s string, quote byte) string {
	s = strings.TrimPrefix(s, string(quote))
	return strings.TrimSuffix(s, string(quote))
}

// evalHTMLAttrValueNodes evaluates the nodes of an attribute value. It
// writes the value to val and, to raw, the value as html/template writes
// it between quote (0 for none).
func (s *state) evalHTMLAttrValueNodes(dot reflect.Value, nodes []Node, quote byte, val, raw *strings.Builder) error {
	for _, v := range nodes {
		switch vn := v.(type) {
		case *TextNode:
			val.Write(vn.Text)
			raw.Write(vn.Text)
		case *ActionNode:
			value, err := s.evalPipeline(dot, vn.Pipe)
			if err != nil {
				return err
			}
			if !value.IsValid() {
				val.WriteString("<no value>")
				raw.WriteString(escapeAttrText("<no value>", quote))
				continue
			}
			text, trust, err := s.htmlActionText(value)
			if err != nil {
				return err
			}
			text, _ = escapeAction(text, vn.HTMLContext, trust)
			val.WriteString(text)
			raw.WriteString(escapeAttrText(text, quote))
		case *IfNode, *WithNode, *RangeNode:
			outer := s.attrSource
			s.attrSource = make(map[*html.Node]bool)
			htmlNodes, err := s.walkHTMLNode(dot, vn)
			source := s.attrSource
			s.attrSource = outer
			if err != nil {
				return err
			}
			for _, hn := range htmlNodes {
				val.WriteString(htmlNodeText(hn))
				raw.WriteString(rawAttrText(hn, quote, source))
			}
		}
	}
	return nil
}

func (s *state) walkHTMLAction(dot reflect.Value, action *ActionNode) ([]*html.Node, error) {
	action.count.Add(1)
	val, err := s.evalPipeline(dot, action.Pipe)
	if err != nil {
		return nil, err
	}
	if len(action.Pipe.Decl) > 0 {
		return nil, nil
	}
	if at := s.tagAt(); at != nil && at.part != tagNone {
		return s.walkHTMLTagAction(val, at)
	}
	if !val.IsValid() {
		return []*html.Node{{
			Type: html.TextNode,
			Data: "<no value>",
		}}, nil
	}
	inScript := action.HTMLContext != nil && action.HTMLContext.State == "text" && action.HTMLContext.Element == "script"
	if val.Kind() == reflect.Interface && val.IsNil() && !inScript {
		// html/template skips a nil interface.
		if s.attrSource != nil {
			return []*html.Node{{Type: html.TextNode}}, nil
		}
		return nil, nil
	}
	iface := printableValue(val)
	if iface == nil {
		return nil, s.errorf("can't print %s of type %s", val, val.Type())
	}
	trust := s.trustOf(val)
	// In a script expression a value that is not a string is written as
	// JSON, as html/template's jsValEscaper writes it. A value trusted as
	// JS is left to escapeAction.
	if action.HTMLContext != nil && action.HTMLContext.State == "text" &&
		action.HTMLContext.Element == "script" && action.HTMLContext.JSContext == "expr" &&
		trust != TrustJS && trust != TrustJSStr {
		if text, ok := jsExprJSONMarshal(iface); ok {
			return []*html.Node{{Type: html.RawNode, Data: text}}, nil
		}
	}
	text := fmt.Sprint(iface)
	text, raw := escapeAction(text, action.HTMLContext, trust)
	if s.escapableText && !s.rawText {
		// In textarea and title even trusted HTML is text, and is
		// escaped when rendered, as html/template escapes it.
		raw = false
	}
	if text == "" {
		if s.attrSource != nil {
			// In an attribute value an action that writes nothing is
			// still written: an unquoted value must not end there.
			return []*html.Node{{Type: html.TextNode}}, nil
		}
		return nil, nil
	}
	nodeType := html.TextNode
	if raw {
		nodeType = html.RawNode
	}
	return []*html.Node{{
		Type: nodeType,
		Data: text,
	}}, nil
}

// walkHTMLTagAction writes the value of an action in a tag the parser left
// as text, escaped for where scan says the action stands: an attribute
// name or a value.
func (s *state) walkHTMLTagAction(val reflect.Value, scan *tagScan) ([]*html.Node, error) {
	text, trust, err := s.htmlActionText(val)
	if err != nil {
		return nil, err
	}
	var at tagScan
	at, *scan = scan.action()
	switch at.part {
	case tagAttrName:
		text = htmlNameFilter(text, trust)
	case tagValue:
		ctx := &HTMLContext{State: "attr", Element: at.element, Attr: valueAttr(at.attr)}
		text = escapeAttrText(escapeAttrValue(text, ctx, trust), at.quote)
	default:
		return nil, s.errorf("an action in a tag that htt cannot follow")
	}
	return []*html.Node{{Type: html.RawNode, Data: text}}, nil
}

func (s *state) walkHTMLIf(dot reflect.Value, n *IfNode) ([]*html.Node, error) {
	return s.walkHTMLBranch(dot, &n.BranchNode, "if")
}

func (s *state) walkHTMLWith(dot reflect.Value, n *WithNode) ([]*html.Node, error) {
	return s.walkHTMLBranch(dot, &n.BranchNode, "with")
}

func (s *state) walkHTMLBranch(dot reflect.Value, branch *BranchNode, name string) ([]*html.Node, error) {
	branch.openCount.Add(1)
	mark := len(s.vars)
	defer func() { s.vars = s.vars[:mark] }()
	pipe, err := s.evalPipeline(dot, branch.Pipe)
	if err != nil {
		return nil, err
	}
	truth, ok := isTrue(indirectInterface(pipe))
	if !ok {
		return nil, s.errorf("if/with can't use %v", pipe)
	}
	var nodes []*html.Node
	if truth {
		newDot := dot
		if name == "with" {
			newDot = pipe
		}
		lastTrim := bodyLastTrim(branch)
		nodes, err = s.walkHTMLList(newDot, branch.List, branch.StartTrimRight, lastTrim)
	} else if branch.ElseList != nil {
		branch.elseCount.Add(1)
		nodes, err = s.walkHTMLList(dot, branch.ElseList, branch.ElseTrimRight, branch.EndTrimLeft)
	}
	if err != nil {
		return nodes, err
	}
	branch.endCount.Add(1)
	return nodes, nil
}

func (s *state) walkHTMLRange(dot reflect.Value, r *RangeNode) ([]*html.Node, error) {
	r.BranchNode.openCount.Add(1)
	val, err := s.evalPipeline(dot, r.Pipe)
	if err != nil {
		return nil, err
	}
	val, _ = indirect(val)
	mark := len(s.vars)
	var result []*html.Node
	ran, err := s.rangeOver(val, len(r.Pipe.Decl), func(index, elem reflect.Value) (bool, error) {
		s.setRangeVars(r.Pipe, index, elem)
		lastTrim := bodyLastTrim(&r.BranchNode)
		nodes, err := s.walkHTMLList(elem, r.List, r.StartTrimRight, lastTrim)
		s.vars = s.vars[:mark]
		result = append(result, nodes...)
		if isBreak(err) {
			return true, nil
		}
		if isContinue(err) {
			return false, nil
		}
		return false, err
	})
	if err != nil {
		return nil, err
	}
	s.vars = s.vars[:mark]
	if !ran && r.ElseList != nil {
		r.BranchNode.elseCount.Add(1)
		nodes, walkErr := s.walkHTMLList(dot, r.ElseList, r.ElseTrimRight, r.EndTrimLeft)
		if walkErr != nil {
			return nodes, walkErr
		}
		result = nodes
	}
	r.BranchNode.endCount.Add(1)
	return result, nil
}

func (s *state) walkHTMLTemplate(dot reflect.Value, t *TemplateNode) ([]*html.Node, error) {
	t.count.Add(1)
	if s.depth >= maxExecDepth {
		return nil, s.errorf("exceeded maximum template depth (%d)", maxExecDepth)
	}
	if s.finder == nil {
		return nil, s.errorf("template %q not defined", t.Name)
	}
	tree, ok := s.finder.FindTree(t.Name)
	if !ok {
		return nil, s.errorf("template %q not defined", t.Name)
	}
	newDot := dot
	if t.Pipe != nil {
		var err error
		newDot, err = s.evalPipeline(dot, t.Pipe)
		if err != nil {
			return nil, err
		}
	}
	newState := &state{
		tree:       tree,
		cp:         s.cp,
		depth:      s.depth + 1,
		funcs:      s.funcs,
		finder:     s.finder,
		missingKey: s.missingKey,
		trust:      s.trust,
		markup:     s.markup,
		vars:       []variable{{"$", newDot}},
		// The called template's text carries on where this one's
		// stands; in an attribute value it is escaped whole.
		scan:          s.tagAt(),
		rawText:       s.rawText || s.attrSource != nil,
		escapableText: s.escapableText,
	}
	s.cp.AddTree(tree)
	root := tree.Root
	if root == nil {
		return nil, nil
	}
	if len(root.Nodes) == 1 {
		if frag, ok := root.Nodes[0].(*HTMLDocumentFragmentNode); ok {
			return newState.walkHTMLNodes(newDot, frag.Children, false, false)
		}
	}
	// The tree of a define or block has no fragment, and still needs
	// contextual escaping.
	if tree.HTMLMode || rootHasHTMLNodes(root) {
		return newState.walkHTMLList(newDot, root, tree.RootTrimFirst, tree.RootTrimLast)
	}
	// A text tree: its output is one text node.
	var buf bytes.Buffer
	newState.wr = &buf
	if err := newState.walk(newDot, root); err != nil {
		return nil, err
	}
	if buf.Len() == 0 {
		return nil, nil
	}
	return []*html.Node{{
		Type: html.TextNode,
		Data: buf.String(),
	}}, nil
}

func (s *state) walkHTMLList(dot reflect.Value, list *ListNode, trimFirst, trimLast bool) ([]*html.Node, error) {
	return s.walkHTMLNodes(dot, list.Nodes, trimFirst, trimLast)
}

// walkHTMLNodes walks sibling nodes, trimming text where a neighbor's trim
// marker, or trimFirst and trimLast at the ends, says to. On a break or
// continue it returns the nodes built so far with the error.
func (s *state) walkHTMLNodes(dot reflect.Value, nodes []Node, trimFirst, trimLast bool) ([]*html.Node, error) {
	var result []*html.Node
	for i, node := range nodes {
		if tn, ok := node.(*TextNode); ok {
			text := string(tn.Text)
			if (i == 0 && trimFirst) || (i > 0 && nodeRightTrim(nodes[i-1])) {
				text = strings.TrimLeft(text, trimCutset)
			}
			if (i == len(nodes)-1 && trimLast) || (i < len(nodes)-1 && nodeLeftTrim(nodes[i+1])) {
				text = strings.TrimRight(text, trimCutset)
			}
			tn.count.Add(1)
			if len(text) > 0 {
				nodeType := html.TextNode
				if tn.Verbatim || s.markup != nil {
					// The template's own text is written as it is,
					// entities and all.
					nodeType = html.RawNode
				}
				hn := &html.Node{
					Type: nodeType,
					Data: s.writeText(text),
				}
				if s.attrSource != nil {
					s.attrSource[hn] = true
				}
				result = append(result, hn)
			}
			continue
		}
		htmlNodes, err := s.walkHTMLNode(dot, node)
		if err != nil {
			if isBreak(err) || isContinue(err) {
				result = append(result, htmlNodes...)
				return result, err
			}
			return nil, err
		}
		result = append(result, htmlNodes...)
	}
	return result, nil
}

// walkHTMLAttrBranch executes an if or with written among an element's
// attributes.
func (s *state) walkHTMLAttrBranch(dot reflect.Value, branch *BranchNode, name string) ([]sourceAttr, error) {
	branch.openCount.Add(1)
	mark := len(s.vars)
	defer func() { s.vars = s.vars[:mark] }()
	pipe, err := s.evalPipeline(dot, branch.Pipe)
	if err != nil {
		return nil, err
	}
	truth, ok := isTrue(indirectInterface(pipe))
	if !ok {
		return nil, s.errorf("if/with can't use %v", pipe)
	}
	var attrs []sourceAttr
	if truth {
		newDot := dot
		if name == "with" {
			newDot = pipe
		}
		attrs, err = s.walkHTMLAttrList(newDot, branch.List)
	} else if branch.ElseList != nil {
		branch.elseCount.Add(1)
		attrs, err = s.walkHTMLAttrList(dot, branch.ElseList)
	}
	if err != nil {
		return nil, err
	}
	branch.endCount.Add(1)
	return attrs, nil
}

func (s *state) walkHTMLAttrRange(dot reflect.Value, r *RangeNode) ([]sourceAttr, error) {
	r.BranchNode.openCount.Add(1)
	val, err := s.evalPipeline(dot, r.Pipe)
	if err != nil {
		return nil, err
	}
	val, _ = indirect(val)
	mark := len(s.vars)
	var result []sourceAttr
	ran, err := s.rangeOver(val, len(r.Pipe.Decl), func(index, elem reflect.Value) (bool, error) {
		s.setRangeVars(r.Pipe, index, elem)
		attrs, err := s.walkHTMLAttrList(elem, r.List)
		s.vars = s.vars[:mark]
		if err != nil {
			return false, err
		}
		result = append(result, attrs...)
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	s.vars = s.vars[:mark]
	if !ran && r.ElseList != nil {
		r.BranchNode.elseCount.Add(1)
		attrs, walkErr := s.walkHTMLAttrList(dot, r.ElseList)
		if walkErr != nil {
			return nil, walkErr
		}
		result = attrs
	}
	r.BranchNode.endCount.Add(1)
	return result, nil
}

func (s *state) walkHTMLAttrList(dot reflect.Value, list *ListNode) ([]sourceAttr, error) {
	var result []sourceAttr
	for _, node := range list.Nodes {
		switch n := node.(type) {
		case *HTMLAttributeNode:
			n.count.Add(1)
			attr, err := s.evalHTMLAttribute(dot, n)
			if err != nil {
				return nil, err
			}
			result = append(result, attr)
		case *IfNode:
			attrs, err := s.walkHTMLAttrBranch(dot, &n.BranchNode, "if")
			if err != nil {
				return nil, err
			}
			result = append(result, attrs...)
		case *WithNode:
			attrs, err := s.walkHTMLAttrBranch(dot, &n.BranchNode, "with")
			if err != nil {
				return nil, err
			}
			result = append(result, attrs...)
		case *RangeNode:
			attrs, err := s.walkHTMLAttrRange(dot, n)
			if err != nil {
				return nil, err
			}
			result = append(result, attrs...)
		case *ActionNode:
			attr, ok, err := s.evalAttrNameAction(dot, n)
			if err != nil {
				return nil, err
			}
			if ok {
				result = append(result, attr)
			}
		case *TemplateNode:
			return nil, errTemplateInTag(s, n)
		case *TextNode:
			// Whitespace between attributes — ignore.
		}
	}
	return result, nil
}

// htmlNodeText returns the text of n and its descendants.
func htmlNodeText(n *html.Node) string {
	if n.Type == html.TextNode || n.Type == html.RawNode {
		return n.Data
	}
	var buf strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		buf.WriteString(htmlNodeText(c))
	}
	return buf.String()
}

// errTemplateInTag is the error for a {{template}} call among a tag's
// attributes: the called tree was parsed as content, not as attributes.
func errTemplateInTag(s *state, n *TemplateNode) error {
	return s.errorf("{{template %q}} in a tag is not supported by htt", n.Name)
}

// rawAttrText returns the text of a node from an attribute value as
// html/template writes it between quote. The template's own text, which
// source holds, is written as it is. All else is escaped whatever the
// node's type: a trusted value is a raw node too.
func rawAttrText(n *html.Node, quote byte, source map[*html.Node]bool) string {
	if source[n] {
		return n.Data
	}
	switch n.Type {
	case html.TextNode, html.RawNode:
		return escapeAttrText(n.Data, quote)
	}
	var buf strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		buf.WriteString(rawAttrText(c, quote, source))
	}
	return buf.String()
}

// validateAttrName rejects a name that holds space, a quote, '>', '/', '='
// or a control character.
func validateAttrName(name string) error {
	if name == "" {
		return fmt.Errorf("empty attribute name")
	}
	for _, r := range name {
		switch {
		case r == ' ', r == '\t', r == '\n', r == '\f', r == '\r':
			return fmt.Errorf("contains whitespace")
		case r == '"', r == '\'', r == '>', r == '/', r == '=':
			return fmt.Errorf("contains %q", r)
		case r <= 0x1f || r == 0x7f:
			return fmt.Errorf("contains control character")
		}
	}
	return nil
}

// Error helpers

func (s *state) errorf(format string, args ...any) ExecError {
	name := s.tree.Name
	return errorf(name, format, args...)
}

func (s *state) writeError(err error) ExecError {
	return s.errorf("write error: %w", err)
}

var reflectValueType = reflect.TypeFor[reflect.Value]()

// validateArg prepares arg for a parameter of paramType: wrapped for a
// reflect.Value parameter, taken out of its interface otherwise. As
// text/template does, a pointer whose element is assignable to the
// parameter is dereferenced, so that a *T can be passed to a func(T).
func (s *state) validateArg(paramType reflect.Type, arg reflect.Value) (reflect.Value, error) {
	if paramType == reflectValueType {
		return reflect.ValueOf(arg), nil
	}
	if !arg.IsValid() {
		if paramType != nil && canBeNil(paramType) {
			return reflect.Zero(paramType), nil
		}
		return arg, nil
	}
	if arg.Kind() == reflect.Interface && !arg.IsNil() {
		arg = arg.Elem()
	}
	if paramType != nil && arg.Kind() == reflect.Pointer && !arg.Type().AssignableTo(paramType) && arg.Type().Elem().AssignableTo(paramType) {
		if arg.IsNil() {
			return missingVal, s.errorf("dereference of nil pointer of type %s", arg.Type())
		}
		arg = arg.Elem()
	}
	return arg, nil
}

// missingVal is the zero reflect.Value used to signal "missing".
var missingVal reflect.Value

// Sentinel errors for break/continue in range loops.
type (
	breakError    struct{}
	continueError struct{}
)

func (breakError) Error() string    { return "break" }
func (continueError) Error() string { return "continue" }

func isBreak(err error) bool {
	_, ok := err.(breakError)
	return ok
}

func isContinue(err error) bool {
	_, ok := err.(continueError)
	return ok
}
