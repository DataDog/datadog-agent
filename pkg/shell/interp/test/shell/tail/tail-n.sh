#!/bin/sh
# Exercise tail -n — adapted from GNU coreutils tests/tail/tail.pl
#
# Ported from the Perl test matrix (non-obsolete, non-follow-mode).
# Removed: obsolete +Nc forms, -f tests, zero-terminated (-z), -b block tests.
#
# Copyright (C) 2008-2026 Free Software Foundation, Inc.
# Apache License Version 2.0 for this adapted version.

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

fail=0

# check NAME INPUT EXPECTED CMD
# Runs: printf '%s' "$INPUT" | safe_shell "CMD"
# Compares output to EXPECTED (trailing newlines stripped by $(...) are acceptable).
check() {
  name="$1"; input="$2"; expected="$3"; cmd="$4"
  out=$(printf '%s' "$input" | safe_shell "$cmd") || { echo "FAIL[$name]: command exited non-zero"; fail=1; return; }
  if test "$out" != "$expected"; then
    printf 'FAIL[%s]: expected |%s| got |%s|\n' "$name" "$expected" "$out"
    fail=1
  fi
}

check_exit1() {
  name="$1"; cmd="$2"
  printf '' | safe_shell "$cmd" >/dev/null 2>&1
  ret=$?
  test $ret -ne 0 || { echo "FAIL[$name]: expected exit non-zero, got 0"; fail=1; }
}

# ---------------------------------------------------------------------------
# -n last-N lines
# ---------------------------------------------------------------------------

# tail -n 1 of single line (no trailing newline)
check "n-single-nolf" "x" "x" "tail -n 1"

# tail -n 1 of two lines with trailing newline → last line "y"
check "n-lf1" "$(printf 'x\ny\n')" "y" "tail -n 1"

# tail -n 1 of two lines without trailing newline → last line "y"
check "n-lf2" "$(printf 'x\ny')" "y" "tail -n 1"

# tail -n 10 of 12 lines (x, y*10, z)
INPUT12=$(printf 'x\n'; printf 'y\n%.0s' 1 2 3 4 5 6 7 8 9 10; printf 'z')
check "n-12-last10" "$INPUT12" "$(printf 'y\n%.0s' 1 2 3 4 5 6 7 8 9; printf 'z')" "tail -n 10"

# tail -n 0: no output
check "n-0" "$(printf 'y\n%.0s' 1 2 3 4 5)" "" "tail -n 0"

# ---------------------------------------------------------------------------
# -n +N (from-line) mode
# ---------------------------------------------------------------------------

# +1 = from line 1 = all lines
INPUT5=$(printf 'y\n%.0s' 1 2 3 4 5)
check "n-plus0" "$INPUT5" "$INPUT5" "tail -n +0"
check "n-plus1" "$INPUT5" "$INPUT5" "tail -n +1"

# +2 = skip first line
check "n-plus2" "$(printf 'x\ny\n')" "y" "tail -n +2"

# tail -n +10 of 12-line input → lines 10, 11, 12
check "n-plus10" "$INPUT12" "$(printf 'y\ny\nz')" "tail -n +10"

# ---------------------------------------------------------------------------
# Error cases
# ---------------------------------------------------------------------------
check_exit1 "err-invalid-bytes" "tail -c l"
check_exit1 "err-unknown-flag"  "tail --follow /dev/null"

Exit $fail
