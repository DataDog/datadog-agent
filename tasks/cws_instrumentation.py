import datetime
import glob
import os
import platform
import shutil

from invoke import task
from invoke.exceptions import Exit

from tasks.build_tags import get_default_build_tags
from tasks.libs.common.git import get_commit_sha, get_current_branch
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
    incremental_build=True,
    major_version='7',
    go_mod="mod",
    static=False,
    no_strip_binary=False,
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
    build_tags += get_default_build_tags(
        build="cws-instrumentation"
    )  # TODO/FIXME: Arch not passed to preserve build tags. Should this be fixed?
    build_tags.append("netgo")
    build_tags.append("osusergo")

    race_opt = "-race" if race else ""
    build_type = "" if incremental_build else "-a"
    go_build_tags = " ".join(build_tags)
    agent_bin = BIN_PATH

    strip_flags = "" if no_strip_binary else "-s -w"

    cmd = (
        f'go build -mod={go_mod} {race_opt} {build_type} -tags "{go_build_tags}" '
        f'-o {agent_bin} -gcflags="{gcflags}" -ldflags="{ldflags} {strip_flags}" {REPO_PATH}/cmd/cws-instrumentation'
    )

    ctx.run(cmd, env=env)


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
    cws_instrumentation_base = f"{build_context}/datadog-cws-instrumentation"
    exec_path = f"{cws_instrumentation_base}/cws-instrumentation.{arch}"
    dockerfile_path = f"{build_context}/Dockerfile"

    try:
        os.mkdir(cws_instrumentation_base)
    except FileExistsError:
        # Directory already exists
        pass
    except OSError as e:
        # Handle other OS-related errors
        print(f"Error creating directory: {e}")

    shutil.copy2(latest_file, exec_path)
    ctx.run(f"docker build -t {tag} --platform linux/{arch} {build_context} -f {dockerfile_path}")
    ctx.run(f"rm {exec_path}")

    if push:
        ctx.run(f"docker push {tag}")
