"""
Dogstatsd tasks
"""

import os
import shutil
import sys

from invoke import task
from invoke.exceptions import Exit

from tasks.build_tags import filter_incompatible_tags, get_build_tags, get_default_build_tags
from tasks.flavor import AgentFlavor
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import REPO_PATH, bin_name, get_build_flags, get_root
from tasks.windows_resources import build_messagetable, build_rc, versioninfo_vars

# constants
DOGSTATSD_BIN_PATH = os.path.join(".", "bin", "dogstatsd")
STATIC_BIN_PATH = os.path.join(".", "bin", "static")
MAX_BINARY_SIZE = 44 * 1024
DOGSTATSD_TAG = "datadog/dogstatsd:master"


@task
def build(
    ctx,
    rebuild=False,
    race=False,
    static=False,
    build_include=None,
    build_exclude=None,
    major_version='7',
    go_mod="readonly",
):
    """
    Build Dogstatsd
    """
    build_include = (
        get_default_build_tags(build="dogstatsd", flavor=AgentFlavor.dogstatsd)
        if build_include is None
        else filter_incompatible_tags(build_include.split(","))
    )
    build_exclude = [] if build_exclude is None else build_exclude.split(",")
    build_tags = get_build_tags(build_include, build_exclude)
    ldflags, gcflags, env = get_build_flags(ctx, static=static, major_version=major_version)
    bin_path = DOGSTATSD_BIN_PATH

    # generate windows resources
    if sys.platform == 'win32':
        build_messagetable(ctx)
        vars = versioninfo_vars(ctx, major_version=major_version)
        build_rc(
            ctx,
            "cmd/dogstatsd/windows_resources/dogstatsd.rc",
            vars=vars,
            out="cmd/dogstatsd/rsrc.syso",
        )

    if static:
        bin_path = STATIC_BIN_PATH

    go_build(
        ctx,
        f"{REPO_PATH}/cmd/dogstatsd",
        mod=go_mod,
        race=race,
        rebuild=rebuild,
        gcflags=gcflags,
        ldflags=ldflags,
        build_tags=build_tags,
        bin_path=os.path.join(bin_path, bin_name("dogstatsd")),
        env=env,
    )

    # Render the configuration file template
    #
    # We need to remove cross compiling bits if any because go generate must
    # build and execute in the native platform
    env = {
        "GOOS": "",
        "GOARCH": "",
    }
    cmd = "go generate -mod={} {}/cmd/dogstatsd"
    ctx.run(cmd.format(go_mod, REPO_PATH), env=env)

    if static and sys.platform.startswith("linux"):
        cmd = "file {bin_name} "
        args = {
            "bin_name": os.path.join(bin_path, bin_name("dogstatsd")),
        }
        result = ctx.run(cmd.format(**args))
        if "statically linked" not in result.stdout:
            print("Dogstatsd binary is not static, exiting...")
            raise Exit(code=1)

    refresh_assets(ctx)


@task
def refresh_assets(_):
    """
    Clean up and refresh Collector's assets and config files
    """
    # ensure DOGSTATSD_BIN_PATH exists
    if not os.path.exists(DOGSTATSD_BIN_PATH):
        os.mkdir(DOGSTATSD_BIN_PATH)

    dist_folder = os.path.join(DOGSTATSD_BIN_PATH, "dist")
    if os.path.exists(dist_folder):
        shutil.rmtree(dist_folder)
    shutil.copytree("./cmd/dogstatsd/dist/", dist_folder, dirs_exist_ok=True)


@task
def run(ctx, rebuild=False, race=False, build_include=None, build_exclude=None, skip_build=False):
    """
    Run Dogstatsd binary. Build the binary before executing, unless
    --skip-build was passed.
    """
    if not skip_build:
        print("Building dogstatsd...")
        build(ctx, rebuild=rebuild, race=race, build_include=build_include, build_exclude=build_exclude)

    target = os.path.join(DOGSTATSD_BIN_PATH, bin_name("dogstatsd"))
    ctx.run(f"{target} start")


@task
def system_tests(ctx, skip_build=False, go_mod="readonly"):
    """
    Run the system testsuite.
    """
    if not skip_build:
        print("Building dogstatsd...")
        build(ctx)

    env = {
        "DOGSTATSD_BIN": os.path.join(get_root(), DOGSTATSD_BIN_PATH, bin_name("dogstatsd")),
    }
    cmd = "go test -mod={go_mod} -tags '{build_tags}' -v {REPO_PATH}/test/system/dogstatsd/"
    args = {
        "go_mod": go_mod,
        "build_tags": " ".join(get_default_build_tags(build="system-tests", flavor=AgentFlavor.dogstatsd)),
        "REPO_PATH": REPO_PATH,
    }
    ctx.run(cmd.format(**args), env=env)


@task
def size_test(ctx, skip_build=False):
    """
    Run the size test for the static binary
    """
    if not skip_build:
        print("Building dogstatsd...")
        build(ctx, static=True)

    bin_path = os.path.join(STATIC_BIN_PATH, bin_name("dogstatsd"))
    stat_info = os.stat(bin_path)
    size = stat_info.st_size / 1024

    if size > MAX_BINARY_SIZE:
        print(f"DogStatsD static build size too big: {size} kB")
        print("This means your PR added big classes or dependencies in the packages dogstatsd uses")
        raise Exit(code=1)

    print(f"DogStatsD static build size OK: {size} kB")


@task
def image_build(ctx, arch='amd64', skip_build=False):
    """
    Build the docker image
    """
    import docker

    client = docker.from_env()

    binary_name = bin_name("dogstatsd")
    src = os.path.join(STATIC_BIN_PATH, binary_name)
    dst = os.path.join("Dockerfiles", "dogstatsd", "alpine", "static")

    if not skip_build:
        build(ctx, rebuild=True, static=True)
    if not os.path.exists(src):
        print(f"Could not find dogstatsd static binary at {src} ")
        raise Exit(code=1)
    if not os.path.exists(dst):
        os.makedirs(dst)

    shutil.copy(src, os.path.join(dst, f"{binary_name}.{arch}"))
    build_context = "Dockerfiles/dogstatsd/alpine"
    dockerfile_path = "Dockerfile"

    client.images.build(
        path=build_context,
        dockerfile=dockerfile_path,
        rm=True,
        tag=DOGSTATSD_TAG,
        platform=f"linux/{arch}",
        buildargs={"TARGETARCH": arch},
    )
    ctx.run(f"rm -rf {build_context}/static")


@task
def clean(ctx):
    """
    Remove temporary objects and binary artifacts
    """
    # go clean
    print("Executing go clean")
    ctx.run("go clean")

    # remove the bin/dogstatsd folder
    print("Remove agent binary folder")
    ctx.run("rm -rf ./bin/dogstatsd")
