#!/usr/bin/env python3
import re
import sys

from tasks.libs.common.color import color_message

# Select only relevant files
files = [
    path
    for path in sys.argv[1:]
    if path.endswith(".sh")
    or path.endswith("Dockerfile")
    or path.endswith(".yml")
    or (path.endswith(".yaml") and not path.startswith(".pre-commit-config"))
]
errors = []
for file in files:
    with open(file) as f:
        for nb, line in enumerate(f):
            if re.search(r"set( +-[^ ])* +-[^ ]*(x|( +xtrace))", line):
                errors.append(
                    f"{color_message(file, 'magenta')}:{color_message(nb + 1, 'green')}: {color_message(line.strip(), 'red')}"
                )
if errors:
    for error in errors:
        print(error, file=sys.stderr)
    print(color_message('error:', 'red'), 'No shell script should use "set -x"', file=sys.stderr)
    sys.exit(1)
