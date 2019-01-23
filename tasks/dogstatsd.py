"""
Dogstatsd tasks
"""
from __future__ import print_function, absolute_import

import os
import sys
import shutil
from distutils.dir_util import copy_tree

import invoke
from invoke import task
from invoke.exceptions import Exit

from .build_tags import get_build_tags, get_default_build_tags
from .utils import get_build_flags, bin_name, get_root, load_release_versions, get_version
from .utils import REPO_PATH

from .go import deps

# constants
DOGSTATSD_BIN_PATH = os.path.join(".", "bin", "dogstatsd")
STATIC_BIN_PATH = os.path.join(".", "bin", "static")
MAX_BINARY_SIZE = 20 * 1024
DOGSTATSD_TAG = "datadog/dogstatsd:master"
DEFAULT_BUILD_TAGS = [
    "zlib",
    "docker",
    "kubelet",
]


@task
def build(ctx, rebuild=False, race=False, static=False, build_include=None,
          build_exclude=None, use_embedded_libs=False):
    """
    Build Dogstatsd
    """
    build_include = DEFAULT_BUILD_TAGS if build_include is None else build_include.split(",")
    build_exclude = [] if build_exclude is None else build_exclude.split(",")
    build_tags = get_build_tags(build_include, build_exclude)
    ldflags, gcflags, env = get_build_flags(ctx, static=static, use_embedded_libs=use_embedded_libs)
    bin_path = DOGSTATSD_BIN_PATH

    if static:
        bin_path = STATIC_BIN_PATH

    # NOTE: consider stripping symbols to reduce binary size
    cmd = "go build {race_opt} {build_type} -tags '{build_tags}' -o {bin_name} "
    cmd += "-gcflags=\"{gcflags}\" -ldflags=\"{ldflags}\" {REPO_PATH}/cmd/dogstatsd"
    args = {
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
    cmd = "go generate {}/cmd/dogstatsd"
    ctx.run(cmd.format(REPO_PATH), env=env)

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
def refresh_assets(ctx):
    """
    Clean up and refresh Collector's assets and config files
    """
    # ensure DOGSTATSD_BIN_PATH exists
    if not os.path.exists(DOGSTATSD_BIN_PATH):
        os.mkdir(DOGSTATSD_BIN_PATH)

    dist_folder = os.path.join(DOGSTATSD_BIN_PATH, "dist")
    if os.path.exists(dist_folder):
        shutil.rmtree(dist_folder)
    copy_tree("./cmd/dogstatsd/dist/", dist_folder)


@task
def run(ctx, rebuild=False, race=False, build_include=None, build_exclude=None,
        skip_build=False):
    """
    Run Dogstatsd binary. Build the binary before executing, unless
    --skip-build was passed.
    """
    if not skip_build:
        print("Building dogstatsd...")
        build(ctx, rebuild=rebuild, race=race, build_include=build_include,
              build_exclude=build_exclude)

    target = os.path.join(DOGSTATSD_BIN_PATH, bin_name("dogstatsd"))
    ctx.run("{} start".format(target))


@task
def system_tests(ctx, skip_build=False):
    """
    Run the system testsuite.
    """
    if not skip_build:
        print("Building dogstatsd...")
        build(ctx)

    env = {
        "DOGSTATSD_BIN": os.path.join(get_root(), DOGSTATSD_BIN_PATH, bin_name("dogstatsd")),
    }
    cmd = "go test -tags '{build_tags}' -v {REPO_PATH}/test/system/dogstatsd/"
    args = {
        "build_tags": " ".join(get_default_build_tags()),
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
        print("DogStatsD static build size too big: {} kB".format(size))
        print("This means your PR added big classes or dependencies in the packages dogstatsd uses")
        raise Exit(code=1)

    print("DogStatsD static build size OK: {} kB".format(size))


@task
def omnibus_build(ctx, log_level="info", base_dir=None, gem_path=None,
                  skip_deps=False, release_version="nightly", omnibus_s3_cache=False):
    """
    Build the Dogstatsd packages with Omnibus Installer.
    """
    if not skip_deps:
        deps(ctx)

    # omnibus config overrides
    overrides = []

    # base dir (can be overridden through env vars, command line takes precedence)
    base_dir = base_dir or os.environ.get("DSD_OMNIBUS_BASE_DIR")
    if base_dir:
        overrides.append("base_dir:{}".format(base_dir))

    overrides_cmd = ""
    if overrides:
        overrides_cmd = "--override=" + " ".join(overrides)

    with ctx.cd("omnibus"):
        env = load_release_versions(ctx, release_version)
        cmd = "bundle install"
        if gem_path:
            cmd += " --path {}".format(gem_path)
        ctx.run(cmd, env=env)
        omnibus = "bundle exec omnibus.bat" if sys.platform == 'win32' else "bundle exec omnibus"
        cmd = "{omnibus} build dogstatsd --log-level={log_level} {populate_s3_cache} {overrides}"
        args = {
            "omnibus": omnibus,
            "log_level": log_level,
            "overrides": overrides_cmd,
            "populate_s3_cache": ""
        }
        if omnibus_s3_cache:
            args['populate_s3_cache'] = " --populate-s3-cache "
        env['PACKAGE_VERSION'] = get_version(ctx, include_git=True, url_safe=True, git_sha_length=7, match=None)        
        ctx.run(cmd.format(**args), env=env)


@task
def integration_tests(ctx, install_deps=False, race=False, remote_docker=False):
    """
    Run integration tests for dogstatsd
    """
    if install_deps:
        deps(ctx)

    test_args = {
        "go_build_tags": " ".join(get_default_build_tags()),
        "race_opt": "-race" if race else "",
        "exec_opts": "",
    }

    if remote_docker:
        test_args["exec_opts"] = "-exec \"inv docker.dockerize-test\""

    go_cmd = 'go test {race_opt} -tags "{go_build_tags}" {exec_opts}'.format(**test_args)

    prefixes = [
        "./test/integration/dogstatsd/...",
    ]

    for prefix in prefixes:
        ctx.run("{} {}".format(go_cmd, prefix))


@task
def image_build(ctx, skip_build=False):
    """
    Build the docker image
    """
    import docker
    client = docker.from_env()

    src = os.path.join(STATIC_BIN_PATH, bin_name("dogstatsd"))
    dst = os.path.join("Dockerfiles", "dogstatsd", "alpine", "static")

    if not skip_build:
        build(ctx, rebuild=True, static=True)
    if not os.path.exists(src):
        raise Exit(code=1)
    if not os.path.exists(dst):
        os.makedirs(dst)

    shutil.copy(src, dst)
    client.images.build(path="Dockerfiles/dogstatsd/alpine/", rm=True, tag=DOGSTATSD_TAG)
    ctx.run("rm -rf Dockerfiles/dogstatsd/alpine/static")


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
