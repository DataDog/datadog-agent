from __future__ import annotations

import os
import platform
from typing import TYPE_CHECKING, Optional

import invoke.exceptions as ie
from invoke.context import Context

from tasks.kernel_matrix_testing.vars import arch_mapping

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import Arch

try:
    from termcolor import colored
except ImportError:

    def colored(text: str, color: Optional[str]) -> str:  # noqa: U100
        return text


def ask(question: str) -> str:
    return input(colored(question, "blue"))


def debug(msg: str):
    print(colored(msg, "white"))


def info(msg: str):
    print(colored(msg, "green"))


def warn(msg: str):
    print(colored(msg, "yellow"))


def error(msg: str):
    print(colored(msg, "red"))


def Exit(msg: str):
    return ie.Exit(colored(msg, "red"))


def NoLibvirt():
    return Exit(
        "libvirt python module not installed. Install with 'pip install -r tasks/kernel_matrix_testing/requirements.txt'"
    )


def is_root():
    return os.getuid() == 0


def full_arch(arch: str):
    if arch == "local":
        return arch_mapping[platform.machine()]
    return arch_mapping[arch]


def get_binary_target_arch(ctx: Context, file: str) -> Optional[Arch]:
    res = ctx.run(f"file {file}")
    if res is None or not res.ok:
        return None

    # Only do this with executable files
    if 'executable' not in res.stdout:
        return None

    # Second field of the file output is the architecture, get a standard
    # value if possible
    binary_arch = res.stdout.split(",")[1].strip()
    for word in binary_arch.split(" "):
        if word in arch_mapping:
            return arch_mapping[word]

    return None
