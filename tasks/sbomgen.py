import os

from invoke.tasks import task

from tasks.build_tags import get_default_build_tags
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import (
    REPO_PATH,
    get_build_flags,
)

BIN_DIR = os.path.join(".", "bin")
BIN_PATH = os.path.join(BIN_DIR, "sbomgen", "sbomgen")


@task
def build(
    ctx,
    dumpdep=False,
    install_path=None,
    major_version='7',
    static=False,
):
    """
    Build the sbomgen binary
    """

    ldflags, gcflags, env = get_build_flags(ctx, major_version=major_version, static=static, install_path=install_path)
    ldflags += "-s -w"
    if dumpdep:
        ldflags += " -dumpdep"

    build_tags = get_default_build_tags(build="sbomgen")

    if os.path.exists(BIN_PATH):
        os.remove(BIN_PATH)

    go_build(
        ctx,
        f"{REPO_PATH}/cmd/sbomgen",
        mod="readonly",
        gcflags=gcflags,
        ldflags=ldflags,
        build_tags=build_tags,
        bin_path=BIN_PATH,
        env=env,
    )

    ctx.run(f"ls -alh {BIN_PATH}")
