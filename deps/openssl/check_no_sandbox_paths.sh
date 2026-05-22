#!/usr/bin/env bash
# Verify that no Bazel sandbox paths were baked into the OpenSSL shared libraries.
# If FIX_OPENSSL_PATHS failed to run, the configure-time sandbox path (containing
# "bazel-out", "execroot", or "build_tmpdir") would still be present in the binary,
# causing SSL cert lookups and provider loading to fail at runtime (cf. INC-46919).
set -euo pipefail

FAILED=0

libs=("$@")

for lib_relpath in "${libs[@]}"; do
    lib="$TEST_SRCDIR/$lib_relpath"
    echo "Checking $(basename "$lib")..."
    if [[ ! -f "$lib" ]]; then
        echo "FAIL: library not found: $lib" >&2
        FAILED=1
        continue
    fi
    # Only flag strings that combine a Bazel sandbox indicator with an OpenSSL
    # runtime path suffix. This avoids false positives from the compiler
    # command line that OpenSSL embeds in the binary for `openssl version -a`.
    bad_paths=$(strings "$lib" | grep -E "(bazel-out|execroot|build_tmpdir).*/?(ssl|lib/ossl-modules|lib/engines-[0-9]+)" || true)
    if [[ -n "$bad_paths" ]]; then
        echo "FAIL: Bazel sandbox path found in $lib:"
        echo "$bad_paths"
        FAILED=1
    else
        echo "OK: no sandbox paths in $(basename "$lib")"
    fi
done

exit $FAILED
