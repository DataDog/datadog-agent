import os
import re
import shutil

from invoke import task
from invoke.exceptions import Exit

from tasks.build_tags import get_default_build_tags
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import REPO_PATH, bin_name, get_version_ldflags

BIN_NAME = "otel-agent"
CFG_NAME = "otel-config.yaml"
BIN_DIR = os.path.join(".", "bin", "otel-agent")
BIN_PATH = os.path.join(BIN_DIR, bin_name("otel-agent"))
DDOT_DEV_AGENT_TAG = "nightly-full-main-jmx"
DDOT_DEV_AGENT_BRANCH = "main"
DDOT_AGENT_IMAGE_NAME = "datadog/agent"
DDOT_AGENT_TAG = "main-ddot"
DDOT_BYOC_DOCKERFILE = os.path.join("Dockerfiles", "agent-ddot", "Dockerfile.agent-otel")


@task
def byoc_release(ctx, image=DDOT_DEV_AGENT_TAG, branch=DDOT_DEV_AGENT_BRANCH, repo=DDOT_AGENT_IMAGE_NAME):
    """
    Modify dockerfile
    """
    with open(DDOT_BYOC_DOCKERFILE) as file:
        contents = file.readlines()

    with open(DDOT_BYOC_DOCKERFILE, 'w') as file:
        for line in contents:
            if re.search("^ARG AGENT_REPO=.*$", line):
                line = f"ARG AGENT_REPO={repo}\n"
            elif re.search("^ARG AGENT_VERSION=.*$", line):
                line = f"ARG AGENT_VERSION={image}\n"
            elif re.search("^ARG AGENT_BRANCH=.*$", line):
                line = f"ARG AGENT_BRANCH={branch}\n"

            file.write(line)


@task
def build(ctx, byoc=False):
    """
    Build the otel agent
    """

    if os.path.exists(BIN_PATH):
        os.remove(BIN_PATH)

    env = {"GO111MODULE": "on"}
    build_tags = get_default_build_tags(build="otel-agent")
    ldflags = get_version_ldflags(ctx, major_version='7')
    ldflags += f' -X github.com/DataDog/datadog-agent/cmd/otel-agent/command.BYOC={byoc}'

    go_build(
        ctx,
        f"{REPO_PATH}/cmd/otel-agent",
        mod="readonly",
        build_tags=build_tags,
        ldflags=ldflags,
        bin_path=BIN_PATH,
        env=env,
    )

    dist_folder = os.path.join(BIN_DIR, "dist")
    if os.path.exists(dist_folder):
        shutil.rmtree(dist_folder)
    os.mkdir(dist_folder)

    shutil.copy("./cmd/otel-agent/dist/otel-config.yaml", os.path.join(dist_folder, "otel-config.yaml"))


@task
def image_build(ctx, arch='amd64', base_version='latest', tag=DDOT_AGENT_TAG, push=False, no_cache=False):
    """
    Build the otel agent container image
    """

    build_context = os.path.join("Dockerfiles", "agent")
    dockerfile = os.path.join("Dockerfiles", "agent-ddot", "Dockerfile")

    otel_binary = os.path.join(BIN_DIR, BIN_NAME)
    config_file = os.path.join(BIN_DIR, "dist", CFG_NAME)

    if (not os.path.exists(otel_binary)) or (not os.path.exists(config_file)):
        print("Please run otel-agent.build")
        raise Exit(code=1)

    shutil.copy2(otel_binary, build_context)
    shutil.copy2(config_file, build_context)

    common_build_opts = (
        f"-t {DDOT_AGENT_IMAGE_NAME}:{tag} -f {dockerfile} --build-arg=\"BASE_IMAGE_DD_VERSION={base_version}\""
    )
    if no_cache:
        common_build_opts = f"{common_build_opts} --no-cache"
    ctx.run(f"docker build {common_build_opts} --platform linux/{arch} {build_context}")
    if push:
        ctx.run(f"docker push {tag}")

    os.remove(os.path.join(build_context, BIN_NAME))
    os.remove(os.path.join(build_context, CFG_NAME))


@task
def integration_test(ctx):
    """
    Run the otel integration test
    """
    cmd = """go test -timeout 0s -tags otlp,test -run ^TestIntegration$ \
        github.com/DataDog/datadog-agent/comp/otelcol/otlp/integrationtest -v"""
    ctx.run(cmd)
