#!/usr/bin/env sh
# Build a Mint-Interfaces Go package into a standalone, statically linked
# executable (CGO_ENABLED=0) that runs on any Linux machine without Go installed.
#
# Usage:
#   ./tools/build.sh ./templates/app          -> produces ./mytool (or package dir name)
#   ./tools/build.sh ./examples/demo ./demo   -> produces ./demo

set -eu

if [ $# -lt 1 ]; then
    echo "Usage: $0 <package-dir> [output-binary]" >&2
    exit 1
fi

PKG="$1"
OUT="${2:-$(basename "$PKG")}"

case "$OUT" in /*) ;; *) OUT="$PWD/$OUT" ;; esac

if [ -f "$PKG/go.mod" ]; then
    # An app with its own module (like apps/<tool>) is built from its folder.
    (cd "$PKG" && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$OUT" .)
else
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$OUT" "$PKG"
fi
echo "Built standalone static binary: $OUT"
