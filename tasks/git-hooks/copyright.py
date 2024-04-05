#!/usr/bin/env python3

import sys

sys.path.insert(0, "tasks")
from tasks.libs.types.copyright import CopyrightLinter, LintFailure  # noqa: E402

# Exclude non go files
files = [path for path in sys.argv[1:] if path.endswith(".go")]
try:
    CopyrightLinter().assert_compliance(files=files)
except LintFailure:
    # the linter prints useful messages on its own, so no need to print the exception
    sys.exit(1)
