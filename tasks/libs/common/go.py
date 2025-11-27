from __future__ import annotations

import io
import os
import os.path
import platform
import sys
from pathlib import Path
from typing import cast

from invoke.context import Context
from invoke.runners import Local, Result

from tasks.libs.common.retry import run_command_with_retry
from tasks.libs.common.utils import timed


def download_go_dependencies(ctx: Context, paths: list[str], verbose: bool = False, max_retry: int = 3):
    print("downloading dependencies")
    with timed("go mod download && go mod tidy"):
        verbosity = ' -x' if verbose else ''
        for path in paths:
            with ctx.cd(path):
                run_command_with_retry(
                    ctx, f"go mod download{verbosity} && go mod tidy{verbosity}", max_retry=max_retry
                )


def go_build(
    ctx: Context,
    entrypoint: str | Path,
    mod: str | None = None,
    race: bool = False,
    gcflags: str | None = None,
    ldflags: str | None = None,
    build_tags: list[str] | None = None,
    rebuild: bool = False,
    env: dict[str, str] | None = None,
    bin_path: str | Path | None = None,
    verbose: bool = False,
    echo: bool = False,
    check_deadcode: bool = False,
    coverage: bool = False,
    trimpath: bool = True,
) -> Result:
    cmd = "go build"
    if coverage:
        cmd += " -cover -covermode=atomic"
        build_tags = build_tags or []
        build_tags.append("e2ecoverage")
    if mod:
        cmd += f" -mod={mod}"
    if race:
        cmd += " -race"
    if rebuild:
        cmd += " -a"
    if verbose:
        cmd += " -v"
    if echo:
        cmd += " -x"
    if build_tags:
        cmd += f" -tags \"{' '.join(build_tags)}\""
    if bin_path:
        cmd += f" -o {bin_path}"
    if gcflags:
        cmd += f" -gcflags=\"{gcflags}\""
    if check_deadcode:
        ldflags = (ldflags or "") + " -dumpdep"
    if ldflags:
        cmd += f" -ldflags=\"{ldflags}\""
    if trimpath and 'DELVE' not in os.environ:
        cmd += " -trimpath"

    cmd += f" {entrypoint}"

    if check_deadcode:
        result = _handle_pipe_to_whydeadcode(ctx, os.path.basename(entrypoint), cmd, env)
    else:
        result = cast(Result, ctx.run(cmd, env=env))

    if sys.platform == "win32" or result.exited != 0 or bin_path is None:
        return result

    if os.path.exists(bin_path):
        uid = os.environ.get("HOST_UID", "-1")
        gid = os.environ.get("HOST_GID", "-1")
        if uid != "-1" and gid != "-1":
            os.chown(bin_path, int(uid), int(gid))

    return result


def _handle_pipe_to_whydeadcode(ctx: Context, name: str, cmd: str, env: dict[str, str] | None = None) -> Result:
    """
    - Runs `go build` with the `dumpdep` flag in a custom runner. This runner reads big chunks to improve invoke I/O performance, see https://github.com/pyinvoke/invoke/issues/774
    - Calls `whydeadcode` in the same runner using an associated custom reader.
    """
    runner = Local(ctx)
    runner.read_chunk_size = 1024 * 1024 * 10
    _ = runner.read_chunk_size  # please linters
    runner.input_sleep = 0
    _ = runner.input_sleep  # please linters

    # -dumpdep is very verbose so we hide that
    # any unrecognized log line is shown by whydeadcode anyway
    result = cast(Result, runner.run(cmd, env=env, hide="stderr"))

    # worst case it's already installed and nothing happens
    with ctx.cd("internal/tools"):
        # pass the env to the command so that it can check GOPATH/GOBIN if provided
        ctx.run("go install github.com/aarzilli/whydeadcode", env=env)

    # whydeadcode prints unexpected input on stderr (eg. build warnings), and
    # dead code call stack on stdout
    # it returns non-zero if non-expected input is passed, and 0 otherwise, even if dead code elimination is disabled
    # so we check whether stdout is empty to know if dead code elimination is disabled
    whydeadcoderes = cast(
        Result, runner.run("whydeadcode", in_stream=CustomReader(result.stderr), warn=True, hide="out", env=env)
    )
    if whydeadcoderes.stdout:
        arch = platform.machine()
        osname = sys.platform
        print(
            f"dead code elimination is disabled for {name} on {osname} {arch} by the following call stack (only the first one is guaranteed to be a true positive):\n{whydeadcoderes.stdout}"
        )

    return result


class CustomReader(io.StringIO):
    """
    Custom reader to read 10MiB at a time.
    This is a workaround to increase invoke performance at reading from stdin
    See https://github.com/pyinvoke/invoke/issues/819
    """

    def __init__(self, data: str):
        super().__init__(data)

    def read(self, n: int | None = None) -> str:
        return super().read(1024 * 1024 * 10)
