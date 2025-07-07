"""
systray tasks
"""

import os
import sys

from invoke import task

from tasks.libs.common.go import go_build
from tasks.libs.common.utils import REPO_PATH, bin_name, get_version_ldflags
from tasks.libs.releasing.version import get_version_numeric_only

# constants
BIN_PATH = os.path.join(".", "bin", "agent")
AGENT_TAG = "datadog/agent:master"


@task
def build(ctx, debug=False, console=False, rebuild=False, race=False, major_version='7', go_mod="readonly"):
    """
    Build the agent. If the bits to include in the build are not specified,
    the values from `invoke.yaml` will be used.

    Example invokation:
        dda inv systray.build
    """

    if not sys.platform == 'win32':
        print("Systray only available on Windows")
        return

    # This generates the manifest resource. The manifest resource is necessary for
    # being able to load the ancient C-runtime that comes along with Python 2.7
    # command = "rsrc -arch amd64 -manifest cmd/agent/agent.exe.manifest -o cmd/agent/rsrc.syso"
    ver = get_version_numeric_only(ctx, major_version=major_version)
    build_maj, build_min, build_patch = ver.split(".")
    env = {}
    windres_target = "pe-x86-64"

    command = f"windres -v  --target {windres_target} --define MAJ_VER={build_maj} --define MIN_VER={build_min} --define PATCH_VER={build_patch} "
    command += "-i cmd/systray/systray.rc -O coff -o cmd/systray/rsrc.syso"
    ctx.run(command)
    ldflags = get_version_ldflags(ctx, major_version=major_version)
    if not debug:
        ldflags += "-s -w "
    if console:
        subsystem = 'console'
    else:
        subsystem = 'windows'
    ldflags += f"-X {REPO_PATH}/cmd/systray/command/command.subsystem={subsystem} "
    ldflags += f"-linkmode external -extldflags '-Wl,--subsystem,{subsystem}' "
    go_build(
        ctx,
        f"{REPO_PATH}/cmd/systray",
        mod=go_mod,
        race=race,
        rebuild=rebuild,
        bin_path=os.path.join(BIN_PATH, bin_name("ddtray")),
        ldflags=ldflags,
        env=env,
    )


@task
def run(ctx, rebuild=False, race=False, skip_build=False):
    """
    Execute the systray binary.

    By default it builds the agent before executing it, unless --skip-build was
    passed. It accepts the same set of options as agent.build.
    """
    if not skip_build:
        build(ctx, rebuild, race)

    ctx.run(os.path.join(BIN_PATH, bin_name("ddtray")))


@task
def clean(ctx):
    """
    Remove temporary objects and binary artifacts
    """
    # go clean
    print("Executing go clean")
    ctx.run("go clean")

    # remove the bin/agent folder
    print("Remove systray executable")
    try:
        os.remove(os.path.join(BIN_PATH, bin_name("ddtray")))
    except Exception as e:
        print(e)
