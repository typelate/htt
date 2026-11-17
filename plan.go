package htt

import (
	"io"
)

// Plan carries the edits an escaper makes to a text tree.
type Plan interface {
	// Text returns the text to write for n, already trimmed, or false to write n as it is.
	Text(instance string, n *TextNode) ([]byte, bool)
	// Action returns how to write the value of n, or false to print it as it is.
	Action(instance string, n *ActionNode) (ActionEdit, bool)
	// Call returns the instance the template called at n runs as, or "" for the callee's own name.
	Call(instance string, n Node) string
}

// ActionEdit pipes an action's value through Funcs in order and writes the result.
type ActionEdit struct {
	// Drop is the number of commands at the end of the pipeline that are not run.
	Drop int
	// EvalArgs passes the arguments of a one-command pipeline to Funcs' first function instead of running the command.
	EvalArgs bool
	// Funcs receive the value's Interface() first, then the previous result.
	Funcs []func(args ...any) string
}

// WithPlan sets the plan of a text tree and the instance its root runs as.
func WithPlan(p Plan, instance string) Option {
	return func(c *config) {
		c.plan = p
		c.instance = instance
	}
}

// replaceText returns the plan's replacement for n, or false when there is none.
func (s *state) replaceText(n *TextNode) ([]byte, bool) {
	if s.plan == nil {
		return nil, false
	}
	return s.plan.Text(s.instance, n)
}

// writeEdit passes args to the first function of edit, pipes the result through the rest, and writes it.
func (s *state) writeEdit(args []any, edit ActionEdit) error {
	var text string
	for i, fn := range edit.Funcs {
		if i == 0 {
			text = fn(args...)
		} else {
			text = fn(text)
		}
	}
	if _, err := io.WriteString(s.wr, text); err != nil {
		return s.writeError(err)
	}
	return nil
}
