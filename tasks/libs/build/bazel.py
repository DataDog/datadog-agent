"""Provides helper functions for invoking Bazel commands."""

from __future__ import annotations

import shlex
import shutil
import subprocess
import sys

from tasks.libs.common.color import color_message


def bazel_not_found_message(color: str) -> str:
    return color_message("Please run `inv install-tools` for `bazel` support!", color)


def bazel(*args: str, capture_output: bool = False) -> None | str:
    """Execute a bazel command. Returns the captured standard output as string if capture_output=True."""

    cmd = shutil.which("bazel") or sys.exit(bazel_not_found_message("red"))
    print(color_message(shlex.join(("bazel", *args)), "bold"), file=sys.stderr)
    return (subprocess.check_output if capture_output else subprocess.check_call)((cmd, *args), text=True)
