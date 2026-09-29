"""Build the feature-branch-only EUDM scenario simulator."""

import os

from invoke import task

from tasks.build_tags import compute_build_tags_for_flavor
from tasks.devcontainer import run_on_devcontainer
from tasks.flavor import AgentFlavor
from tasks.libs.common.constants import REPO_PATH
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import bin_name, get_build_flags


@task
@run_on_devcontainer
def build(ctx, build_include=None, build_exclude="python", rebuild=False, go_mod="readonly"):
    """Build eudm-simulator for native capture or portable staging replay."""
    # Native capture uses Go core checks; the simulator does not embed Python.
    tags = compute_build_tags_for_flavor(
        flavor=AgentFlavor.base,
        build="agent",
        build_include=build_include,
        build_exclude=build_exclude,
    )
    ldflags, gcflags, env = get_build_flags(ctx, include_python="python" in tags)
    go_build(
        ctx,
        f"{REPO_PATH}/cmd/eudm-simulator",
        build_tags=tags,
        ldflags=ldflags,
        gcflags=gcflags,
        env=env,
        rebuild=rebuild,
        mod=go_mod,
        bin_path=os.path.join("bin", "eudm-simulator", bin_name("eudm-simulator")),
    )
