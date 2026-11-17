#!/bin/sh
# Fuzzes each target for FUZZTIME (default 30s) and fails if any target
# fails. Run from the repository root. go test fuzzes one target of one
# package per run, hence the loops. A failing input is written under the
# package's testdata/fuzz; commit it with the fix.
set -eu

fuzztime="${FUZZTIME:-30s}"
failed=""

for pkg in $(go list ./...); do
	for target in $(go test -list '^Fuzz' "$pkg" | grep '^Fuzz' || true); do
		echo "== $pkg $target"
		if ! go test -run '^$' -fuzz "^${target}\$" -fuzztime "$fuzztime" "$pkg"; then
			failed="$failed $pkg.$target"
		fi
	done
done

if [ -n "$failed" ]; then
	echo "failed:$failed" >&2
	exit 1
fi
