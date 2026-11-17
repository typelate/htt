// Package htt parses Go templates into trees that keep source positions and
// HTML structure, and executes those trees.
//
// The parser accepts the templates text/template and html/template accept.
// In HTML mode, elements, attributes and comments are nodes beside the
// actions, and each {{define}} and {{block}} stays where it was written.
// Package template puts html/template's API over the same engine.
//
// Execution serves analysis, tests and coverage. Its contextual escaping
// is weaker than html/template's (see [ExecuteTemplate]), so its output
// must not be served to a browser.
//
// The API carries no compatibility promise.
//
// Parts of this module derive from the Go standard library; see NOTICE.txt.
package htt
