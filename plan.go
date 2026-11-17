package htt

import (
	"io"
	"reflect"
)

// Plan carries the edits an escaper makes to a text tree.
type Plan interface {
	// Action returns how to write the value of n, or false to print it as it is.
	Action(instance string, n *ActionNode) (ActionEdit, bool)
}

// ActionEdit pipes an action's value through Funcs in order and writes the result.
type ActionEdit struct {
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

// writeEdit pipes val through the functions of edit and writes the result.
func (s *state) writeEdit(val reflect.Value, edit ActionEdit) error {
	var arg any
	if val.IsValid() {
		arg = val.Interface()
	}
	var text string
	for i, fn := range edit.Funcs {
		if i == 0 {
			text = fn(arg)
		} else {
			text = fn(text)
		}
	}
	if _, err := io.WriteString(s.wr, text); err != nil {
		return s.writeError(err)
	}
	return nil
}
