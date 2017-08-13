"""
Agent namespaced tasks
"""
from __future__ import print_function
import os

import invoke
from invoke import task

from .utils import bin_name, get_ldflags, pkg_config_path
from .utils import REPO_PATH
from .build_tags import get_build_tags, get_puppy_build_tags

#constants
BIN_PATH = "./bin/agent"


@task
def build(ctx, incremental=None, race=None, build_include=None, build_exclude=None,
          puppy=None):
    """
    Build the agent. If the bits to include in the build are not specified,
    the values from `invoke.yaml` will be used.

    Example invokation:
        inv agent.build --build-exclude=snmp
    """
    incremental = incremental or ctx.agent.incremental
    race = race or ctx.agent.race
    build_include = ctx.agent.build_include if build_include is None else build_include.split(",")
    build_exclude = ctx.agent.build_exclude if build_exclude is None else build_exclude.split(",")
    puppy = puppy or ctx.agent.puppy

    if puppy:
        build_tags = get_puppy_build_tags()
    else:
        build_tags = get_build_tags(build_include, build_exclude)
    ldflags = get_ldflags(ctx)
    gcflags = ""

    env = {
        "PKG_CONFIG_LIBDIR": pkg_config_path(ctx.use_system_libs)
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

    if os.environ.get("DELVE"):
        gcflags = "-N -l"
        if invoke.platform.WINDOWS:
            # On windows, need to build with the extra argument -ldflags="-linkmode internal"
            # if you want to be able to use the delve debugger.
            ldflags += " -linkmode internal"


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

    ctx.run(cmd.format(**args), env={})

    # TODO: Rake::Task["agent:refresh_assets"].invoke
