package htt

import (
	"fmt"
	"strings"
	texttemplate "text/template"

	"github.com/typelate/htt/lex"
)

// ExecError is the error returned when a template fails while it executes.
// It is text/template's type, so a caller can handle the errors of both
// packages alike. Its text names the template, the position and the text of
// the node being evaluated, and the message.
type ExecError = texttemplate.ExecError

// errorf returns an ExecError for a failure while node n of tree t was
// evaluated, worded as text/template words it:
// "template: LOCATION: executing NAME at <CONTEXT>: MESSAGE", or
// "template: NAME: MESSAGE" when n is nil.
func errorf(t *Tree, n Node, format string, args ...any) ExecError {
	return namedErrorf(t.Name, t, n, format, args...)
}

// namedErrorf is errorf for a tree running under name, which is not always
// the tree's own: text/template names a template in an error as it was
// executed or called.
func namedErrorf(name string, t *Tree, n Node, format string, args ...any) ExecError {
	escaped := doublePercent(name)
	if n == nil {
		format = fmt.Sprintf("template: %s: %s", escaped, format)
	} else {
		location, context := errorContext(t, n)
		format = fmt.Sprintf("template: %s: executing %q at <%s>: %s", location, escaped, doublePercent(context), format)
	}
	return ExecError{
		Name: name,
		Err:  fmt.Errorf(format, args...),
	}
}

// doublePercent returns the string with %'s replaced by %%, so it can be
// used safely inside a Printf format string.
func doublePercent(str string) string {
	return strings.ReplaceAll(str, "%", "%%")
}

// errorContext mirrors (*parse.Tree).ErrorContext: the location of n as
// ParseName:line:column, with a 1-based line and a 0-based column, and the
// text of n. t stands in for a node that has no tree of its own.
func errorContext(t *Tree, n Node) (location, context string) {
	pos := int(execPosition(n))
	tree := n.tree()
	if tree == nil {
		tree = t
	}
	text := tree.text[:min(max(pos, 0), len(tree.text))]
	byteNum := strings.LastIndex(text, "\n")
	if byteNum == -1 {
		byteNum = len(text) // On first line.
	} else {
		byteNum++ // After the newline.
		byteNum = len(text) - byteNum
	}
	lineNum := 1 + strings.Count(text, "\n")
	return fmt.Sprintf("%s:%d:%d", tree.ParseName, lineNum, byteNum), errorContextText(n)
}

// errorContextText returns n as text/template's String writes it: one space
// between words, no trim markers, whatever the source had. A node whose
// String is the same text in both packages is not rewritten. The nodes'
// own String is the source span and stays so.
func errorContextText(n Node) string {
	switch n.(type) {
	case *PipeNode, *ActionNode, *CommandNode, *TemplateNode:
		var sb strings.Builder
		n.writeTo(&sb)
		return sb.String()
	}
	return n.String()
}

// execPosition returns the offset text/template records for n, where it
// reports an execution error on n. It is not always n.Start(): text/template
// records the first token of the pipeline for an action or a branch, the
// name for a template call, the keyword for break and continue, and for a
// chain of fields the first field after the term.
func execPosition(n Node) lex.Pos {
	switch n := n.(type) {
	case *ActionNode:
		return n.Pipe.Start()
	case *IfNode:
		return n.Pipe.Start()
	case *RangeNode:
		return n.Pipe.Start()
	case *WithNode:
		return n.Pipe.Start()
	case *BreakNode:
		return n.KeywordStart()
	case *ContinueNode:
		return n.KeywordStart()
	case *TemplateNode:
		return n.NameStart()
	case *BlockNode:
		return n.Call().NameStart()
	case *FieldNode:
		if len(n.Ident) > 1 {
			return n.start + 1 + lex.Pos(len(n.Ident[0]))
		}
	case *VariableNode:
		if len(n.Ident) > 1 {
			return n.start + lex.Pos(len(n.Ident[0]))
		}
	case *ChainNode:
		first := n.end
		for _, f := range n.Field {
			first -= 1 + lex.Pos(len(f))
		}
		return first
	}
	return n.Start()
}
