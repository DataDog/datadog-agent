"""Provides helper functions for invoking Bazel commands."""

from __future__ import annotations

import ast
import os
import shlex
import shutil
import subprocess
import sys
from pathlib import Path

from invoke import Exit
from invoke.context import Context

from tasks.libs.common.color import color_message


def bazel_not_found_message(color: str) -> str:
    return color_message("Please run `inv install-tools` for `bazel` support!", color)


def bazel(
    ctx: Context,
    *args: str,
    capture_output: bool = False,
    sudo: bool = False,
    hide_stderr: bool = False,
    capture_stderr: bool = False,
) -> None | str:
    """Execute a bazel command. Returns captured output as a string if capture_output=True.

    capture_stderr: also capture stderr and append it to the returned string.
        Implies hide_stderr. Use this when Bazel writes important output (e.g.
        test results) to stderr and the caller needs to process it.
    hide_stderr: suppress Bazel's progress/info lines written to stderr without
        capturing them. Only meaningful when capture_output=True and
        capture_stderr=False.
    """

    if not (resolved_bazel := shutil.which("bazel")):
        raise Exit(bazel_not_found_message("red"))
    cmd = ("sudo", resolved_bazel) if sudo else ("bazel",)
    kwargs = {}
    if capture_output:
        kwargs["hide"] = True if (hide_stderr or capture_stderr) else "out"
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
