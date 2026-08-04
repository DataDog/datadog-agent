import os

from invoke.tasks import task

from tasks.devcontainer import run_on_devcontainer
from tasks.libs.common.constants import REPO_PATH
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import bin_name, get_build_flags

BIN_DIR = os.path.join(".", "bin", "dsd-socket-holder")
BIN_PATH = os.path.join(BIN_DIR, bin_name("dsd-socket-holder"))


@task
@run_on_devcontainer
def build(
    ctx,
    install_path=None,
    go_mod="readonly",
):
    """
    Build the DogStatsD socket holder, a small helper that binds the DogStatsD
    socket once and hands its file descriptor over to the Agent.
    """
    ldflags, gcflags, env = get_build_flags(ctx, install_path=install_path)
    # The socket holder has no build-tag dependent code.
    go_build(
        ctx,
        f"{REPO_PATH}/cmd/dsd-socket-holder",
        build_tags=[],
        ldflags=ldflags,
        gcflags=gcflags,
        env=env,
        bin_path=BIN_PATH,
        mod=go_mod,
    )
