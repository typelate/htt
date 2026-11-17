package template

import (
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/typelate/htt"
)

// errUnsupported marks a template the escaper does not take yet; execute
// then runs the old engine. It goes when the escaper takes every template.
var errUnsupported = errors.New("escaper: context not supported")

// planKey names a node as it runs in one instance of its template: the
// template name mangled with its start context.
type planKey struct {
	instance string
	node     htt.Node
}

// escapePlan implements htt.Plan with the edits an escaper recorded. It is
// not changed once made.
type escapePlan struct {
	actions map[planKey]htt.ActionEdit
}

// Text writes every text node as it is: no rewrite is planned yet.
func (p *escapePlan) Text(string, *htt.TextNode) ([]byte, bool) {
	return nil, false
}

// Action returns the edit recorded for n in instance.
func (p *escapePlan) Action(instance string, n *htt.ActionNode) (htt.ActionEdit, bool) {
	edit, ok := p.actions[planKey{instance, n}]
	return edit, ok
}

// Call names no instance: no call is planned yet.
func (p *escapePlan) Call(string, htt.Node) string {
	return ""
}

// commit makes the plan of the edits e recorded.
func (e *escaper) commit() *escapePlan {
	p := &escapePlan{actions: make(map[planKey]htt.ActionEdit, len(e.actionNodeEdits))}
	for k, escapers := range e.actionNodeEdits {
		p.actions[k] = planPipeline(k.node.(*htt.ActionNode).Pipe, escapers)
	}
	return p
}

// planPipeline is html/template's ensurePipelineContains as an edit: the
// value of the pipeline is piped through the escapers, in order.
// A pipeline that names a predefined escaper is not planned yet, so p is
// not read.
func planPipeline(_ *htt.PipeNode, escapers []string) htt.ActionEdit {
	edit := htt.ActionEdit{Funcs: make([]func(args ...any) string, 0, len(escapers))}
	for _, name := range escapers {
		edit.Funcs = append(edit.Funcs, funcMap[name])
	}
	if len(edit.Funcs) > 0 {
		// html/template's types are trusted as this package's are.
		first := edit.Funcs[0]
		edit.Funcs[0] = func(args ...any) string { return first(localContent(args)...) }
	}
	return edit
}

// localContent returns args with a value of one of html/template's
// safe-content types as this package's type of the same name. This package
// cannot import html/template, so it knows them by name, as trustOf does.
func localContent(args []any) []any {
	if len(args) != 1 {
		return args
	}
	v := reflect.ValueOf(args[0])
	for v.IsValid() && v.Kind() == reflect.Pointer && !v.IsNil() {
		v = v.Elem()
	}
	if !v.IsValid() || v.Kind() != reflect.String || v.Type().PkgPath() != stdlibPkgPath {
		return args
	}
	switch s := v.String(); v.Type().Name() {
	case "CSS":
		return []any{CSS(s)}
	case "HTML":
		return []any{HTML(s)}
	case "HTMLAttr":
		return []any{HTMLAttr(s)}
	case "JS":
		return []any{JS(s)}
	case "JSStr":
		return []any{JSStr(s)}
	case "Srcset":
		return []any{Srcset(s)}
	case "URL":
		return []any{URL(s)}
	}
	return args
}

// evalArgs formats the list of arguments into a string. It is equivalent to
// fmt.Sprint(args...), except that it dereferences all pointers.
// Copied from html/template/escape.go of go1.27.3.
func evalArgs(args ...any) string {
	// Optimization for simple common case of a single string argument.
	if len(args) == 1 {
		if s, ok := args[0].(string); ok {
			return s
		}
	}
	for i, arg := range args {
		args[i] = indirectToStringerOrError(arg)
	}
	return fmt.Sprint(args...)
}

// funcMap maps the names of the escapers the escaper records to the
// functions that render their inputs safe.
// Copied from html/template/escape.go of go1.27.3.
var funcMap = map[string]func(args ...any) string{
	"_html_template_attrescaper":      attrEscaper,
	"_html_template_commentescaper":   commentEscaper,
	"_html_template_cssescaper":       cssEscaper,
	"_html_template_cssvaluefilter":   cssValueFilter,
	"_html_template_htmlnamefilter":   htmlNameFilter,
	"_html_template_htmlescaper":      htmlEscaper,
	"_html_template_jsregexpescaper":  jsRegexpEscaper,
	"_html_template_jsstrescaper":     jsStrEscaper,
	"_html_template_jstmpllitescaper": jsTmplLitEscaper,
	"_html_template_jsvalescaper":     jsValEscaper,
	"_html_template_nospaceescaper":   htmlNospaceEscaper,
	"_html_template_rcdataescaper":    rcdataEscaper,
	"_html_template_srcsetescaper":    srcsetFilterAndEscaper,
	"_html_template_urlescaper":       urlEscaper,
	"_html_template_urlfilter":        urlFilter,
	"_html_template_urlnormalizer":    urlNormalizer,
	"_eval_args_":                     evalArgs,
}

// escapeSet owns the escaping state of one template set. Each template is
// escaped once, and its plan is not changed afterward, so executions that
// share it need no lock.
type escapeSet struct {
	mu      sync.Mutex
	results map[string]escapeResult // by template name
}

// escapeResult is what escaping one template came to.
type escapeResult struct {
	plan *escapePlan
	err  error
}

// escapeTemplate returns the plan to run the named template as the shadow
// trees in trees, or errUnsupported. The result is computed once per name.
func (s *escapeSet) escapeTemplate(trees map[string]*htt.Tree, name string) (*escapePlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.results[name]; ok {
		return r.plan, r.err
	}
	plan, err := escapeTemplate(trees, name)
	if s.results == nil {
		s.results = make(map[string]escapeResult)
	}
	s.results[name] = escapeResult{plan, err}
	return plan, err
}
