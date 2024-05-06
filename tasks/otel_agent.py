import os

from invoke import task

from tasks.libs.common.utils import REPO_PATH, bin_name

BIN_DIR = os.path.join(".", "bin", "otel-agent")
BIN_PATH = os.path.join(BIN_DIR, bin_name("otel-agent"))


@task
def build(ctx):
    """
    Build the otel agent
    """

    if os.path.exists(BIN_PATH):
        os.remove(BIN_PATH)

    env = {"GO111MODULE": "on"}
    build_tags = ['otlp']

    cmd = f"go build -tags=\"{' '.join(build_tags)}\" -o {BIN_PATH} {REPO_PATH}/cmd/otel-agent"

    ctx.run(cmd, env=env)


