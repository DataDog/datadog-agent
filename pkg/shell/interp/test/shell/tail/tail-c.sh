#!/bin/sh
# Exercise tail -c (byte mode).
# Adapted from GNU coreutils tests/tail/tail-c.sh for the Datadog Agent safe
# shell interpreter.  The original also tested /proc/version, /dev/zero, and
# /dev/urandom; those sections are omitted because:
#   - /proc/version / /sys paths are non-portable.
#   - /dev/zero never reaches EOF on non-seekable read, which our circular
#     buffer implementation does not guard against (by design — we cap at
#     maxTailBytes and still loop).  Avoid this in automated tests.
#   - /dev/urandom tests rely on timeout(1) which is not a builtin.

# Copyright (C) 2014-2026 Free Software Foundation, Inc.
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <https://www.gnu.org/licenses/>.

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

# Make sure -c works on pipes (non-seekable path: circular byte buffer).
printf '123456' | safe_shell "tail -c 3" > out || fail=1
printf '456'    > exp || framework_failure_
compare exp out || fail=1

# Make sure -c works on a regular file (seekable path: lseek optimisation).
printf 'abcdef\n' > regular.txt || framework_failure_
safe_shell "tail -c 3 regular.txt" > out2 || fail=1
printf 'ef\n'   > exp2 || framework_failure_
compare exp2 out2 || fail=1

# Requesting more bytes than file size should return the entire file.
printf 'hi\n' > small.txt || framework_failure_
safe_shell "tail -c 1000 small.txt" > out3 || fail=1
printf 'hi\n' > exp3 || framework_failure_
compare exp3 out3 || fail=1

# Requesting 0 bytes should produce no output.
safe_shell "tail -c 0 regular.txt" > out4 || fail=1
compare /dev/null out4 || fail=1

# +N offset: from byte N to end.
printf 'abcdef\n' | safe_shell "tail -c +4" > out5 || fail=1
printf 'def\n'  > exp5 || framework_failure_
compare exp5 out5 || fail=1

Exit $fail
