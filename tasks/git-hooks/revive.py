#!/usr/bin/env python3

import os.path
import subprocess
import sys

# Exclude non go files
# Get the package for each file
targets = {"./" + os.path.dirname(path) for path in sys.argv[1:] if path.endswith(".go")}

# Call invoke command
# We do this workaround since we can't do relative imports
cmd = 'inv -e lint {}'.format(",".join(targets))

try:
    subprocess.run(cmd, shell=True, check=True)
except subprocess.CalledProcessError:
    # Signal failure to pre-commit
    sys.exit(-1)
