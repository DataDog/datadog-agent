#!/bin/sh
# Exercise tail -c (byte mode)
# Adapted from GNU coreutils tests/tail/tail-c.sh

# Copyright (C) 2014-2025 Free Software Foundation, Inc.
# License: GPLv3+

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

# Make sure it works for pipes
printf '123456' | safe_shell "tail -c3" > out || fail=1
printf '456' > exp || framework_failure_
compare exp out || fail=1

# tail -c on a regular file
printf 'abcdefghij' > infile || framework_failure_
safe_shell "tail -c 5 $PWD/infile" > out || fail=1
printf 'fghij' > exp || framework_failure_
compare exp out || fail=1

# tail -c +N (from byte offset)
safe_shell "tail -c +4 $PWD/infile" > out || fail=1
printf 'defghij' > exp || framework_failure_
compare exp out || fail=1

# tail -c 0 (should output nothing)
safe_shell "tail -c 0 $PWD/infile" > out || fail=1
printf '' > exp || framework_failure_
compare exp out || fail=1

# tail -c +1 (entire file)
safe_shell "tail -c +1 $PWD/infile" > out || fail=1
printf 'abcdefghij' > exp || framework_failure_
compare exp out || fail=1

Exit $fail
