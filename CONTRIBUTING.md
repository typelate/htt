# How to change htt

Use the Go version on the `go` line of `go.mod`. Run commands from the repository root.

```sh
go test -short ./...       # after each edit
ci/test.sh                 # before a commit
ci/fuzz.sh                 # every fuzz target; FUZZTIME sets the time for each
ci/downstream.sh ../muxt   # muxt's tests against this checkout
```

- A failing case in `TestCompatTextExecTests`, `TestCompatHTMLEscape` or `TestCompatHTMLErrors` is a bug in htt. Those tables come from the standard library's tests.
- The comparison tests need a Go no older than `escaperCopyVersion` in `template/diff_fuzz_test.go`.
- A file whose header says `Copied from html/template` is a copy. Mark each edit `// Changed:` and keep the original order.
- A fuzz failure writes its input under `testdata/fuzz`. Commit it with the fix.
- Throwaway programs go in `cmd/tmp/<name>/` and build to `bin/`.
- Commit subjects follow Conventional Commits. A behavior change states Given, When, Then in its body.
