"""
systray tasks
"""
from __future__ import print_function
import glob
import os
import shutil
from distutils.dir_util import copy_tree

import invoke
from invoke import task
from invoke.exceptions import Exit

from .utils import bin_name, get_build_flags, pkg_config_path, get_version_numeric_only, load_release_versions
from .utils import REPO_PATH
from .utils import get_version_ldflags
from .build_tags import get_build_tags, get_default_build_tags, ALL_TAGS
from .go import deps

#constants
BIN_PATH = os.path.join(".", "bin", "agent")
AGENT_TAG = "datadog/agent:master"

@task
def build(ctx, rebuild=False, race=False, build_include=None, build_exclude=None,
          puppy=False, use_embedded_libs=False, development=True, precompile_only=False,
          skip_assets=False):
    """
    Build the agent. If the bits to include in the build are not specified,
    the values from `invoke.yaml` will be used.

    Example invokation:
        inv systray.build 
    """
    
    if not invoke.platform.WINDOWS:
        print("Systray only available on Windows")
        return

    # This generates the manifest resource. The manifest resource is necessary for
    # being able to load the ancient C-runtime that comes along with Python 2.7
    #command = "rsrc -arch amd64 -manifest cmd/agent/agent.exe.manifest -o cmd/agent/rsrc.syso"
    ver = get_version_numeric_only(ctx)
    build_maj, build_min, build_patch = ver.split(".")

    command = "windres -v --define MAJ_VER={build_maj} --define MIN_VER={build_min} --define PATCH_VER={build_patch} ".format(
        build_maj=build_maj,
        build_min=build_min,
        build_patch=build_patch
    )
    command += "-i cmd/systray/systray.rc --target=pe-x86-64 -O coff -o cmd/systray/rsrc.syso"
    ctx.run(command)
    ldflags = get_version_ldflags(ctx)
    ldflags += "-s -w -linkmode external -extldflags '-Wl,--subsystem,windows' "
    cmd = "go build {race_opt} {build_type} -o {agent_bin} -ldflags=\"{ldflags}\" {REPO_PATH}/cmd/systray"
    args = {
        "race_opt": "-race" if race else "",
        "build_type": "-a" if rebuild else ("-i" if precompile_only else ""),
       "agent_bin": os.path.join(BIN_PATH, bin_name("ddtray")),
       "ldflags": ldflags,
        "REPO_PATH": REPO_PATH,
    }
    ctx.run(cmd.format(**args))



@task
def run(ctx, rebuild=False, race=False, build_include=None, build_exclude=None,
        puppy=False, skip_build=False):
    """
    Execute the systray binary.

    By default it builds the agent before executing it, unless --skip-build was
    passed. It accepts the same set of options as agent.build.
    """
    if not skip_build:
        build(ctx, rebuild, race, build_include, build_exclude, puppy)

    ctx.run(os.path.join(BIN_PATH, bin_name("ddtray.exe")))


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
    ctx.run("rm -rf ./bin/agent/ddtray.exe")
