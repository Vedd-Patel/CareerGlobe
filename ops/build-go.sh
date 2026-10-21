#!/usr/bin/env bash
# Prints the path of a Go binary from api/cmd/<name>, building it first when
# it is missing or older than the Go sources. Usage: ops/build-go.sh nightly
set -euo pipefail
REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
name="$1"
bin="$REPO/api/bin/$name"
if [[ ! -x "$bin" ]] || [[ -n "$(find "$REPO/api" -name '*.go' -newer "$bin" -print -quit)" ]]; then
  (cd "$REPO/api" && go build -o "bin/$name" "./cmd/$name") >&2
fi
echo "$bin"
