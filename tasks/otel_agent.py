import os
import shutil

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.utils import REPO_PATH, bin_name, get_version_ldflags
from tasks.libs.releasing.version import get_version

BIN_NAME = "otel-agent"
CFG_NAME = "otel-config.yaml"
BIN_DIR = os.path.join(".", "bin", "otel-agent")
BIN_PATH = os.path.join(BIN_DIR, bin_name("otel-agent"))
OT_AGENT_IMAGE_NAME = "datadog/agent"
OT_AGENT_TAG = "main-ot"


@task
def build(ctx, goarch="amd64"):
    """
    Build the otel agent
    """

    if os.path.exists(BIN_PATH):
        os.remove(BIN_PATH)

    env = {"GO111MODULE": "on", "GOOS": "linux", "GOARCH": goarch}
    build_tags = ["otlp"]
    ldflags = get_version_ldflags(ctx)
    print(ldflags)
    cmd = f"go build -mod=mod -tags=\"{' '.join(build_tags)}\" -ldflags=\"{ldflags}\" -o {BIN_PATH} {REPO_PATH}/cmd/otel-agent"

    ctx.run(cmd, env=env)

    dist_folder = os.path.join(BIN_DIR, "dist")
    if os.path.exists(dist_folder):
        shutil.rmtree(dist_folder)
    os.mkdir(dist_folder)

    shutil.copy("./cmd/otel-agent/dist/otel-config.yaml", os.path.join(dist_folder, "otel-config.yaml"))


@task
def image_build(ctx, arch="amd64", base_version="latest", tag=OT_AGENT_TAG, push=False, no_cache=False, hide=False):
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
    ctx.run(f"docker build {common_build_opts} --platform linux/{arch} {build_context}", hide=hide)
    if push:
        ctx.run(f"docker push {tag}", hide=hide)

    os.remove(os.path.join(build_context, BIN_NAME))
    os.remove(os.path.join(build_context, CFG_NAME))


@task
def test_image_build(ctx, python_command="python3", arch="amd64"):
    """run image_build task and perform follow-on tests to ensure image builds correctly"""
    try:
        build(ctx, goarch=arch)
        print("testing if otel agent build command ran successfully")
    except Exception as e:
        print(f"Error occurred during otel agent build command: {e}")
        raise
    try:
        image_build(ctx, arch=arch, hide=True)
        print("testing if image built successfully")
    except Exception as e:
        print(f"Error occurred during image build command: {e}")
        raise
    env = {
        "OT_AGENT_IMAGE_NAME": OT_AGENT_IMAGE_NAME,
        "OT_AGENT_TAG": OT_AGENT_TAG,
        "EXPECTED_VERSION": get_version(ctx),
    }
    result = ctx.run(f"docker image inspect {OT_AGENT_IMAGE_NAME}:{OT_AGENT_TAG}", env=env, hide=True)
    if "Error" in result.stdout and "No such image" in result.stdout:
        raise Exit(message=f"Build failed; docker build stdout below:\n{result.stdout}", code=1)

    print("Image build complete, running OtelAgentBuildTest")
    ctx.run(f"{python_command} ./tasks/unit_tests/otel_agent_build_tests.py", env=env)
