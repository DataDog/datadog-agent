"""
Agent namespaced tasks
"""
from __future__ import print_function
import os
import shutil
from distutils.dir_util import copy_tree

import invoke
from invoke import task

from .utils import bin_name, get_ldflags, pkg_config_path
from .utils import REPO_PATH
from .build_tags import get_build_tags, get_puppy_build_tags
from .go import deps

#constants
BIN_PATH = os.path.join(".", "bin", "agent")


@task
def build(ctx, incremental=True, race=False, build_include=None, build_exclude=None,
          puppy=False, use_embedded_libs=False):
    """
    Build the agent. If the bits to include in the build are not specified,
    the values from `invoke.yaml` will be used.

    Example invokation:
        inv agent.build --build-exclude=snmp
    """
    build_include = ctx.agent.build_include if build_include is None else build_include.split(",")
    build_exclude = ctx.agent.build_exclude if build_exclude is None else build_exclude.split(",")

    if puppy:
        build_tags = get_puppy_build_tags()
    else:
        build_tags = get_build_tags(build_include, build_exclude)
    ldflags, gcflags = get_ldflags(ctx)

    env = {
        "PKG_CONFIG_PATH": pkg_config_path(use_embedded_libs)
    }

    if invoke.platform.WINDOWS:
        # This generates the manifest resource. The manifest resource is necessary for
        # being able to load the ancient C-runtime that comes along with Python 2.7
        #command = "rsrc -arch amd64 -manifest cmd/agent/agent.exe.manifest -o cmd/agent/rsrc.syso"

        # fixme -- still need to calculate correct *_VER numbers at build time rather than
        # hard-coded here.
        command = "windres --define MAJ_VER=6 --define MIN_VER=0 --define PATCH_VER=0 "
        command += "-i cmd/agent/agent.rc --target=pe-x86-64 -O coff -o cmd/agent/rsrc.syso"
        ctx.run(command, env=env)

    cmd = "go build {race_opt} {build_type} -tags \"{go_build_tags}\" "
    cmd += "-o {agent_bin} -gcflags=\"{gcflags}\" -ldflags=\"{ldflags}\" {REPO_PATH}/cmd/agent"
    args = {
        "race_opt": "-race" if race else "",
        "build_type": "-i" if incremental else "-a",
        "go_build_tags": " ".join(build_tags),
        "agent_bin": os.path.join(BIN_PATH, bin_name("agent")),
        "gcflags": gcflags,
        "ldflags": ldflags,
        "REPO_PATH": REPO_PATH,
    }

    ctx.run(cmd.format(**args), env=env)
    refresh_assets(ctx)


@task
def refresh_assets(ctx):
    """
    Clean up and refresh Collector's assets and config files
    """
    # ensure BIN_PATH exists
    if not os.path.exists(BIN_PATH):
        os.mkdir(BIN_PATH)

    dist_folder = os.path.join(BIN_PATH, "dist")
    if os.path.exists(dist_folder):
        shutil.rmtree(dist_folder)
    copy_tree("./pkg/collector/dist/", dist_folder)
    copy_tree("./pkg/status/dist/", dist_folder)
    copy_tree("./dev/dist/", dist_folder)

    bin_agent = os.path.join(BIN_PATH, "agent")
    shutil.move(os.path.join(dist_folder, "agent"), bin_agent)
    os.chmod(bin_agent, 0755)


@task
def run(ctx, incremental=True, race=False, build_include=None, build_exclude=None,
        puppy=False, skip_build=False):
    """
    Execute the agent binary.

    By default it builds the agent before executing it, unless --skip-build was
    passed. It accepts the same set of options as agent.build.
    """
    if not skip_build:
        build(ctx, incremental, race, build_include, build_exclude, puppy)

    ctx.run(os.path.join(BIN_PATH, bin_name("agent")))


@task
def system_tests(ctx):
    """
    Run the system testsuite.
    """
    pass


@task
def integration_tests(ctx, install_deps=False):
    """
    Run integration tests for the Agent
    """
    if install_deps:
        deps(ctx)

    build_tags = get_build_tags()

    # config_providers
    cmd = "go test -tags '{}' {}/test/integration/config_providers/..."
    ctx.run(cmd.format(" ".join(build_tags), REPO_PATH))

    # listeners
    cmd = "go test -tags '{}' {}/test/integration/listeners/..."
    ctx.run(cmd.format(" ".join(build_tags), REPO_PATH))

    # autodiscovery
    # TODO

    # metadata_providers
    # TODO


@task
def omnibus_build(ctx, puppy=False):
    """
    Build the Agent packages with Omnibus Installer.
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
        ctx.run("bundle install")
        omnibus = "omnibus.bat" if invoke.platform.WINDOWS else "omnibus"
        cmd = "{omnibus} build {project_name} --log-level={log_level} {overrides}"
        args = {
            "omnibus": omnibus,
            "project_name": "puppy" if puppy else "datadog-agent6",
            "log_level": os.environ.get("AGENT_OMNIBUS_LOG_LEVEL", "info"),
            "overrides": overrides_cmd
        }
        ctx.run(cmd.format(**args))


@task
def clean(ctx):
    """
    Remove temporary objects and binary artifacts
    """
    # go clean
    print("Executing go clean")
    ctx.run("go clean")

    # remove the bin/agent folder
    print("Remove agent binary folder")
    ctx.run("rm -rf ./bin/agent")
