#!/bin/sh
# Verify that tail works when seeking within a file.
# Adapted from GNU coreutils tests/tail/basic-seek.sh for the Datadog Agent
# safe shell interpreter.  The original test verified that coreutils tail did
# not crash (exit 139) when reading a large regular file with -n; this adapted
# version checks that the builtin tail returns the correct line count.

# Copyright (C) 2025-2026 Free Software Foundation, Inc.
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

# Resolve init.sh relative to this script's location before setup_ cds away.
srcdir=$(cd "$(dirname "$0")/../.." && pwd)
. "$srcdir/init.sh"

# Locate the agent binary.
# Override via AGENT_BIN=/path/to/agent or ensure the binary is built first.
AGENT_BIN=${AGENT_BIN:-}
if test -z "$AGENT_BIN"; then
  AGENT_BIN=$(command -v agent 2>/dev/null || true)
fi
if test -z "$AGENT_BIN" || ! test -x "$AGENT_BIN"; then
  skip_ "agent binary not found; build with: dda inv agent.build"
fi

# safe_shell CMD: run CMD through the Datadog Agent safe shell interpreter.
safe_shell() { "$AGENT_BIN" shell --command "$*"; }

# Generate a 1000-line file using the host shell (not the safe interpreter).
i=0
while test $i -lt 1000; do
  echo '================================='
  i=$(expr $i + 1)
done > file.in || framework_failure_

# Requesting 200 lines from the tail of a 1000-line file should give exactly
# 200 lines (exercises the seek optimisation for regular files).
actual=$(safe_shell "tail -n 200 file.in" | wc -l | tr -d ' ')
test "$actual" = 200 || { echo "expected 200 lines, got $actual"; fail=1; }

Exit $fail
