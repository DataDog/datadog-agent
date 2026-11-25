"""
systray tasks
"""

import os
import sys

from invoke import task

from tasks.flavor import FipsBackend
from tasks.libs.build.bazel import build_binary_with_bazel
from tasks.libs.common.color import color_message
from tasks.libs.common.utils import bin_name

# constants
BIN_PATH = os.path.join(".", "bin", "agent")


@task
def build(
    ctx,
    console=False,
    enable_bazel=False,
    fips_backend=FipsBackend.none,
):
    """
    Build the systray binary with Bazel.

    Example invokation:
        dda inv systray.build
    """

    if not sys.platform == 'win32':
        print("Systray only available on Windows")
        return

    if enable_bazel:
        sys.stderr.write(color_message('--enable-bazel is deprecated, Bazel is now always used\n', "orange"))

    build_binary_with_bazel(
        "//cmd/systray:systray",
        args=[f"--//cmd/systray:console={console}"],
        bin_path=os.path.join(BIN_PATH, bin_name("ddtray")),
        fips_backend=fips_backend,
    )


@task
def run(ctx, console=False):
    """
    Build and execute the systray binary.
    """
    build(ctx, console=console)
    ctx.run(os.path.join(BIN_PATH, bin_name("ddtray")))


@task
def clean(ctx):
    """
    Deprecated, does nothing.
    """
    sys.stderr.write(color_message('systray.clean is deprecated and does nothing\n', "orange"))
