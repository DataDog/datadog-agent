#!/bin/sh
# Basic tail tests for the agent safe-shell interpreter.
# Adapted from GNU coreutils tail tests.
srcdir=$(cd "$(dirname "$0")/.." && pwd)
. "$srcdir/init.sh"

AGENT_BIN=${AGENT_BIN:-}
if test -z "$AGENT_BIN"; then
  AGENT_BIN=$(command -v agent 2>/dev/null || true)
fi
if test -z "$AGENT_BIN" || ! test -x "$AGENT_BIN"; then
  skip_ "agent binary not found; build with: dda inv agent.build"
fi

safe_shell() { "$AGENT_BIN" shell --command "$*"; }

# ---------------------------------------------------------------------------
# last-N lines default (10)
# ---------------------------------------------------------------------------
printf '%s\n' 1 2 3 4 5 6 7 8 9 10 11 12 > input.txt

out=$(safe_shell "tail input.txt") || fail_ "tail default failed"
test "$(printf '%s\n' $out | wc -l)" != 0 || fail_ "expected output"
# last line should be 12
last=$(printf '%s\n' $out | tail -n 1)
test "$last" = "12" || fail_ "last line should be 12, got: $last"

# ---------------------------------------------------------------------------
# -n 3
# ---------------------------------------------------------------------------
out=$(safe_shell "tail -n 3 input.txt") || fail_ "tail -n 3 failed"
expected=$(printf '10\n11\n12\n')
test "$out" = "$expected" || fail_ "-n 3: expected '10 11 12', got: $out"

# ---------------------------------------------------------------------------
# -n 0 produces no output
# ---------------------------------------------------------------------------
out=$(safe_shell "tail -n 0 input.txt") || fail_ "tail -n 0 failed"
test -z "$out" || fail_ "-n 0 should produce no output, got: $out"

# ---------------------------------------------------------------------------
# -n +3 (from line 3)
# ---------------------------------------------------------------------------
printf 'a\nb\nc\nd\ne\n' > lines.txt
out=$(safe_shell "tail -n +3 lines.txt") || fail_ "tail -n +3 failed"
expected=$(printf 'c\nd\ne\n')
test "$out" = "$expected" || fail_ "-n +3 expected 'c d e', got: $out"

# ---------------------------------------------------------------------------
# -c (bytes)
# ---------------------------------------------------------------------------
printf 'abcdef' > bytes.txt
out=$(safe_shell "tail -c 3 bytes.txt") || fail_ "tail -c 3 failed"
test "$out" = "def" || fail_ "-c 3: expected 'def', got: $out"

# ---------------------------------------------------------------------------
# -c +3 (from byte 3)
# ---------------------------------------------------------------------------
out=$(safe_shell "tail -c +3 bytes.txt") || fail_ "tail -c +3 failed"
test "$out" = "cdef" || fail_ "-c +3: expected 'cdef', got: $out"

Exit 0
