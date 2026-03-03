"""
Helpers for setting up your environment
"""

from __future__ import annotations

import re
import sys
import traceback
from collections.abc import Iterable
from dataclasses import dataclass

from invoke import task
from invoke.exceptions import Exit

from tasks import vscode
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.git import get_default_branch
from tasks.libs.common.status import Status
from tasks.libs.common.utils import is_linux, is_windows, running_in_pyapp


@dataclass
class SetupResult:
    name: str
    status: str
    message: str = ""


@task(default=True)
def setup(ctx, vscode=False):
    """
    Set up your environment
    """
    setup_functions = [
        check_git_repo,
        check_python_version,
        check_go_version,
        update_ddtool,
        install_go_tools,
        install_protoc,
        enable_pre_commit,
    ]

    if vscode:
        setup_functions.append(setup_vscode)
    else:
        print(
            f'{color_message("warning:", Color.ORANGE)} Skipping vscode setup, run `dda inv setup --vscode` to setup vscode as well'
        )

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
            print(color_message(result.message, Color.ORANGE))

    print()

    if final_result == Status.OK:
        print(color_message("Setup completed successfully.", Color.GREEN))
    elif final_result == Status.WARN:
        print(color_message("Setup completed with warnings.", Color.ORANGE))
    else:
        print(color_message("Setup completed with errors.", Color.RED))
        raise Exit(code=1)


def check_git_repo(ctx) -> SetupResult:
    """Check if the git repository is up to date."""
    print(color_message("Fetching git repository...", Color.BLUE))
    ctx.run("git fetch", hide=True)

    print(color_message("Checking main branch...", Color.BLUE))
    output = ctx.run(f'git rev-list "^HEAD" origin/{get_default_branch()} --count', hide=True)
    count = output.stdout.strip()

    message = ""
    status = Status.OK

    if count != "0":
        status = Status.WARN
        message = f"Your branch is {count} commit(s) behind {get_default_branch()}. Please update your branch."

    return SetupResult("Check git repository", status, message)


def check_go_version(ctx) -> SetupResult:
    """Check if the Go version is up to date."""
    print(color_message("Checking Go version...", Color.BLUE))

    with open(".go-version") as f:
        expected_version = f.read().strip()

    try:
        output = ctx.run("go version", hide=True)
    except Exception:
        return SetupResult(
            "Check Go version", Status.FAIL, f"Go is not installed. Please install Go {expected_version}."
        )

    version = re.search(r'go version go(\d+.\d+.\d+)', output.stdout)
    assert version, f"Could not parse Go version from '{output.stdout}'"

    if version.group(1) != expected_version:
        return SetupResult(
            "Check Go version",
            Status.WARN,
            f"The Go version is {version.group(1)}. Please install Go {expected_version}.",
        )

    return SetupResult("Check Go version", Status.OK)


def check_python_version(_ctx) -> SetupResult:
    """Check if the Python version is up to date."""
    print(color_message("Checking Python version...", Color.BLUE))

    with open(".python-version") as f:
        expected_version = f.read().strip()

    message = ""
    status = Status.OK
    if tuple(sys.version_info)[:2] != tuple(int(d) for d in expected_version.split(".")):
        status = Status.FAIL
        install_message = f"Please install Python {expected_version} with 'brew install python@{expected_version}'"
        if is_windows():
            install_message = f"Please install Python {expected_version} from https://www.python.org/downloads/windows/"
        elif is_linux():
            install_message = (
                f"Please install Python {expected_version} with 'sudo apt-get install python{expected_version}-dev'"
            )

        message = f"Python version out of date, current is {sys.version_info[0]}.{sys.version_info[1]} while expected is {expected_version}.\n{install_message}"

    return SetupResult("Check Python version", status, message)


@task
def pre_commit(ctx, interactive=True):
    """Will set up pre-commit hooks.

    Note:
        pre-commit hooks will be uninstalled / cleaned before being installed.
    """

    print(color_message("Enabling pre-commit...", Color.BLUE))

    status = Status.OK
    message = ""

    if not ctx.run("pre-commit --version", hide=True, warn=True).ok:
        message = "Please install pre-commit binary first: https://pre-commit.com/#installation."
        status = Status.FAIL

        if interactive:
            raise Exit(code=1, message=f'{color_message("Error:", Color.RED)} {message}')

        return Status.FAIL, message

    try:
        # Some dd-hooks can mess up with pre-commit
        hooks_path = ctx.run("git config --global core.hooksPath", hide=True).stdout.strip()
        ctx.run("git config --global --unset core.hooksPath", hide=True)
    except Exception:
        hooks_path = ""

    if running_in_pyapp():
        import shutil

        # We use a custom version that use deva instead of dda inv directly, that requires the venv to be loaded
        from pre_commit import update_pyapp_file

        config_file = update_pyapp_file()
        if not shutil.which("dda"):
            status = Status.WARN
            if shutil.which("devagent"):
                message = "`devagent` has been renamed `dda`. Please, rename your binary, no need to download the new version."
            else:
                message = "`dda` is not in your PATH"
    else:
        config_file = ".pre-commit-config.yaml"

    # Uninstall in case someone switch from one config to the other
    ctx.run("pre-commit uninstall", hide=True)
    ctx.run("pre-commit clean", hide=True)
    # build-vcs avoids errors when getting go dependencies
    ctx.run(f"GOFLAGS=-buildvcs=false pre-commit install --config {config_file}", hide=True)

    if hooks_path:
        ctx.run(f"git config --global core.hooksPath {hooks_path}", hide=True)

    if interactive:
        if message:
            print(f'{color_message("Warning:", Color.ORANGE)} {message}')

        print(color_message("Pre-commit installed and enabled.", Color.GREEN))

    return status, message


def enable_pre_commit(ctx) -> SetupResult:
    """Enable pre-commit hooks."""
    status, message = pre_commit(ctx, interactive=False)

    return SetupResult("Enable pre-commit", status, message)


def setup_vscode(ctx) -> SetupResult:
    """Set up VS Code."""
    print(color_message("Setting up VS Code...", Color.BLUE))

    try:
        vscode.setup(ctx, force=True)
        message = "VS Code setup completed."
        status = Status.OK
    except Exception:
        trace = traceback.format_exc()
        message = f'VS Code setup failed:\n{trace}'
        status = Status.FAIL

    return SetupResult("Setup vscode", status, message)


def update_ddtool(ctx) -> SetupResult:
    """Update ddtool."""
    print(color_message("Updating ddtool...", Color.BLUE))
    status = Status.OK
    message = ""

    try:
        ctx.run('brew update && brew upgrade ddtool', hide=True)
    except Exception as e:
        message = f'Ddtool update failed: {e}'
        status = Status.FAIL

    return SetupResult("Update ddtool", status, message)


def install_go_tools(ctx) -> SetupResult:
    """Install go tools."""
    print(color_message("Installing go tools...", Color.BLUE))
    status = Status.OK
    message = ""

    try:
        from tasks import install_tools

        install_tools(ctx)
    except Exception as e:
        message = f'Go tools setup failed: {e}'
        status = Status.FAIL

    return SetupResult("Install Go tools", status, message)


def install_protoc(ctx) -> SetupResult:
    """Install protoc."""
    print(color_message("Installing protoc...", Color.BLUE))
    status = Status.OK
    message = ""

    try:
        from tasks import install_protoc

        install_protoc(ctx)
    except Exception as e:
        message = f'Protoc setup failed: {e}'
        status = Status.FAIL

    return SetupResult("Install protoc", status, message)
