"""
Agentless-scanner tasks
"""

import os
import sys

from invoke import task
from invoke.exceptions import Exit

from tasks.build_tags import get_build_tags
from tasks.flavor import AgentFlavor
from tasks.libs.common.utils import REPO_PATH, bin_name, get_build_flags

# constants
AGENTLESS_SCANNER_BIN_PATH = os.path.join(".", "bin", "agentless-scanner")
STATIC_BIN_PATH = os.path.join(".", "bin", "static")


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
    Build Agentless-scanner
    """
    build_tags = get_build_tags(
        flavor=AgentFlavor.agentless_scanner,
        build="agentless-scanner",
        arch=arch,
        build_include=build_include,
        build_exclude=build_exclude,
    )
    ldflags, gcflags, env = get_build_flags(ctx, static=static, major_version=major_version)
    bin_path = AGENTLESS_SCANNER_BIN_PATH

    if static:
        bin_path = STATIC_BIN_PATH

    # NOTE: consider stripping symbols to reduce binary size
    cmd = "go build -mod={go_mod} {race_opt} {build_type} -tags \"{build_tags}\" -o {bin_name} "
    cmd += "-gcflags=\"{gcflags}\" -ldflags=\"{ldflags}\" {REPO_PATH}/cmd/agentless-scanner"
    args = {
        "go_mod": go_mod,
        "race_opt": "-race" if race else "",
        "build_type": "-a" if rebuild else "",
        "build_tags": " ".join(build_tags),
        "bin_name": os.path.join(bin_path, bin_name("agentless-scanner")),
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
    cmd = "go generate -mod={} {}/cmd/agentless-scanner"
    ctx.run(cmd.format(go_mod, REPO_PATH), env=env)

    if static and sys.platform.startswith("linux"):
        cmd = "file {bin_name} "
        args = {
            "bin_name": os.path.join(bin_path, bin_name("agentless-scanner")),
        }
        result = ctx.run(cmd.format(**args))
        if "statically linked" not in result.stdout:
            print("agentless-scanner binary is not static, exiting...")
            raise Exit(code=1)


@task
def clean(ctx):
    """
    Remove temporary objects and binary artifacts
    """
    # go clean
    print("Executing go clean")
    ctx.run("go clean")

    # remove the bin/agentless-scanner folder
    print("Remove agentless-scanner binary folder")
    ctx.run("rm -rf ./bin/agentless-scanner")
