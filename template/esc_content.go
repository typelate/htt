// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Copied from html/template/content.go and html/template/error.go of go1.27.3, with the changes marked below.

package template

import (
	"fmt"
	"reflect"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
)

// Changed: the content types (CSS, HTML, HTMLAttr, JS, JSStr, URL, Srcset)
// are declared in content.go and are not repeated here.

type contentType uint8

const (
	contentTypePlain contentType = iota
	contentTypeCSS
	contentTypeHTML
	contentTypeHTMLAttr
	contentTypeJS
	contentTypeJSStr
	contentTypeURL
	contentTypeSrcset
	// contentTypeUnsafe is used in attr.go for values that affect how
	// embedded content and network messages are formed, vetted,
	// or interpreted; or which credentials network messages carry.
	contentTypeUnsafe
)

// indirect returns the value, after dereferencing as many times
// as necessary to reach the base type (or nil).
func indirect(a any) any {
	if a == nil {
		return nil
	}
	if t := reflect.TypeOf(a); t.Kind() != reflect.Pointer {
		// Avoid creating a reflect.Value if it's not a pointer.
		return a
	}
	v := reflect.ValueOf(a)
	for v.Kind() == reflect.Pointer && !v.IsNil() {
		v = v.Elem()
	}
	return v.Interface()
}

var (
	errorType       = reflect.TypeFor[error]()
	fmtStringerType = reflect.TypeFor[fmt.Stringer]()
)

// indirectToStringerOrError returns the value, after dereferencing as many times
// as necessary to reach the base type (or nil) or an implementation of fmt.Stringer
// or error.
func indirectToStringerOrError(a any) any {
	if a == nil {
		return nil
	}
	v := reflect.ValueOf(a)
	for !v.Type().Implements(fmtStringerType) && !v.Type().Implements(errorType) && v.Kind() == reflect.Pointer && !v.IsNil() {
		v = v.Elem()
	}
	return v.Interface()
}

// stringify converts its arguments to a string and the type of the content.
// All pointers are dereferenced, as in the text/template package.
func stringify(args ...any) (string, contentType) {
	if len(args) == 1 {
		switch s := indirect(args[0]).(type) {
		case string:
			return s, contentTypePlain
		case CSS:
			return string(s), contentTypeCSS
		case HTML:
			return string(s), contentTypeHTML
		case HTMLAttr:
			return string(s), contentTypeHTMLAttr
		case JS:
			return string(s), contentTypeJS
		case JSStr:
			return string(s), contentTypeJSStr
		case URL:
			return string(s), contentTypeURL
		case Srcset:
			return string(s), contentTypeSrcset
		}
	}
	i := 0
	for _, arg := range args {
		// We skip untyped nil arguments for backward compatibility.
		// Without this they would be output as <nil>, escaped.
		// See issue 25875.
		if arg == nil {
			continue
		}

		args[i] = indirectToStringerOrError(arg)
		i++
	}
	return fmt.Sprint(args[:i]...), contentTypePlain
}

// errorf creates an error given a format string f and args.
// The template Name still needs to be supplied.
// Changed: node is an htt.Node (was parse.Node).
func errorf(k ErrorCode, node htt.Node, line int, f string, args ...any) *Error {
	return &Error{k, node, "", line, fmt.Sprintf(f, args...), ""}
}

// locate records where in t the error's node starts, the way
// (*parse.Tree).ErrorContext reports it: ParseName, the 1-based line and the
// 0-based column. An earlier location is kept.
// Changed: new. html/template computes this in Error; here the tree is at
// hand only while it is escaped, so it is computed then.
func (e *Error) locate(t *htt.Tree, pos lex.Pos) {
	if e.location != "" {
		return
	}
	p := t.PositionAt(pos)
	e.location = fmt.Sprintf("%s:%d:%d", t.ParseName, p.Line, p.Column-1)
}

// nodeStart returns the offset that text/template records for node, which is
// where html/template locates an error on it. It returns false for a node
// whose offset htt does not record.
// Changed: new.
func nodeStart(node interface{ String() string }) (lex.Pos, bool) {
	switch n := node.(type) {
	case *htt.ActionNode:
		return pipeStart(n.Pipe)
	case *htt.IfNode:
		return pipeStart(n.Pipe)
	case *htt.RangeNode:
		return pipeStart(n.Pipe)
	case *htt.WithNode:
		return pipeStart(n.Pipe)
	case *htt.BreakNode:
		return n.KeywordStart(), true
	case *htt.ContinueNode:
		return n.KeywordStart(), true
	case *htt.TemplateNode:
		return n.NameStart(), true
	}
	return 0, false
}

// pipeStart returns the start of a branch's or action's pipeline, which is
// where text/template records a branch or action.
func pipeStart(pipe *htt.PipeNode) (lex.Pos, bool) {
	if pipe == nil {
		return 0, false
	}
	return pipe.Start(), true
}
