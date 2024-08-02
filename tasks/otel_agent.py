import os
import shutil

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.utils import REPO_PATH, bin_name

BIN_NAME = "otel-agent"
CFG_NAME = "otel-config.yaml"
BIN_DIR = os.path.join(".", "bin", "otel-agent")
BIN_PATH = os.path.join(BIN_DIR, bin_name("otel-agent"))
OT_AGENT_IMAGE_NAME = "datadog/agent"
OT_AGENT_TAG = "main-ot"


@task
def build(ctx):
    """
    Build the otel agent
    """

    if os.path.exists(BIN_PATH):
        os.remove(BIN_PATH)

    env = {"GO111MODULE": "on"}
    build_tags = ['otlp']

    cmd = f"go build -mod=mod -tags=\"{' '.join(build_tags)}\" -o {BIN_PATH} {REPO_PATH}/cmd/otel-agent"

    ctx.run(cmd, env=env)

    dist_folder = os.path.join(BIN_DIR, "dist")
    if os.path.exists(dist_folder):
        shutil.rmtree(dist_folder)
    os.mkdir(dist_folder)

    shutil.copy("./cmd/otel-agent/dist/otel-config.yaml", os.path.join(dist_folder, "otel-config.yaml"))


@task
def image_build(ctx, arch='amd64', base_version='latest', tag=OT_AGENT_TAG, push=False, no_cache=False):
    """
    Build the otel agent container image
    """

    build_context = os.path.join("Dockerfiles", "agent")
    dockerfile = os.path.join("Dockerfiles", "agent-ot", "Dockerfile")

    otel_binary = os.path.join(BIN_DIR, BIN_NAME)
    config_file = os.path.join(BIN_DIR, "dist", CFG_NAME)

    if (not os.path.exists(otel_binary)) or (not os.path.exists(config_file)):
        print("Please run otel-agent.build")
        raise Exit(code=1)

    shutil.copy2(otel_binary, build_context)
    shutil.copy2(config_file, build_context)

    common_build_opts = (
        f"-t {OT_AGENT_IMAGE_NAME}:{tag} -f {dockerfile} --build-arg=\"BASE_IMAGE_DD_VERSION={base_version}\""
    )
    if no_cache:
        common_build_opts = f"{common_build_opts} --no-cache"
    ctx.run(f"docker build {common_build_opts} --platform linux/{arch} {build_context}")
    if push:
        ctx.run(f"docker push {tag}")

    os.remove(os.path.join(build_context, BIN_NAME))
    os.remove(os.path.join(build_context, CFG_NAME))
