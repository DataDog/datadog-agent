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

    # Prefer the 'bzl' launcher (Datadog wrapper) over plain 'bazel'.
    resolved_bazel = shutil.which("bzl") or shutil.which("bazel")
    if not resolved_bazel:
        raise Exit(bazel_not_found_message("red"))
    launcher = "bzl" if shutil.which("bzl") else "bazel"
    cmd = ("sudo", resolved_bazel) if sudo else (launcher,)
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


def bazel_build_binary(
    ctx: Context,
    label: str,
    platform: str | None = None,
    stamp: bool = True,
) -> str:
    """Build a Bazel binary target and return the path to the built artifact.

    label: Bazel label e.g. "//cmd/agent:agent"
    platform:  optional platform flag e.g. "//bazel/platforms:linux_x86_64"
    stamp: whether to embed version info via workspace_status_command (default True)
    Returns the path to the built binary under bazel-bin.
    """
    import os
    import subprocess

    repo_root = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))
    status_script = os.path.join(repo_root, "bazel", "tools", "workspace_status.sh")

    args = ["build", label]
    if stamp:
        args += ["--stamp", f"--workspace_status_command={status_script}"]
    if platform:
        args += [f"--platforms={platform}"]

    bazel(ctx, *args)

    # Resolve the binary path from bazel-bin.
    # Bazel puts go_binary outputs at bazel-bin/<pkg>/<name>_/<name>
    # e.g. //cmd/agent:agent -> bazel-bin/cmd/agent/agent_/agent
    pkg = label.lstrip("/").split(":")[0]  # "cmd/agent"
    name = label.split(":")[-1] if ":" in label else pkg.split("/")[-1]  # "agent"
    candidates = [
        os.path.join(repo_root, "bazel-bin", pkg, f"{name}_", name),
        os.path.join(repo_root, "bazel-bin", pkg, name),
    ]
    for p in candidates:
        if os.path.isfile(p):
            return p
    # Fallback: search bazel-bin for the executable
    result = subprocess.run(
        ["find", "-L", os.path.join(repo_root, "bazel-bin", pkg), "-name", name, "-type", "f", "-executable"],
        capture_output=True,
        text=True,
    )
    found = [line for line in result.stdout.splitlines() if line]
    if found:
        return found[0]
    raise Exit(f"bazel_build_binary: built {label} but could not find binary under bazel-bin/{pkg}")
