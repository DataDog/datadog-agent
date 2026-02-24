"""Provides helper functions for invoking Bazel commands."""

from __future__ import annotations

import shutil
import subprocess
import sys

from tasks.libs.common.utils import get_repo_root


def bazel(*args: str, capture_output: bool = False) -> None | str:
    """Execute a bazel command. Returns the captured standard output as string if capture_output=True."""

    cmd = shutil.which("bazel") or sys.exit((get_repo_root() / "tools" / "bazelisk.md").read_text())
    return (subprocess.check_output if capture_output else subprocess.check_call)((cmd, *args), text=True)
