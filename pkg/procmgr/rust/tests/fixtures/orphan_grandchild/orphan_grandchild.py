"""Double-fork a short-lived grandchild; write its pid to <root>/grandchild.pid."""

import os
import subprocess
import sys
import time

root = sys.argv[1]
pidfile = os.path.join(root, "grandchild.pid")

middle = (
    "import os, subprocess, sys;"
    "g = subprocess.Popen([sys.executable, '-c', 'import time; time.sleep(3)']);"
    "tmp = sys.argv[1] + '.tmp';"
    "open(tmp, 'w').write(str(g.pid));"
    "os.replace(tmp, sys.argv[1])"
)
subprocess.run([sys.executable, "-c", middle, pidfile], check=True)

time.sleep(300)
