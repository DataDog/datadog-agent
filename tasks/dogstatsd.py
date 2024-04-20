"""
Dogstatsd tasks
"""

import os
import shutil
import sys

from invoke import task
from invoke.exceptions import Exit

from tasks.build_tags import get_build_tags
from tasks.flavor import AgentFlavor
from tasks.go import deps
from tasks.libs.common.utils import REPO_PATH, bin_name, get_build_flags, get_root
from tasks.windows_resources import build_messagetable, build_rc, versioninfo_vars

# constants
DOGSTATSD_BIN_PATH = os.path.join(".", "bin", "dogstatsd")
STATIC_BIN_PATH = os.path.join(".", "bin", "static")
MAX_BINARY_SIZE = 42 * 1024
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
    arch="x64",
    go_mod="mod",
):
    """
    Build Dogstatsd
    """
    build_tags = get_build_tags(
        build="dogstatsd",
        arch=arch,
        flavor=AgentFlavor.dogstatsd,
        build_include=build_include,
        build_exclude=build_exclude,
    )

    ldflags, gcflags, env = get_build_flags(ctx, static=static, major_version=major_version)
    bin_path = DOGSTATSD_BIN_PATH

    # generate windows resources
    if sys.platform == 'win32':
        if arch == "x86":
            env["GOARCH"] = "386"

        build_messagetable(ctx, arch=arch)
        vars = versioninfo_vars(ctx, major_version=major_version, arch=arch)
        build_rc(
            ctx,
            "cmd/dogstatsd/windows_resources/dogstatsd.rc",
            arch=arch,
            vars=vars,
            out="cmd/dogstatsd/rsrc.syso",
        )

    if static:
        bin_path = STATIC_BIN_PATH

    # NOTE: consider stripping symbols to reduce binary size
    cmd = "go build -mod={go_mod} {race_opt} {build_type} -tags \"{build_tags}\" -o {bin_name} "
    cmd += "-gcflags=\"{gcflags}\" -ldflags=\"{ldflags}\" {REPO_PATH}/cmd/dogstatsd"
    args = {
        "go_mod": go_mod,
        "race_opt": "-race" if race else "",
        "build_type": "-a" if rebuild else "",
        "build_tags": " ".join(build_tags),
        "bin_name": os.path.join(bin_path, bin_name("dogstatsd")),
        "gcflags": gcflags,
        "ldflags": ldflags,
        "REPO_PATH": REPO_PATH,
    }
    ctx.run(cmd.format(**args), env=env)

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
def system_tests(ctx, skip_build=False, go_mod="mod", arch="x64"):
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
        "build_tags": " ".join(get_build_tags(build="system-tests", arch=arch, flavor=AgentFlavor.dogstatsd)),
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
def integration_tests(ctx, install_deps=False, race=False, remote_docker=False, go_mod="mod", arch="x64"):
    """
    Run integration tests for dogstatsd
    """
    if sys.platform == 'win32':
        raise Exit(message='dogstatsd integration tests are not supported on Windows', code=0)

    if install_deps:
        deps(ctx)

    go_build_tags = " ".join(get_build_tags(build="test", arch=arch))
    race_opt = "-race" if race else ""
    exec_opts = ""

    # since Go 1.13, the -exec flag of go test could add some parameters such as -test.timeout
    # to the call, we don't want them because while calling invoke below, invoke
    # thinks that the parameters are for it to interpret.
    # we're calling an intermediate script which only pass the binary name to the invoke task.
    if remote_docker:
        exec_opts = f"-exec \"{os.getcwd()}/test/integration/dockerize_tests.sh\""

    go_cmd = f'go test -mod={go_mod} {race_opt} -tags "{go_build_tags}" {exec_opts}'

    prefixes = [
        "./test/integration/dogstatsd/...",
    ]

    for prefix in prefixes:
        ctx.run(f"{go_cmd} {prefix}")


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
