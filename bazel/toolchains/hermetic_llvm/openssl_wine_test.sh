#!/usr/bin/env bash

# --- begin runfiles.bash initialization v3 ---
set -uo pipefail; set +e; f=bazel_tools/tools/bash/runfiles/runfiles.bash
# shellcheck disable=SC1090
source "${RUNFILES_DIR:-/dev/null}/$f" 2>/dev/null || \
  source "$(grep -sm1 "^$f " "${RUNFILES_MANIFEST_FILE:-/dev/null}" | cut -f2- -d' ')" 2>/dev/null || \
  source "$0.runfiles/$f" 2>/dev/null || \
  source "$(grep -sm1 "^$f " "$0.runfiles_manifest" | cut -f2- -d' ')" 2>/dev/null || \
  source "$(grep -sm1 "^$f " "$0.exe.runfiles_manifest" | cut -f2- -d' ')" 2>/dev/null || \
  { echo>&2 "ERROR: cannot find $f"; exit 1; }; f=; set -e
# --- end runfiles.bash initialization v3 ---

runner="$(rlocation "$WINE_RUN")"
for f in $OPENSSL_FILES; do
    [[ "$f" == */openssl.exe ]] && exe="$(rlocation "$f")"
done

# The DLLs sit next to openssl.exe, where Windows looks for them first.
out="$("$runner" "$exe" version)"
if [[ "$out" != OpenSSL\ 3.* ]]; then
    echo "unexpected openssl version: $out" >&2
    exit 1
fi
