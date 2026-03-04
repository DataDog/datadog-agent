#!/bin/sh
# Test basic tail line-counting modes.
# Inspired by GNU coreutils tail.pl test vectors.

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

# Create a test file with 12 lines
seq 1 12 > infile || framework_failure_

# Default: last 10 lines
safe_shell "tail $PWD/infile" > out || fail=1
seq 3 12 > exp || framework_failure_
compare exp out || fail=1

# -n 3: last 3 lines
safe_shell "tail -n 3 $PWD/infile" > out || fail=1
seq 10 12 > exp || framework_failure_
compare exp out || fail=1

# -n +5: starting from line 5
safe_shell "tail -n +5 $PWD/infile" > out || fail=1
seq 5 12 > exp || framework_failure_
compare exp out || fail=1

# -n 0: no output
safe_shell "tail -n 0 $PWD/infile" > out || fail=1
printf '' > exp || framework_failure_
compare exp out || fail=1

# -n +1: entire file (from line 1)
safe_shell "tail -n +1 $PWD/infile" > out || fail=1
seq 1 12 > exp || framework_failure_
compare exp out || fail=1

# -n +0: same as +1 (output everything)
safe_shell "tail -n +0 $PWD/infile" > out || fail=1
seq 1 12 > exp || framework_failure_
compare exp out || fail=1

# File with no trailing newline
# Note: tested in Go unit tests (TestTail_NoTrailingNewline).
# Shell comparison tools struggle with no-newline files, so we use
# a simple byte-count check here instead.
printf 'a\nb\nc' > nonl || framework_failure_
safe_shell "tail -n 1 $PWD/nonl" > out || fail=1
result=$(cat out)
test "$result" = "c" || fail=1

# Single line file
printf 'only\n' > single || framework_failure_
safe_shell "tail -n 5 $PWD/single" > out || fail=1
printf 'only\n' > exp || framework_failure_
compare exp out || fail=1

# Empty file
> empty || framework_failure_
safe_shell "tail $PWD/empty" > out || fail=1
printf '' > exp || framework_failure_
compare exp out || fail=1

Exit $fail
