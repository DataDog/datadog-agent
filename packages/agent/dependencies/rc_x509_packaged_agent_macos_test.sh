#!/usr/bin/env bash

set -euo pipefail

archive="$1"
if [[ "$archive" != /* ]]; then
    archive="$PWD/$archive"
fi

stage="$TEST_TMPDIR/rc-x509-agent-package"
mkdir -p "$stage"
/usr/bin/tar -xf "$archive" -C "$stage"

agent="$stage/bin/agent/agent"
if [[ ! -f "$agent" ]]; then
    echo "missing staged production Agent: $agent" >&2
    exit 1
fi

case "$(uname -m)" in
    arm64) expected_arch="arm64" ;;
    x86_64) expected_arch="x86_64" ;;
    *)
        echo "unsupported macOS runner architecture: $(uname -m)" >&2
        exit 1
        ;;
esac
if ! /usr/bin/lipo -archs "$agent" | tr ' ' '\n' | grep -Fxq "$expected_arch"; then
    echo "production Agent does not contain expected architecture $expected_arch" >&2
    /usr/bin/lipo -archs "$agent" >&2
    exit 1
fi

# AWS-LC is linked statically into the rc-x509 client, so the package must not
# contain or dynamically reference a runtime AWS-LC dylib.
packaged_dylibs="$(find "$stage" -type f -name 'libaws_lc*.dylib' -print)"
if [[ -n "$packaged_dylibs" ]]; then
    echo "unexpected packaged AWS-LC dylib on macOS:" >&2
    printf '%s\n' "$packaged_dylibs" >&2
    exit 1
fi

if /usr/bin/otool -L "$agent" | tail -n +2 | grep -E 'aws_lc'; then
    echo "production Agent unexpectedly links a dynamic AWS-LC library:" >&2
    /usr/bin/otool -L "$agent" >&2
    exit 1
fi

if ! LC_ALL=C grep -aFq 'rc-x509-client' "$agent"; then
    echo "production Agent does not contain the statically linked rc-x509 client" >&2
    exit 1
fi

if /usr/bin/otool -L "$agent" | tail -n +2 | awk '{print $1}' | grep -E '(^/Users/|/bazel-out/|/sandbox/|^/opt/homebrew/|^/usr/local/)'; then
    echo "production Agent links a build-machine library path:" >&2
    /usr/bin/otool -L "$agent" >&2
    exit 1
fi
