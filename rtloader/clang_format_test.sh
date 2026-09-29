#!/usr/bin/env bash

set -euo pipefail

TESTDATA_DIR="${TEST_SRCDIR}/_main/rtloader"

OUT=$(mktemp)
FTMP=$(mktemp)
cd "$TESTDATA_DIR"
(
  find . -name '*.c' -o -name '*.cpp' -o -name '*.h'  | while read path ; do
    clang-format --style=file -Werror "$path" >"$FTMP" || true
    diff "$path" "$FTMP" || echo "FAIL: $path"
  done
) >"$OUT"
cat "$OUT"
ERR_COUNT=$(wc -l <$OUT)
if [[ "$ERR_COUNT" -gt 0 ]] ; then
  exit 1
fi
/bin/rm "$OUT" "$FTMP"
