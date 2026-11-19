#!/bin/sh
# The checks every change must pass. Run from the repository root.
set -eu

unformatted="$(gofmt -l .)"
if [ -n "$unformatted" ]; then
	echo "gofmt needed:" "$unformatted" >&2
	exit 1
fi

# muxt builds htt/template in place of html/template, so these packages may
# not import it outside tests. go list -deps leaves test files out.
if go list -deps . ./lex ./template | grep -qx 'html/template'; then
	echo "a non-test file of htt, htt/lex or htt/template imports html/template" >&2
	exit 1
fi

go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@"${STATICCHECK_VERSION:-latest}" ./...
go test -race -shuffle=on ./...
