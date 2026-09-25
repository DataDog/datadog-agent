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
# Any Windows executable will do; Wine ships one in wine_run's runfiles.
cmd="$(rlocation wine_linux_x86_64/lib/wine/x86_64-windows/cmd.exe)"

out="$("$runner" "$cmd" /c echo hello-from-wine)"
if [[ "$out" != *hello-from-wine* ]]; then
    echo "unexpected output: $out" >&2
    exit 1
fi

# The Windows exit code must reach the caller.
if "$runner" "$cmd" /c exit 3; then
    echo "expected a non-zero exit code" >&2
    exit 1
else
    rc=$?
fi
if [[ $rc -ne 3 ]]; then
    echo "expected exit code 3, got $rc" >&2
    exit 1
fi
