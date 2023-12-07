import datetime
import glob
import os
import platform
import shutil

from invoke import task
from invoke.exceptions import Exit

from .build_tags import get_default_build_tags
from .system_probe import CURRENT_ARCH
from .utils import (
    REPO_PATH,
    bin_name,
    get_build_flags,
    get_git_branch_name,
    get_git_commit,
    get_go_version,
    get_version,
)

BIN_DIR = os.path.join(".", "bin")
BIN_PATH = os.path.join(BIN_DIR, "profiler", bin_name("profiler"))
CONTAINER_PLATFORM_MAPPING = {"aarch64": "arm64", "amd64": "amd64", "x86_64": "amd64"}


@task(iterable=["build_tags"])
def build(
    ctx,
    build_tags,
    race=False,
    incremental_build=True,
    major_version='7',
    # arch is never used here; we keep it to have a
    # consistent CLI on the build task for all agents.
    arch=CURRENT_ARCH,  # noqa: U100
    go_mod="mod",
    static=False,
):
    """
    Build profiler
    """
    ldflags, gcflags, env = get_build_flags(ctx, major_version=major_version, python_runtimes='3', static=static)

    # TODO use pkg/version for this
    main = "main."
    ld_vars = {
        "Version": get_version(ctx, major_version=major_version),
        "GoVersion": get_go_version(),
        "GitBranch": get_git_branch_name(),
        "GitCommit": get_git_commit(),
        "BuildDate": datetime.datetime.now().strftime("%Y-%m-%dT%H:%M:%S"),
    }

    ldflags += ' '.join([f"-X '{main + key}={value}'" for key, value in ld_vars.items()])
    build_tags += get_default_build_tags(
        build="profiler"
    )  # TODO/FIXME: Arch not passed to preserve build tags. Should this be fixed?
    build_tags.append("netgo")
    build_tags.append("osusergo")

    race_opt = "-race" if race else ""
    build_type = "" if incremental_build else "-a"
    go_build_tags = " ".join(build_tags)
    agent_bin = BIN_PATH

    cmd = (
        f'go build -mod={go_mod} {race_opt} {build_type} -tags "{go_build_tags}" '
        f'-o {agent_bin} -gcflags="{gcflags}" -ldflags="{ldflags} -s -w" {REPO_PATH}/cmd/profiler'
    )

    ctx.run(cmd, env=env)

