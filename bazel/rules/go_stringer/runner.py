#!/usr/bin/env python3
import os
import subprocess
import sys
from pathlib import Path

stringer_cwd = Path(os.environ["SRC"]).parent
subprocess.check_call(
    [Path(os.environ["STRINGER"]).absolute()] + sys.argv[1:],
    cwd=stringer_cwd,
    env={"HOME": Path.cwd(), "PATH": Path(os.environ["GO"]).parent.absolute()},
)
(stringer_cwd / os.environ["STRINGER_OUT"]).rename(os.environ["OUT"])
