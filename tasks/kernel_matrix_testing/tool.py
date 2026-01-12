from __future__ import annotations

import os
import sys
from typing import TYPE_CHECKING

import invoke.exceptions as ie
from invoke.context import Context

from tasks.libs.common.color import Color, color_message
from tasks.libs.types.arch import Arch

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import KMTArchName, KMTArchNameOrLocal, PathOrStr


def _logprint(msg: str):
    print(msg, flush=True, file=sys.stderr)


def ask(question: str) -> str:
    return input(color_message(question, Color.BLUE))


def debug(msg: str):
    _logprint(color_message(msg, Color.GREY))


def info(msg: str):
    _logprint(color_message(msg, Color.GREEN))


def warn(msg: str):
    _logprint(color_message(msg, Color.ORANGE))


def error(msg: str):
    _logprint(color_message(msg, Color.RED))


def Exit(msg: str):
    return ie.Exit(color_message(msg, Color.RED))


def is_root():
    return os.getuid() == 0


def get_binary_target_arch(ctx: Context, file: PathOrStr) -> Arch | None:
    res = ctx.run(f"file {file}")
    if res is None or not res.ok:
        return None

    # Only do this with executable files
    if 'executable' not in res.stdout:
        return None

    # Second field of the file output is the architecture, get a standard
    # Get a standard value if possible
    words = [x.strip(",.") for x in res.stdout.split(" ")]
    for word in words:
        try:
            return Arch.from_str(word)
        except KeyError:
            pass

    return None


def convert_kmt_arch_or_local(arch: KMTArchNameOrLocal) -> KMTArchName:
    if arch == "local":
        return Arch.local().kmt_arch
    return arch
