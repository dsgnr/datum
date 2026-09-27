#!/bin/sh
# A VERSION-only change must rebuild a binary even when no source file changed.
set -eu
workspace=$(mktemp -d)
trap 'rm -rf "$workspace"' EXIT
cp Makefile go.mod go.sum "$workspace/"
cp -R cmd internal "$workspace/"
for version in 0.1.0~test.1 0.1.0~test.2; do
    make -s --no-print-directory -C "$workspace" build DOCKER_ARCH=unused VERSION="$version"
    actual=$("$workspace/bin/datum" version | head -1)
    if [ "$actual" != "datum $version" ]; then
        echo "expected datum $version, got $actual" >&2
        exit 1
    fi
done
echo "ok: changing VERSION rebuilds the binary"
