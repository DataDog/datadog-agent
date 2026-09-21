# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

import os
from pathlib import Path

from invoke import task

from tasks.libs.common.utils import join_command

# Keep in sync with deps/foldspace/foldspace.MODULE.bazel.
# DataDog/foldspace PR 78 (ryan.hall/exclusive-reliable-class).
FOLDSPACE_GIT_REMOTE = "https://github.com/DataDog/foldspace.git"
FOLDSPACE_GIT_COMMIT = "30d68b0b982fa70466f74a1c82ce70e9b570da31"
FOLDSPACE_PACKAGE = "foldspace-go-ffi"


def _foldspace_src(ctx, source):
    src = source or os.environ.get("FOLDSPACE_SRC", "")
    if src:
        return src
    return _checkout_pinned(ctx)


def _checkout_pinned(ctx):
    cache = Path(os.environ.get("XDG_CACHE_HOME", Path.home() / ".cache")) / "datadog-agent" / "foldspace" / FOLDSPACE_GIT_COMMIT
    git_dir = cache / ".git"
    if git_dir.is_dir():
        head = ctx.run(
            join_command(["git", "-C", str(cache), "rev-parse", "HEAD"]),
            hide=True,
            warn=True,
            encoding="utf-8",
        )
        if head.ok and head.stdout.strip() == FOLDSPACE_GIT_COMMIT:
            return str(cache)
    else:
        cache.parent.mkdir(parents=True, exist_ok=True)
        ctx.run(join_command(["git", "init", str(cache)]), encoding="utf-8")
        ctx.run(
            join_command(["git", "-C", str(cache), "remote", "add", "origin", FOLDSPACE_GIT_REMOTE]),
            encoding="utf-8",
        )

    ctx.run(
        join_command(["git", "-C", str(cache), "fetch", "--depth=1", "origin", FOLDSPACE_GIT_COMMIT]),
        encoding="utf-8",
    )
    ctx.run(
        join_command(["git", "-C", str(cache), "checkout", "--detach", FOLDSPACE_GIT_COMMIT]),
        encoding="utf-8",
    )
    return str(cache)


@task
def build(ctx, source="", release=False):
    """Build libfoldspace_go from DataDog/foldspace at the pinned commit.

    source: local foldspace checkout (or FOLDSPACE_SRC). When omitted, clones
    https://github.com/DataDog/foldspace.git at the commit recorded in
    deps/foldspace/foldspace.MODULE.bazel (the same pin Bazel fetches as
    @foldspace). The resulting library is meant to be passed via CGO_LDFLAGS
    when compiling the agent with --build-include=foldspace.
    """
    src = _foldspace_src(ctx, source)

    profile = ["--release"] if release else []
    cmd = join_command(["cargo", "build", "-p", FOLDSPACE_PACKAGE, *profile])
    with ctx.cd(src):
        ctx.run(cmd, encoding="utf-8")
    target = "release" if release else "debug"
    print(f"built {src}/target/{target}/libfoldspace_go")
