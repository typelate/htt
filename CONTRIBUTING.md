# How to change htt

Use the Go version on the `go` line of `go.mod`. Run commands from the repository root.

```sh
go test -short ./...       # after each edit
ci/test.sh                 # before a commit
ci/fuzz.sh                 # every fuzz target; FUZZTIME sets the time for each
ci/downstream.sh ../muxt   # muxt's tests against this checkout
```

- A failing test in a `compat_*_test.go` or `std_*_test.go` file is a bug in htt. Those tests come from the standard library.
- The comparison tests need a Go no older than `escaperCopyVersion` in `template/diff_fuzz_test.go`.
- A file whose header says `Copied from` is a copy of a standard-library file. Mark each edit `// Changed:` and keep the original order.
- A fuzz failure writes its input under `testdata/fuzz`. Commit it with the fix.
- Throwaway programs go in `cmd/tmp/<name>/` and build to `bin/`.
- Commit subjects follow Conventional Commits. A behavior change states Given, When, Then in its body.
