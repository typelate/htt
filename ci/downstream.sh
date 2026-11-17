#!/bin/sh
# Runs muxt's tests against this checkout of htt. muxt holds tests that
# exercise htt's parser, diagnostics and coverage, so a change here can
# break them.
#
#	ci/downstream.sh path/to/muxt [go test flags]
#
# The muxt checkout is not modified: its committed state is cloned to a
# temporary directory, and the clone's go.mod is pointed at this htt. A
# workspace file would not do, because muxt's tests run the go command in
# modules of their own, outside any workspace.
#
# muxt's overlay tests need a GOROOT outside the module cache, as the
# pipeline's image has. Where the go command on PATH is a downloaded
# toolchain they fail whatever htt is; on such a machine run with an
# installed Go first on PATH and GOTOOLCHAIN=local.
set -eu

# This muxt test asserts that the engine note names a released htt, and
# here htt is a directory replacement, so it cannot pass.
skip='^TestTemplateCoverageRunsTheEngineOverlay$'

htt="$(cd "$(dirname "$0")/.." && pwd)"
muxt="$(cd "${1:?usage: ci/downstream.sh path/to/muxt [go test flags]}" && pwd)"
shift

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

git clone --quiet --local "$muxt" "$work/muxt"
cd "$work/muxt"
go mod edit -replace "github.com/typelate/htt=$htt"
go mod tidy

# muxt's tests run the muxt command; build it against this htt without
# replacing the one already installed.
export GOBIN="$work/bin"
PATH="$GOBIN:$PATH"
go install ./cmd/muxt

go test -shuffle=on -timeout 30m -skip "$skip" "$@" ./...
