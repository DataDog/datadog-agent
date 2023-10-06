#!/usr/bin/env python3

import os.path
import subprocess
import sys

# List of files to ignore when running lint
FILE_IGNORE_LIST = [
    "pkg/collector/corechecks/containers/generic/adapters.go",
]

# Exclude non go files
# Get the package for each file
targets = {"./" + os.path.dirname(path) for path in sys.argv[1:] if path.endswith(".go")}

# Call invoke command
# We do this workaround since we can't do relative imports
cmd = f"revive {' '.join(targets)}"

try:
    result = subprocess.run(cmd, shell=True, check=True, capture_output=True, text=True)
    if result.stdout:
        files = set()
        skipped_files = set()
        for line in (out for out in result.stdout.split('\n') if out):
            fullname = line.split(":")[0]
            if fullname in FILE_IGNORE_LIST:
                skipped_files.add(fullname)
                continue
            print(line)
            files.add(fullname)

        # add whitespace for readability
        print()
        for skipped in skipped_files:
            print(f"Allowed errors in allowlisted file {skipped}")

        # add whitespace for readability
        print()

        if files:
            print(f"Linting issues found in {len(files)} files.")
            for f in files:
                print(f"Error in {f}")
            sys.exit(1)

    print("revive found no issues")
except subprocess.CalledProcessError:
    # Signal failure to pre-commit
    sys.exit(-1)
