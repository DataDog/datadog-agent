#!/usr/bin/env bash

echo ARGS: $*
TESTDATA_DIR="${TEST_SRCDIR}/${WORKSPACE_NAME}/_main/rtloader"

echo $TESTDATA_DIR
ls $TESTDATA_DIR

OUT=$(mktemp)
find $TESTDATA_DIR -name '*.c' -o -name '*.cpp' -o -name '*.h'  | xargs clang-format --style=file --dry-run >"$OUT"
cat "$OUT"
ERR_COUNT=$(wc -l <$OUT)
if [[ "$ERR_COUNT" -gt 0 ]] ; then
  exit 1
fi
