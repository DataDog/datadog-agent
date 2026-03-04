#!/bin/sh
# Exercise tail -c — adapted from GNU coreutils tests/tail/tail-c.sh
#
# Changes from upstream:
#  - Use AGENT_BIN shell --command instead of the system tail binary
#  - Remove /dev/urandom test (depends on timeout binary, unreliable)
#  - Use printf instead of echo -n (POSIX portability)

# Copyright 2014-2026 Free Software Foundation, Inc.
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

# Helper: run input through a tail command via agent shell
run_tail() {
  input="$1"; shift
  printf '%s' "$input" | safe_shell "tail $*"
}

# -c 3 from pipe: last 3 bytes of "123456" = "456"
out=$(run_tail '123456' '-c3') || { echo "FAIL pipe -c3: command failed"; fail=1; }
test "$out" = "456" || { echo "FAIL pipe -c3: got '$out'"; fail=1; }

# -c +3 (from byte 3): bytes 3-6 of "abcdef" = "cdef"
out=$(run_tail 'abcdef' '-c +3') || { echo "FAIL -c +3: command failed"; fail=1; }
test "$out" = "cdef" || { echo "FAIL -c +3: got '$out'"; fail=1; }

# -c 0: output nothing
out=$(run_tail 'hello' '-c 0') || { echo "FAIL -c 0: command failed"; fail=1; }
test -z "$out" || { echo "FAIL -c 0: expected empty, got '$out'"; fail=1; }

# Very large -c value with empty input should succeed (not error)
out=$(run_tail '' '-c 99999999999999999999') || { echo "FAIL huge -c empty: command failed"; fail=1; }
test -z "$out" || { echo "FAIL huge -c empty: got '$out'"; fail=1; }

Exit $fail
