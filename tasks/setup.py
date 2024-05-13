"""
Helpers for setting up your environment
"""

from __future__ import annotations

import re
import sys
from dataclasses import dataclass
from typing import Iterable

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.color import color_message
from tasks.libs.common.status import Status

PYTHON_VERSION = "3.9"


@dataclass
class SetupResult:
    name: str
    status: str
    message: str = ""


@task(default=True)
def setup(ctx):
    """
    Set up your environment
    """
    setup_functions = [
        check_python_version,
        check_go_version,
        check_git_repo,
        update_dependencies,
        enable_pre_commit,
    ]

    results = []

    for setup_function in setup_functions:
        output = setup_function(ctx)

        if isinstance(output, Iterable):
            results.extend(output)
        else:
            results.append(output)

    print("\nResults:\n")

    final_result = Status.OK

    for result in results:
        if result.status == Status.FAIL:
            final_result = Status.FAIL
        elif result.status == Status.WARN and final_result == Status.OK:
            final_result = Status.WARN

        print(f"{result.name}\t {color_message(result.status, Status.color(result.status))}")
        if result.message:
            print(color_message(result.message, "orange"))

    print()

    if final_result == Status.OK:
        print(color_message("Setup completed successfully.", "green"))
    elif final_result == Status.WARN:
        print(color_message("Setup completed with warnings.", "orange"))
    else:
        print(color_message("Setup completed with errors.", "red"))
        raise Exit(code=1)


def check_git_repo(ctx) -> SetupResult:
    print(color_message("Fetching git repository...", "blue"))
    ctx.run("git fetch", hide=True)

    print(color_message("Checking main branch...", "blue"))
    output = ctx.run("git rev-list ^HEAD origin/main --count", hide=True)
    count = output.stdout.strip()

    message = ""
    status = Status.OK

    if count != "0":
        status = Status.WARN
        message = f"Your branch is {count} commit(s) behind main. Please update your branch."

    return SetupResult("Check git repository", status, message)


def check_go_version(ctx) -> SetupResult:
    print(color_message("Checking Go version...", "blue"))

    with open(".go-version") as f:
        expected_version = f.read().strip()

    try:
        output = ctx.run("go version", hide=True)
    except:
        return SetupResult(
            "Check Go version", Status.FAIL, f"Go is not installed. Please install Go {expected_version}."
        )

    version = re.search(r'go version go(\d+.\d+.\d+)', output.stdout)

    if version.group(1) != expected_version:
        return SetupResult(
            "Check Go version", Status.FAIL, f"Go version is {version.group(1)}. Please install Go {expected_version}."
        )

    return SetupResult("Check Go version", Status.OK)


def check_python_version(_ctx) -> SetupResult:
    print(color_message("Checking Python version...", "blue"))

    message = ""
    status = Status.OK

    if sys.version_info < tuple(int(d) for d in PYTHON_VERSION.split(".")):
        status = Status.FAIL
        message = f"Python version is {sys.version_info[0]}.{sys.version_info[1]}.{sys.version_info[2]}. Please install at least Python {PYTHON_VERSION}."

    return SetupResult("Check Python version", status, message)


def update_dependencies(ctx) -> Iterable[SetupResult]:
    print(color_message("Updating dependencies...", "blue"))

    requirement_files = ["requirements.txt", "tasks/requirements.txt"]

    for requirement_file in requirement_files:
        print(color_message(f"Updating {requirement_file}...", "blue"))

        ctx.run(f"pip install -r {requirement_file}", hide=True)
        yield SetupResult(f"Update dependencies from {requirement_file}", Status.OK)


def enable_pre_commit(ctx) -> SetupResult:
    print(color_message("Enabling pre-commit...", "blue"))

    if not ctx.run("pre-commit --version", hide=True, warn=True).ok:
        return SetupResult(
            "Enable pre-commit", Status.FAIL, "Please install pre-commit first: https://pre-commit.com/#installation."
        )

    try:
        # Some dd-hooks can mess up with pre-commit
        hooks_path = ctx.run("git config --global core.hooksPath", hide=True).stdout.strip()
        ctx.run("git config --global --unset core.hooksPath", hide=True)
    except Exception:
        hooks_path = ""

    ctx.run("pre-commit install", hide=True)

    if hooks_path:
        ctx.run(f"git config --global core.hooksPath {hooks_path}", hide=True)

    return SetupResult("Enable pre-commit", Status.OK)
