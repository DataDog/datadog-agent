"""Provides helper functions for invoking Bazel commands."""

from __future__ import annotations

import ast
import os
import shlex
import shutil
import subprocess
import sys
from pathlib import Path
from typing import NamedTuple

from invoke import Exit
from invoke.context import Context

from tasks.libs.common.color import color_message


class LabelParts(NamedTuple):
    """Component parts of a Bazel label."""

    repo: str | None
    package: str
    name: str | None


def split_label(label: str) -> LabelParts:
    """Split a Bazel label into its component parts.

    The canonical format is ``@@repo//package/path:name``, where the repo
    prefix is optional and may carry one or two leading ``@`` characters.

    Returns a :class:`LabelParts` namedtuple with:
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
        name = label[colon_idx + 1:]
    else:
        package = label
        name = None

    return LabelParts(repo=repo, package=package, name=name)


def package_from_path(path: str) -> str:
    """Return the Bazel package string corresponding to a filesystem path.

    - Relative paths are normalised: backslashes become forward slashes and
      any leading ``./`` is removed.
    - Absolute paths are made relative to the workspace root.

    Forward slashes are always used in the result, regardless of OS.
    """
    from tasks.libs.common.utils import get_repo_root  # avoid top-level circular import risk

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


def bazel(ctx: Context, *args: str, capture_output: bool = False, sudo: bool = False) -> None | str:
    """Execute a bazel command. Returns the captured standard output as string if capture_output=True."""

    if not (resolved_bazel := shutil.which("bazel")):
        raise Exit(bazel_not_found_message("red"))
    cmd = ("sudo", resolved_bazel) if sudo else ("bazel",)
    kwargs = {}
    if capture_output:
        kwargs["hide"] = "out"
    elif not sudo and sys.stdout.isatty() and sys.platform != "win32":
        kwargs["pty"] = True
    result = ctx.run(
        (subprocess.list2cmdline if sys.platform == "win32" else shlex.join)(cmd + args),
        echo=True,
        in_stream=False,
        **kwargs,
    )
    return result.stdout if capture_output else None


class BazelTools:
    """Hermetic Bazel-managed tool paths; populated once on first instantiation."""

    _paths = {}

    def __new__(cls, ctx):
        if not cls._paths:
            labels = (
                "//bazel/toolchains/protoc",
                "@com_github_favadi_protoc_go_inject_tag//:protoc-go-inject-tag",
                "@com_github_golang_mock//mockgen",
                "@com_github_planetscale_vtprotobuf//cmd/protoc-gen-go-vtproto",
                "@com_github_tinylib_msgp//:msgp",
                "@org_golang_google_grpc_cmd_protoc_gen_go_grpc//:protoc-gen-go-grpc",
                "@org_golang_google_protobuf//cmd/protoc-gen-go",
                "@rules_go//go",
            )
            bazel(ctx, "build", *labels)
            root = bazel(ctx, "info", "execution_root", capture_output=True).strip()
            for line in bazel(
                ctx,
                "cquery",
                f"config(set({' '.join(labels)}), target)",
                "--output=starlark",
                "--starlark:expr=target.label.name,target.files_to_run.executable.path",
                capture_output=True,
            ).splitlines():
                name, path = ast.literal_eval(line)
                cls._paths[name] = Path(root, path)
        return super().__new__(cls)

    def __getattr__(self, name):
        return self._paths[name.replace("_", "-")]

    @property
    def go_env(self):
        return {"PATH": f"{self._paths['go_bin_runner'].parent}{os.pathsep}{os.getenv('PATH', '')}"}

    def protoc_plugin(self, name):
        return f"--plugin={name}={self._paths[name]}"
