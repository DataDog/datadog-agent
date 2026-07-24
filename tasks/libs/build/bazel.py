"""Provides helper functions for invoking Bazel commands."""

from __future__ import annotations

import os
import shlex
import shutil
import subprocess
import sys
from pathlib import Path
from typing import IO, NamedTuple

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
    capture_stderr: bool = False,
    ignore_errors: bool = False,
    input_stream: IO[str] | bool = False,
    sudo: bool = False,
) -> str:
    """Execute a bazel command.

    capture_output: capture stdout.
    capture_stderr: also capture stderr and append it to the returned string.
        Use this when Bazel writes important output (e.g.  test results) to stderr
        and the caller needs to process it.
    """

    if not (bazelisk := shutil.which("bazelisk")):  # `/usr/bin/bazel` may otherwise take precedence in DD Workspaces
        raise Exit(bazel_not_found_message("red"))
    cmd = (("sudo",) if sudo else ()) + (bazelisk, *_insert_omnibazel_flags(args))
    cmdline = (subprocess.list2cmdline if sys.platform == "win32" else shlex.join)(cmd)
    print(color_message(cmdline.replace(bazelisk, "bazel", 1), "bold"), file=sys.stderr)  # brevity: abspath -> bazel
    kwargs = {}
    # Invoke terminolgy is subtle. "hide" means hide from the user.
    # In every other libray, that would be called capture, and the
    # act of capturing it would hide it from the user.
    # https://docs.pyinvoke.org/en/stable/api/runners.html#invoke.runners.Runner.run
    if capture_output and capture_stderr:
        kwargs["hide"] = "both"
    elif capture_output:
        kwargs["hide"] = "out"
    elif capture_stderr:
        kwargs["hide"] = "err"
    elif not sudo and sys.stdout.isatty() and sys.platform != "win32":
        kwargs["pty"] = True
    result = ctx.run(cmdline, echo=False, in_stream=input_stream, warn=ignore_errors, **kwargs)
    captured = []
    if capture_output and result.ok:
        captured.append(result.stdout)
    if capture_stderr:
        captured.append(result.stderr)
    return "".join(captured)


def _insert_omnibazel_flags(args: tuple[str, ...]) -> tuple[str, ...]:
    """Insert --//packages/agent:flavor, --//:install_dir and --//:output_config_dir, pinned from the corresponding
    omnibus build environment variables.
    💡 Mirrors `omnibazel_flags` in omnibus/lib/ostools.rb.
    """
    flags = []
    if agent_flavor := os.environ.get("AGENT_FLAVOR"):
        flags.append(f"--//packages/agent:flavor={agent_flavor}")
    if install_dir := os.environ.get("INSTALL_DIR"):
        flags.append(f"--//:install_dir={install_dir}")
        flags.append(f"--//:output_config_dir={os.environ.get("OUTPUT_CONFIG_DIR", "")}")
    if not flags:
        return args
    # insert flags right after the bazel command, preserving startup options before it and subcommand arguments after it
    index = next((i for i, a in enumerate(args, 1) if not a.startswith("-")), len(args))
    return (*args[:index], *flags, *args[index:])
