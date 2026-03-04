#!/bin/sh
# Test tail error handling: missing files, bad flags, rejected flags

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

# Missing file should produce error and exit 1
safe_shell "tail /nonexistent/file/path" > out 2> err
test $? -ne 0 || fail=1

# -f / --follow should be rejected (unknown flag via pflag)
safe_shell "tail -f somefile" > out 2> err
test $? -ne 0 || fail=1

# --follow should be rejected
safe_shell "tail --follow somefile" > out 2> err
test $? -ne 0 || fail=1

# -F should be rejected
safe_shell "tail -F somefile" > out 2> err
test $? -ne 0 || fail=1

# Invalid number
safe_shell "tail -n abc somefile" > out 2> err
test $? -ne 0 || fail=1

# Negative number (not valid for -n)
safe_shell "tail -n -5 somefile" > out 2> err
test $? -ne 0 || fail=1

Exit $fail
