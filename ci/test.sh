#!/bin/sh
# The checks every change must pass. Run from the repository root.
set -eu

unformatted="$(gofmt -l .)"
if [ -n "$unformatted" ]; then
	echo "gofmt needed:" "$unformatted" >&2
	exit 1
fi

go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@"${STATICCHECK_VERSION:-latest}" ./...
go test -race -shuffle=on ./...
