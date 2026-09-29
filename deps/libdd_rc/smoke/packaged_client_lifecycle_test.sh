#!/usr/bin/env bash

set -euo pipefail

archive="$1"
if [[ "$archive" != /* ]]; then
    archive="$PWD/$archive"
fi

stage="$TEST_TMPDIR/rc-x509-package"
mkdir -p "$stage"
/usr/bin/tar -xf "$archive" -C "$stage"

binary="$stage/bin/client_lifecycle_test"
dylib="$stage/embedded/lib/libaws_lc_fips_0_14_2_crypto.dylib"

for file in "$binary" "$dylib"; do
    if [[ ! -f "$file" ]]; then
        echo "missing staged Mach-O file: $file" >&2
        exit 1
    fi
done

packaged_dylibs="$(find "$stage/embedded/lib" -type f -name 'libaws_lc_fips*.dylib' -print)"
dylib_count="$(find "$stage/embedded/lib" -type f -name 'libaws_lc_fips*.dylib' -print | wc -l | tr -d ' ')"
if [[ "$dylib_count" -ne 1 ]]; then
    echo "expected exactly one packaged AWS-LC FIPS dylib, found $dylib_count" >&2
    printf '%s\n' "$packaged_dylibs" >&2
    exit 1
fi

if ! /usr/bin/otool -D "$dylib" | tail -n +2 | grep -Fxq '@rpath/libaws_lc_fips_0_14_2_crypto.dylib'; then
    echo "unexpected dylib install name:" >&2
    /usr/bin/otool -D "$dylib" >&2
    exit 1
fi

if ! /usr/bin/otool -L "$binary" | tail -n +2 | awk '{print $1}' | grep -Fxq '@rpath/libaws_lc_fips_0_14_2_crypto.dylib'; then
    echo "staged executable does not link the packaged AWS-LC FIPS dylib through @rpath:" >&2
    /usr/bin/otool -L "$binary" >&2
    exit 1
fi

if ! /usr/bin/otool -l "$binary" | awk '$1 == "cmd" && $2 == "LC_RPATH" { getline; getline; print $2 }' | grep -Fxq '@loader_path/../embedded/lib'; then
    echo "staged executable does not contain the package-relative LC_RPATH:" >&2
    /usr/bin/otool -l "$binary" >&2
    exit 1
fi

for file in "$binary" "$dylib"; do
    if /usr/bin/otool -L "$file" | tail -n +2 | awk '{print $1}' | grep -E '(^/Users/|/bazel-out/|/sandbox/|^/opt/homebrew/|^/usr/local/)'; then
        echo "staged Mach-O contains a build-machine dependency path: $file" >&2
        /usr/bin/otool -L "$file" >&2
        exit 1
    fi
    /usr/bin/codesign --verify --strict --verbose=4 "$file"
done

env -u DYLD_LIBRARY_PATH -u DYLD_FALLBACK_LIBRARY_PATH \
    "$binary" -test.run='^TestClientLifecycle$' -test.v
