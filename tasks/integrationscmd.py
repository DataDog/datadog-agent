"""
integrationscmd tasks
"""
from __future__ import print_function
import os
import sys

import invoke
from invoke import task

from .utils import bin_name, get_build_flags, get_version_numeric_only
from .utils import REPO_PATH
from .utils import get_version_ldflags
from .build_tags import get_build_tags, get_default_build_tags, LINUX_ONLY_TAGS, REDHAT_AND_DEBIAN_ONLY_TAGS, REDHAT_AND_DEBIAN_DIST
# constants
BIN_PATH = os.path.join(".", "bin", "agent")
AGENT_TAG = "datadog/agent:master"

DEFAULT_BUILD_TAGS = [
    "cpython"
]

@task
def build(ctx, rebuild=False, race=False, build_include=None, build_exclude=None,
          puppy=False, use_embedded_libs=False, development=True, precompile_only=False,
          skip_assets=False):
    """
    Build the integration command. If the bits to include in the build are not specified,
    the values from `invoke.yaml` will be used.

    Example invokation:
        inv integrations-cmd.build
    """

    if not sys.platform == 'win32':
        print("integration command only available on Windows")
        return
    build_include = DEFAULT_BUILD_TAGS if build_include is None else build_include.split(",")
    build_exclude = [] if build_exclude is None else build_exclude.split(",")

    # This generates the manifest resource. The manifest resource is necessary for
    # being able to load the ancient C-runtime that comes along with Python 2.7
    # command = "rsrc -arch amd64 -manifest cmd/agent/agent.exe.manifest -o cmd/agent/rsrc.syso"
    ver = get_version_numeric_only(ctx)
    build_maj, build_min, build_patch = ver.split(".")

    command = "windres -v --define MAJ_VER={build_maj} --define MIN_VER={build_min} --define PATCH_VER={build_patch} ".format(
        build_maj=build_maj,
        build_min=build_min,
        build_patch=build_patch
    )
    command += "-i cmd/integrations-cmd/integrations-cmd.rc --target=pe-x86-64 -O coff -o cmd/integrations-cmd/rsrc.syso"
    ctx.run(command)
    build_tags = get_build_tags(build_include, build_exclude)
    build_tags.append("integrationcmd")

    ldflags, gcflags, env = get_build_flags(ctx, use_embedded_libs=use_embedded_libs)
    #ldflags += "-s -w -linkmode external -extldflags '-Wl,--subsystem,windows' "
    
    print("tags: {}".format(" ".join(build_tags)))
    print("ldflags: {}".format(ldflags))
    cmd = "go build {race_opt} {build_type} -tags \"{go_build_tags}\" "
    cmd += "-o {agent_bin} -gcflags=\"{gcflags}\" -ldflags=\"{ldflags}\" {REPO_PATH}/cmd/integrations-cmd"

    args = {
        "race_opt": "-race" if race else "",
        "build_type": "-a" if rebuild else ("-i" if precompile_only else ""),
        "go_build_tags": " ".join(build_tags),
        "agent_bin": os.path.join(BIN_PATH, bin_name("integrations")),
        "ldflags": ldflags,
        "gcflags": gcflags,
        "REPO_PATH": REPO_PATH,
    }
    ctx.run(cmd.format(**args), env=env)


@task
def run(ctx, rebuild=False, race=False, build_include=None, build_exclude=None,
        puppy=False, skip_build=False):
    """
    Execute the integration-command binary.

    By default it builds the agent before executing it, unless --skip-build was
    passed. It accepts the same set of options as agent.build.
    """
    if not skip_build:
        build(ctx, rebuild, race, build_include, build_exclude, puppy)

    ctx.run(os.path.join(BIN_PATH, bin_name("integrations.exe")))


@task
def clean(ctx):
    """
    Remove temporary objects and binary artifacts
    """
    # go clean
    print("Executing go clean")
    ctx.run("go clean")

    # remove the bin/agent folder
    print("Remove integrations executable")
    ctx.run("rm -rf ./bin/agent/integrations.exe")
