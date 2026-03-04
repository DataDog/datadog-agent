#!/bin/sh
# Verify that tail reads from the current stdin position when stdin is a pipe.
# Adapted from GNU coreutils tests/tail/start-middle.sh for the Datadog Agent
# safe shell interpreter.  The original verified that tail works when an fd
# is not at position 0 (e.g. after a prior read()); this adapted version
# exercises the equivalent using a pipe where only the later lines are passed.

# Copyright (C) 2001-2026 Free Software Foundation, Inc.
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

# Create a two-line file.
printf '1\n2\n' > k || framework_failure_

# Feed only the second line of the file into tail via pipe — simulating
# a caller that has already consumed the first line and left the fd at line 2.
# tail should output exactly "2".
tail -n +2 k | safe_shell "tail" > out || fail=1

printf '2\n' > exp || framework_failure_

compare exp out || fail=1

# Also verify that tail -n 1 on a two-line input gives the last line.
printf '1\n2\n' | safe_shell "tail -n 1" > out2 || fail=1
printf '2\n' > exp2 || framework_failure_
compare exp2 out2 || fail=1

Exit $fail
