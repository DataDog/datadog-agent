"""
Dogstatsd tasks
"""
from __future__ import print_function
import os

import invoke
from invoke import task
from invoke.exceptions import Exit

from .build_tags import get_build_tags
from .utils import get_ldflags, bin_name, get_root
from .utils import REPO_PATH


# constants
DOGSTATSD_BIN_PATH = os.path.join(".", "bin", "dogstatsd")
STATIC_BIN_PATH = os.path.join(".", "bin", "static")
MAX_BINARY_SIZE = 15 * 1024

@task
def build(ctx, incremental=None, race=False, static=False, build_include=None, build_exclude=None):
    """
    Build Dogstatsd
    """
    incremental = ctx.dogstatsd.incremental if incremental is None else is_affirmative(incremental)
    race = race or ctx.dogstatsd.race
    static = static or ctx.dogstatsd.static
    build_include = ctx.dogstatsd.build_include if build_include is None else build_include.split(",")
    build_exclude = ctx.dogstatsd.build_exclude if build_exclude is None else build_exclude.split(",")
    build_tags = get_build_tags(build_include, build_exclude)
    ldflags, gcflags = get_ldflags(ctx, static=static)
    bin_path = DOGSTATSD_BIN_PATH

    if static:
        bin_path = STATIC_BIN_PATH

    cmd = "go build {race_opt} {build_type} -tags '{build_tags}' -o {bin_name} "
    cmd += "-gcflags=\"{gcflags}\" -ldflags=\"{ldflags}\" {REPO_PATH}/cmd/dogstatsd/"
    args = {
        "race_opt": "-race" if race else "",
        "build_type": "-i" if incremental else "-a",
        "build_tags": " ".join(build_tags),
        "bin_name": os.path.join(bin_path, bin_name("dogstatsd")),
        "gcflags": gcflags,
        "ldflags": ldflags,
        "REPO_PATH": REPO_PATH,
    }

    ctx.run(cmd.format(**args))


@task
def run(ctx, incremental=None, race=None, build_include=None, build_exclude=None,
        skip_build=False):
    """
    Run Dogstatsd binary. Build the binary before executing, unless
    --skip-build was passed.
    """
    if not skip_build:
        print("Building dogstatsd...")
        build(ctx, incremental, race, build_include, build_exclude)

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
        "build_tags": " ".join(get_build_tags()),
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
        raise Exit(1)

    print("DogStatsD static build size OK: {} kB".format(size))


@task
def omnibus_build(ctx):
    """
    Build the Dogstatsd packages with Omnibus Installer.
    """
    # omnibus config overrides
    overrides = []

    # base dir (can be overridden through env vars)
    base_dir = os.environ.get("AGENT_OMNIBUS_BASE_DIR")
    if base_dir:
        overrides.append("base_dir:{}".format(base_dir))

    # package_dir (can be overridden through env vars)
    package_dir = os.environ.get("AGENT_OMNIBUS_PACKAGE_DIR")
    if package_dir:
        overrides.append("package_dir:{}".format(package_dir))

    overrides_cmd = ""
    if overrides:
        overrides_cmd = "--override=" + " ".join(overrides)

    with ctx.cd("omnibus"):
        ctx.run("bundle install --without development")
        omnibus = "omnibus.bat" if invoke.platform.WINDOWS else "omnibus"
        cmd = "{omnibus} build dogstatsd --log-level={log_level} {overrides}"
        args = {
            "omnibus": omnibus,
            "log_level": os.environ.get("AGENT_OMNIBUS_LOG_LEVEL", "info"),
            "overrides": overrides_cmd
        }
        ctx.run(cmd.format(**args))
