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
/bin/rm "$FTMP"
ERR_COUNT=$(wc -l <$OUT)
if [[ "$ERR_COUNT" -gt 0 ]] ; then
  echo "To format the rtloader sources use:"
  /bin/sed -n -e 's@FAIL: @clang-format --style=file -i rtloader/@p' $OUT
  /bin/rm "$OUT"
  exit 1
fi
/bin/rm "$OUT"
