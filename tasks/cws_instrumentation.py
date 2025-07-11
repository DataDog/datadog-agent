import datetime
import glob
import os
import platform
import shutil

from invoke import task
from invoke.exceptions import Exit

from tasks.build_tags import add_fips_tags, get_default_build_tags
from tasks.libs.common.git import get_commit_sha, get_current_branch
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import (
    REPO_PATH,
    bin_name,
    get_build_flags,
    get_go_version,
    get_version,
)

BIN_DIR = os.path.join(".", "bin")
BIN_PATH = os.path.join(BIN_DIR, "cws-instrumentation", bin_name("cws-instrumentation"))
AGENT_TAG = "datadog/cws-instrumentation:master"
CONTAINER_PLATFORM_MAPPING = {"aarch64": "arm64", "amd64": "amd64", "x86_64": "amd64"}


@task(iterable=["build_tags"])
def build(
    ctx,
    build_tags=None,
    race=False,
    rebuild=False,
    major_version='7',
    go_mod="readonly",
    static=False,
    fips_mode=False,
    no_strip_binary=False,
    arch_suffix=False,
):
    """
    Build cws-instrumentation
    """
    if build_tags is None:
        build_tags = []
    ldflags, gcflags, env = get_build_flags(ctx, major_version=major_version, static=static)

    # TODO use pkg/version for this
    main = "main."
    ld_vars = {
        "Version": get_version(ctx, major_version=major_version),
        "GoVersion": get_go_version(),
        "GitBranch": get_current_branch(ctx),
        "GitCommit": get_commit_sha(ctx, short=True),
        "BuildDate": datetime.datetime.now().strftime("%Y-%m-%dT%H:%M:%S"),
    }

    ldflags += ' '.join([f"-X '{main + key}={value}'" for key, value in ld_vars.items()])
    build_tags += get_default_build_tags(build="cws-instrumentation")
    build_tags = add_fips_tags(build_tags, fips_mode)

    agent_bin = BIN_PATH
    if arch_suffix:
        arch = CONTAINER_PLATFORM_MAPPING.get(platform.machine().lower())
        agent_bin = f'{agent_bin}.{arch}'

    if not no_strip_binary:
        ldflags += " -s -w"

    go_build(
        ctx,
        f"{REPO_PATH}/cmd/cws-instrumentation",
        mod=go_mod,
        race=race,
        rebuild=rebuild,
        gcflags=gcflags,
        ldflags=ldflags,
        build_tags=build_tags,
        bin_path=agent_bin,
        env=env,
    )


@task
def image_build(ctx, arch=None, tag=AGENT_TAG, push=False):
    """
    Build the docker image
    """
    if arch is None:
        arch = CONTAINER_PLATFORM_MAPPING.get(platform.machine().lower())

    if arch is None:
        print("Unable to determine architecture to build, please set `arch` parameter")
        raise Exit(code=1)

    cws_instrumentation_binary = glob.glob(BIN_PATH)
    # get the last debian package built
    if not cws_instrumentation_binary:
        print(f"{BIN_PATH} not found")
        print("See cws-instrumentation.build")
        raise Exit(code=1)

    latest_file = max(cws_instrumentation_binary, key=os.path.getctime)
    ctx.run(f"chmod +x {latest_file}")

    build_context = "Dockerfiles/cws-instrumentation"
    exec_path = f"{build_context}/cws-instrumentation.{arch}"
    dockerfile_path = f"{build_context}/Dockerfile"

    shutil.copy2(latest_file, exec_path)
    ctx.run(f"docker build -t {tag} --platform linux/{arch} {build_context} -f {dockerfile_path}")
    ctx.run(f"rm {exec_path}")

    if push:
        ctx.run(f"docker push {tag}")
