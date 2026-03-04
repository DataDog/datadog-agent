#!/bin/sh
# Test header behaviour for multiple files.
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

printf 'aaa\n' > a.txt
printf 'bbb\n' > b.txt

# Multiple files: headers shown by default
out=$(safe_shell "tail -n 1 a.txt b.txt") || fail_ "multi-file tail failed"
case "$out" in
  *'==> a.txt <=='*) ;;
  *) fail_ "expected ==> a.txt <== header, got: $out" ;;
esac
case "$out" in
  *'==> b.txt <=='*) ;;
  *) fail_ "expected ==> b.txt <== header, got: $out" ;;
esac

# -q suppresses headers
out=$(safe_shell "tail -q -n 1 a.txt b.txt") || fail_ "-q tail failed"
case "$out" in
  *'==>'*) fail_ "-q should suppress headers, got: $out" ;;
esac

# -v forces header for single file
out=$(safe_shell "tail -v -n 1 a.txt") || fail_ "-v tail failed"
case "$out" in
  *'==> a.txt <=='*) ;;
  *) fail_ "-v should force header, got: $out" ;;
esac

Exit 0
