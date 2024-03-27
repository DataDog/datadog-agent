from typing import Optional

try:
    from termcolor import colored
except ImportError:

    def colored(text: str, color: Optional[str]) -> str:  # noqa: U100
        return text


import os

import invoke.exceptions as ie


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
