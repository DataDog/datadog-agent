import os

from invoke.tasks import task

from tasks.libs.common.go import go_build
from tasks.libs.common.utils import (
    REPO_PATH,
    get_build_flags,
)

BIN_DIR = os.path.join(".", "bin")
BIN_PATH = os.path.join(BIN_DIR, "safe-shell", "safe-shell")


@task
def build(
    ctx,
    install_path=None,
    static=False,
):
    """
    Build the safe-shell binary
    """

    ldflags, gcflags, env = get_build_flags(ctx, static=static, install_path=install_path)
    ldflags += "-s -w"

    if os.path.exists(BIN_PATH):
        os.remove(BIN_PATH)

    go_build(
        ctx,
        f"{REPO_PATH}/cmd/safe-shell",
        mod="readonly",
        gcflags=gcflags,
        ldflags=ldflags,
        bin_path=BIN_PATH,
        env=env,
    )

    ctx.run(f"ls -alh {BIN_PATH}")
