#!/bin/sh
# Verify that tail works when piped input has already been partially consumed.
# Adapted from GNU coreutils tests/tail/start-middle.sh

# Copyright (C) 2001-2025 Free Software Foundation, Inc.
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

# Test default tail (last 10 lines) via pipe
echo -e "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12" | safe_shell "tail" > out || fail=1
cat <<EOF > exp
3
4
5
6
7
8
9
10
11
12
EOF
compare exp out || fail=1

# Test tail -n with pipe
echo -e "a\nb\nc\nd\ne" | safe_shell "tail -n 2" > out || fail=1
cat <<EOF > exp
d
e
EOF
compare exp out || fail=1

Exit $fail
