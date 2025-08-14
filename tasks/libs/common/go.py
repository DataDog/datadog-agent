from __future__ import annotations

import os
import sys
from pathlib import Path
from typing import TYPE_CHECKING

from tasks.libs.common.retry import run_command_with_retry
from tasks.libs.common.utils import timed

if TYPE_CHECKING:
    from invoke import Context
    from invoke.runners import Result


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
    coverage: bool = False,
    trimpath: bool = True,
) -> Result:
    cmd = "go build"
    if coverage:
        cmd += " -cover -covermode=atomic"
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
    if ldflags:
        cmd += f" -ldflags=\"{ldflags}\""
    if trimpath and 'DELVE' not in os.environ:
        cmd += " -trimpath"

    cmd += f" {entrypoint}"

    result = ctx.run(cmd, env=env)
    if sys.platform == "win32" or result.exited != 0 or bin_path is None:
        return result

    if os.path.exists(bin_path):
        uid = os.environ.get("HOST_UID", "-1")
        gid = os.environ.get("HOST_GID", "-1")
        if uid != "-1" and gid != "-1":
            os.chown(bin_path, int(uid), int(gid))

    return result
