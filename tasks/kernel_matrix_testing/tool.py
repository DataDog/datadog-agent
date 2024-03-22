try:
    from termcolor import colored
except ImportError:
    colored = None

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


def is_root():
    return os.getuid() == 0
