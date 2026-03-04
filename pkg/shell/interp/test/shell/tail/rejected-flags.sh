#!/bin/sh
# Verify that unsafe/unsupported flags are rejected with exit code 1.
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

check_rejected() {
  flag="$1"
  out=$(safe_shell "tail $flag /dev/null" 2>&1)
  ret=$?
  test $ret -ne 0 || fail_ "tail $flag should exit non-zero, got 0; output: $out"
}

check_rejected "-f"
check_rejected "--follow"
check_rejected "-F"
check_rejected "--retry"
check_rejected "-r"

Exit 0
