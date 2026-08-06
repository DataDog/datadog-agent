import os

from invoke.tasks import task

from tasks.libs.common.constants import REPO_PATH
from tasks.libs.common.go import go_build

BIN_DIR = os.path.join(".", "bin", "agent-rollout-gate")
BIN_PATH = os.path.join(BIN_DIR, "agent-rollout-gate")


@task
def build(ctx, go_mod="readonly"):
    """Build the lightweight Linux Agent rollout gate."""
    go_build(
        ctx,
        f"{REPO_PATH}/cmd/agent-rollout-gate",
        build_tags=[],
        ldflags="-s -w",
        gcflags="",
        env={"CGO_ENABLED": "0", "GOOS": "linux"},
        bin_path=BIN_PATH,
        mod=go_mod,
    )
