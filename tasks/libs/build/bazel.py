"""Provides helper functions for invoking Bazel commands."""

from __future__ import annotations

import shlex
import shutil
import subprocess
import sys
from pathlib import Path
from typing import NamedTuple

from invoke import Exit
from invoke.context import Context

from tasks.libs.common.color import color_message
from tasks.libs.common.utils import get_repo_root


class Label(NamedTuple):
    """Component parts of a Bazel label."""

    repo: str | None
    package: str
    name: str | None


def split_label(label: str) -> Label:
    """Split a Bazel label into its component parts.

    The canonical format is ``@@repo//package/path:name``, where the repo
    prefix is optional and may carry one or two leading ``@`` characters.

    Returns a :class:`Label` namedtuple with:
    - ``repo``: the repository name, or ``None`` when absent (main workspace).
    - ``package``: the package path (empty string ``""`` for root labels such
      as ``//:foo``).
    - ``name``: the target name after ``:``, or ``None`` when omitted.
    """
    repo = None

    if label.startswith('@'):
        # Strip one or two leading '@' characters to reach "repo//…"
        rest = label.lstrip('@')
        slash_idx = rest.find('//')
        if slash_idx >= 0:
            repo_part = rest[:slash_idx]
            # Empty repo part (e.g. "@//" or "@@//") means the main workspace
            repo = repo_part if repo_part else None
            label = rest[slash_idx:]

    # label is now "//package:name" or "//package"
    if label.startswith('//'):
        label = label[2:]

    colon_idx = label.find(':')
    if colon_idx >= 0:
        package = label[:colon_idx]
        name = label[colon_idx + 1 :]
    else:
        package = label
        name = None

    return Label(repo=repo, package=package, name=name)


def package_from_path(path: str) -> str:
    """Return the Bazel package string corresponding to a filesystem path.

    - Relative paths are normalised: backslashes become forward slashes and
      any leading ``./`` is removed.
    - Absolute paths are made relative to the workspace root.

    Forward slashes are always used in the result, regardless of OS.
    """

    # Normalise backslashes to forward slashes before constructing Path so
    # that Windows-style separators are treated as directory separators on all
    # platforms (on POSIX, Path does not interpret '\' as a separator).
    normalised = path.replace('\\', '/')
    p = Path(normalised)
    if p.is_absolute():
        p = p.relative_to(get_repo_root())

    result = p.as_posix()
    # Path('.') represents "current directory" (i.e. the root package)
    if result == '.':
        return ''
    return result


def bazel_not_found_message(color: str) -> str:
    return color_message("Please run `inv install-tools` for `bazel` support!", color)


def bazel(
    ctx: Context,
    *args: str,
    capture_output: bool = False,
    sudo: bool = False,
    capture_stderr: bool = False,
) -> None | str:
    """Execute a bazel command.

    capture_output: capture stdout.
    capture_stderr: also capture stderr and append it to the returned string.
        Use this when Bazel writes important output (e.g.  test results) to stderr
        and the caller needs to process it.
    """

    if not (resolved_bazel := shutil.which("bazel")):
        raise Exit(bazel_not_found_message("red"))
    cmd = ("sudo", resolved_bazel) if sudo else ("bazel",)
    kwargs = {}
    # Invoke terminolgy is subtle. "hide" means hide from the user.
    # In every other libray, that would be called capture, and the
    # act of capturing it would hide it from the user.
    # https://docs.pyinvoke.org/en/stable/api/runners.html#invoke.runners.Runner.run
    if capture_output:
        kwargs["hide"] = "both" if capture_stderr else "out"
    elif not sudo and sys.stdout.isatty() and sys.platform != "win32":
        kwargs["pty"] = True
    result = ctx.run(
        (subprocess.list2cmdline if sys.platform == "win32" else shlex.join)(cmd + args),
        echo=True,
        in_stream=False,
        **kwargs,
    )
    if not capture_output:
        return None
    if capture_stderr:
        return (result.stdout or "") + (result.stderr or "")
    return result.stdout
