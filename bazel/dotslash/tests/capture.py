"""Connects the fixture's declared files to the formatter's standard streams."""

import os
import subprocess
import sys
from pathlib import Path

with Path(os.environ["FIXTURE_INPUT"]).open("rb") as source:
    with Path(os.environ["FIXTURE_OUTPUT"]).open("wb") as destination:
        result = subprocess.run(sys.argv[1:], stdin=source, stdout=destination, check=False)
sys.exit(result.returncode)
