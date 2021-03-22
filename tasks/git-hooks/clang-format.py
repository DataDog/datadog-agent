#!/usr/bin/env python3

import subprocess
import sys

# Exclude non .c/.h files
targets = [path for path in sys.argv[1:] if path.endswith(".c") or path.endswith(".h")]

# Call invoke command
cmd = 'inv -e system-probe.clang-format --fail-on-issue --targets {}'.format(",".join(targets))

try:
    subprocess.run(cmd, shell=True, check=True)
except subprocess.CalledProcessError:
    # Signal failure to pre-commit
    sys.exit(-1)
