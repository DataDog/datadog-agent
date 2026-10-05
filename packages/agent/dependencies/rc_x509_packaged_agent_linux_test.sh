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

# AWS-LC FIPS is linked statically on Linux, so the package must not contain or
# dynamically reference a runtime AWS-LC shared object.
packaged_shared_libraries="$(find "$stage" -type f \( -name 'libaws_lc_fips*.so' -o -name 'libaws_lc_fips*.so.*' -o -name 'libaws_lc_fips*.dylib' \) -print)"
if [[ -n "$packaged_shared_libraries" ]]; then
    echo "unexpected packaged AWS-LC FIPS shared library on Linux:" >&2
    printf '%s\n' "$packaged_shared_libraries" >&2
    exit 1
fi

if /usr/bin/readelf -d "$agent" | grep -E 'NEEDED.*aws_lc_fips'; then
    echo "production Agent unexpectedly requires a dynamic AWS-LC FIPS library:" >&2
    /usr/bin/readelf -d "$agent" >&2
    exit 1
fi

if ! LC_ALL=C grep -aFq 'rc-x509-client' "$agent"; then
    echo "production Agent does not contain the statically linked rc-x509 client" >&2
    exit 1
fi
