import os

from invoke.tasks import task

from tasks.libs.common.utils import (
    REPO_PATH,
)

BIN_DIR = os.path.join(".", "bin")
BIN_PATH = os.path.join(BIN_DIR, "sbomgen", "sbomgen")


@task
def build(ctx, dumpdep=False):
    build_tags = ["trivy", "grpcnotrace", "containerd", "docker", "crio"]
    ldflags = "-s -w"
    if dumpdep:
        ldflags += " -dumpdep"

    cmd = 'go build -mod=readonly -ldflags="{ldflags}" -tags "{go_build_tags}" -o {agent_bin} {REPO_PATH}/cmd/sbomgen'

    args = {
        "ldflags": ldflags,
        "go_build_tags": ",".join(build_tags),
        "agent_bin": BIN_PATH,
        "REPO_PATH": REPO_PATH,
    }

    ctx.run(cmd.format(**args))
    ctx.run(f"ls -alh {BIN_PATH}")
