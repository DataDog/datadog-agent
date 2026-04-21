#!/usr/bin/env bash
# Verify that no Bazel sandbox paths were baked into the OpenSSL shared libraries.
# If FIX_OPENSSL_PATHS failed to run, the configure-time sandbox path (containing
# "bazel-out", "execroot", or "build_tmpdir") would still be present in the binary,
# causing SSL cert lookups and provider loading to fail at runtime (cf. INC-46919).
set -euo pipefail

FAILED=0

while IFS= read -r lib; do
    echo "Checking $(basename "$lib")..."
    # Only flag strings that combine a Bazel sandbox indicator with an OpenSSL
    # runtime path suffix. This avoids false positives from the compiler
    # command line that OpenSSL embeds in the binary for `openssl version -a`.
    if strings "$lib" | grep -qE "(bazel-out|execroot|build_tmpdir).*/?(ssl|lib/ossl-modules|lib/engines-[0-9]+)"; then
        echo "FAIL: Bazel sandbox path found in $lib:"
        strings "$lib" | grep -E "(bazel-out|execroot|build_tmpdir).*/?(ssl|lib/ossl-modules|lib/engines-[0-9]+)"
        FAILED=1
    else
        echo "OK: no sandbox paths in $(basename "$lib")"
    fi
done < <(find "$TEST_SRCDIR" \( -name "libcrypto.so" -o -name "libssl.so" \
                               -o -name "libcrypto.dylib" -o -name "libssl.dylib" \) 2>/dev/null)

exit $FAILED
