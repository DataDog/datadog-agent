"""
Build task for the par-executor binary (PAR dual-process execution plane).
Mirrors tasks/privateactionrunner.py — same build flags, Linux-only.
"""

import os

from invoke.tasks import task

from tasks.build_tags import get_default_build_tags
from tasks.devcontainer import run_on_devcontainer
from tasks.flavor import AgentFlavor
from tasks.libs.common.constants import REPO_PATH
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import bin_name, get_build_flags

BIN_DIR = os.path.join(".", "bin", "par-executor")
BIN_PATH = os.path.join(BIN_DIR, bin_name("par-executor"))


@task
@run_on_devcontainer
def build(
    ctx,
    install_path=None,
    flavor=AgentFlavor.base.name,
    rebuild=False,
    go_mod="readonly",
):
    """Build the par-executor binary (PAR dual-process execution plane)."""
    ldflags, gcflags, env = get_build_flags(ctx, install_path=install_path)

    # Re-use the same build tags as privateactionrunner — same bundle set.
    build_tags = get_default_build_tags(build="privateactionrunner", flavor=AgentFlavor[flavor])

    go_build(
        ctx,
        f"{REPO_PATH}/cmd/par-executor",
        build_tags=build_tags,
        ldflags=ldflags,
        gcflags=gcflags,
        rebuild=rebuild,
        env=env,
        bin_path=BIN_PATH,
        mod=go_mod,
    )
