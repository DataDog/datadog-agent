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

# Windows looks for DLLs next to the executable first.
cp "$(rlocation "$CONSUMER")" "$TEST_TMPDIR/"
for f in $OPENSSL_FILES; do
    [[ "$f" == *.dll ]] && cp "$(rlocation "$f")" "$TEST_TMPDIR/"
done

out="$("$runner" "$TEST_TMPDIR/$(basename "$CONSUMER")")"
if [[ "$out" != OpenSSL\ 3.* ]]; then
    echo "unexpected output: $out" >&2
    exit 1
fi
