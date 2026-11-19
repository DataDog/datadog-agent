"""Double-fork a grandchild; write its pid to <root>/grandchild.pid.

Uses os.fork so we never call sys.executable (empty under Bazel runfiles).
Exits only after <root>/grandchild.release appears.
"""

import os
import sys
import time

root = sys.argv[1]
pidfile = os.path.join(root, "grandchild.pid")
release = os.path.join(root, "grandchild.release")


def wait_for_release(path: str, timeout: float = 60.0) -> None:
    deadline = time.monotonic() + timeout
    while not os.path.exists(path):
        if time.monotonic() >= deadline:
            os._exit(1)
        time.sleep(0.05)


mid = os.fork()
if mid == 0:
    # Middle: fork grandchild, record its pid, then exit so it is reparented.
    g = os.fork()
    if g == 0:
        wait_for_release(release)
        os._exit(0)
    tmp = pidfile + ".tmp"
    with open(tmp, "w", encoding="utf-8") as f:
        f.write(str(g))
    os.replace(tmp, pidfile)
    os._exit(0)

os.waitpid(mid, 0)
time.sleep(300)
