package template

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/typelate/htt"
)

// CoverEnv names the environment variable that turns on coverage export.
// When it holds a directory, the process appends the blocks its templates
// execute to <dir>/<pid>.cov, in htt.CoverProfile.WriteTo's format.
const CoverEnv = "MUXT_TEMPLATE_COVER"

// CoverDisabledMarker starts the line printed on stderr when coverage
// export fails and stops. The error follows it.
const CoverDisabledMarker = "muxt: template coverage export disabled:"

// processCover is the process's coverage export, nil unless CoverEnv named
// a directory at init. It is package-level state on purpose: this package
// stands in for html/template, whose API has nowhere to pass an export.
// Tests replace it through useCoverExport.
var processCover = coverExportFor(os.Getenv(CoverEnv), os.Stderr)

// coverExport appends each executed block to a cover profile file the
// first time it runs. Writes are synchronous, since there is no exit hook
// to flush from. The file is created with its mode line at init, so that
// an empty profile means no template ran and a missing one means the
// export never started. The first error is reported once on stderr and
// turns the export off: coverage must not fail the tests it rides on.
type coverExport struct {
	path   string
	stderr io.Writer

	mu       sync.Mutex
	disabled bool
	file     *os.File
	written  map[string]struct{}
}

// coverExportFor opens the export to <dir>/<pid>.cov. It returns nil for
// an empty dir, and an export that is off when the file cannot be opened.
func coverExportFor(dir string, stderr io.Writer) *coverExport {
	if dir == "" {
		return nil
	}
	c := &coverExport{
		path:    filepath.Join(dir, strconv.Itoa(os.Getpid())+".cov"),
		stderr:  stderr,
		written: make(map[string]struct{}),
	}
	f, err := openProfile(c.path)
	if err != nil {
		c.disable(err)
		return c
	}
	// The file stays open until the process exits.
	c.file = f
	return c
}

// disable turns the export off and reports err. The caller holds mu or is
// the only one with c.
func (c *coverExport) disable(err error) {
	c.disabled = true
	// A failure to report has nowhere better to go.
	_, _ = fmt.Fprintf(c.stderr, "%s %v\n", CoverDisabledMarker, err)
}

// execute runs text, the shadow tree of a template, with a profile of its
// own, then appends the blocks of the HTML trees it reached for the first
// time. htmlTree gives the tree with HTML structure for each text tree that
// ran. A profile per call keeps the export from walking every tree after
// each execute.
func (c *coverExport) execute(wr io.Writer, text *htt.Tree, data any, opts []htt.Option, htmlTree func(*htt.Tree) *htt.Tree) error {
	cp := new(htt.CoverProfile)
	err := htt.ExecuteTemplate(wr, text, data, append(opts, htt.WithCoverProfile(cp))...)
	c.append(cp, htmlTree)
	return err
}

// append gives each HTML tree the counts of the text tree that ran in its
// place, then writes the blocks not written yet. A text tree that is its own
// HTML tree is counted as it is. The counts are set under mu, since two
// copies could store a stale lower count if they were set unserialized.
func (c *coverExport) append(cp *htt.CoverProfile, htmlTree func(*htt.Tree) *htt.Tree) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.disabled {
		return
	}
	blocks := new(htt.CoverProfile)
	for _, text := range cp.Trees() {
		tree := htmlTree(text)
		if tree == nil {
			continue
		}
		if tree != text {
			tree.CoverFrom(text)
		}
		blocks.AddTree(tree)
	}
	if _, err := blocks.WriteNewlyCovered(c.file, c.written); err != nil {
		c.disable(err)
	}
}

// openProfile opens name for appending and writes the mode line if the
// file is empty. A process with the same pid, as after exec, may have
// written to it already.
func openProfile(name string) (*os.File, error) {
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		return nil, errors.Join(err, f.Close())
	}
	if info.Size() == 0 {
		if _, err := io.WriteString(f, "mode: set\n"); err != nil {
			return nil, errors.Join(err, f.Close())
		}
	}
	return f, nil
}
