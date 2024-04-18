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

    cmd = 'go build -tags="{go_build_tags}" '
    cmd += '-o {agent_bin} {REPO_PATH}/cmd/otel-agent'

    args = {
        "go_build_tags": " ".join(build_tags),
        "agent_bin": BIN_PATH,
        "REPO_PATH": REPO_PATH,
    }

    ctx.run(cmd.format(**args), env=env)


