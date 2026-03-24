"""Provides helper functions for invoking Bazel commands."""

from __future__ import annotations

import shlex
import shutil
import subprocess
import sys

from invoke import Exit
from invoke.context import Context

from tasks.libs.common.color import color_message


def bazel_not_found_message(color: str) -> str:
    return color_message("Please run `inv install-tools` for `bazel` support!", color)


def bazel(ctx: Context, *args: str, capture_output: bool = False, sudo: bool = False) -> None | str:
    """Execute a bazel command. Returns the captured standard output as string if capture_output=True."""

    if not shutil.which("bazel"):
        raise Exit(bazel_not_found_message("red"))
    result = (ctx.sudo if sudo else ctx.run)(
        (subprocess.list2cmdline if sys.platform == "win32" else shlex.join)(("bazel", *args)),
        echo=True,
        in_stream=False,
        **({"hide": "out"} if capture_output else {"pty": sys.stdout.isatty() and sys.platform != "win32"}),  # type: ignore[dict-item]
    )
    return result.stdout if capture_output else None
