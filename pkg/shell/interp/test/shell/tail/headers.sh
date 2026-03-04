#!/bin/sh
# Test tail multi-file headers (-q, -v flags)

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

printf 'aaa\n' > f1 || framework_failure_
printf 'bbb\n' > f2 || framework_failure_

# Multiple files: headers by default
safe_shell "tail -n 1 $PWD/f1 $PWD/f2" > out || fail=1
cat <<EOF > exp
==> $PWD/f1 <==
aaa

==> $PWD/f2 <==
bbb
EOF
compare exp out || fail=1

# -q: suppress headers even with multiple files
safe_shell "tail -q -n 1 $PWD/f1 $PWD/f2" > out || fail=1
cat <<EOF > exp
aaa
bbb
EOF
compare exp out || fail=1

# -v: show header even with single file
safe_shell "tail -v -n 1 $PWD/f1" > out || fail=1
cat <<EOF > exp
==> $PWD/f1 <==
aaa
EOF
compare exp out || fail=1

# Single file: no header by default
safe_shell "tail -n 1 $PWD/f1" > out || fail=1
cat <<EOF > exp
aaa
EOF
compare exp out || fail=1

Exit $fail
