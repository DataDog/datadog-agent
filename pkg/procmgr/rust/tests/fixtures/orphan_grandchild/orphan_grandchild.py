"""Double-fork a short-lived grandchild; write its pid to <root>/grandchild.pid.

Uses os.fork so we never call sys.executable (empty under Bazel runfiles).
"""

import os
import sys
import time

root = sys.argv[1]
pidfile = os.path.join(root, "grandchild.pid")

mid = os.fork()
if mid == 0:
    # Middle: fork grandchild, record its pid, then exit so it is reparented.
    g = os.fork()
    if g == 0:
        time.sleep(3)
        os._exit(0)
    tmp = pidfile + ".tmp"
    with open(tmp, "w", encoding="utf-8") as f:
        f.write(str(g))
    os.replace(tmp, pidfile)
    os._exit(0)

os.waitpid(mid, 0)
time.sleep(300)
