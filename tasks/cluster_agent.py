"""
Cluster Agent tasks
"""

import os
from invoke import task

from .build_tags import get_build_tags
from .utils import get_build_flags, bin_name
from .utils import REPO_PATH

#constants
BIN_PATH = os.path.join(".", "bin", "cluster-agent")
AGENT_TAG = "datadog/cluster_agent:master"

@task
def build(ctx, rebuild=False, race=False, static=False, use_embedded_libs=False):
    """
    Build Cluster Agent

     Example invokation:
        inv cluster-agent.build
    """

    build_tags = get_build_tags("all", "snmp")

    ldflags, gcflags = get_build_flags(ctx, static=static, use_embedded_libs=use_embedded_libs)

    cmd = "go build {race_opt} {build_type} -tags '{build_tags}' -o {bin_name} "
    cmd += "-gcflags=\"{gcflags}\" -ldflags=\"{ldflags}\" {REPO_PATH}/cmd/cluster-agent/"
    args = {
        "race_opt": "-race" if race else "",
        "build_type": "-a" if rebuild else "-i",
        "build_tags": " ".join(build_tags),
        "bin_name": os.path.join(BIN_PATH, bin_name("cluster-agent")),
        "gcflags": gcflags,
        "ldflags": ldflags,
        "REPO_PATH": REPO_PATH,
    }
    ctx.run(cmd.format(**args))

@task
def run(ctx, rebuild=False, race=False, skip_build=False, development=True):
    """
    Run the Cluster Agent's binary. Build the binary before executing, unless
    --skip-build was passed.
    """
    if not skip_build:
        print("Building the Cluster Agent...")
        build(ctx, rebuild=rebuild, race=race)

    target = os.path.join(BIN_PATH, bin_name("cluster-agent"))
    cfgPath = ""
    if development:
        cfgPath = "-c dev/dist/datadog.yaml"

    ctx.run("{0} start {1}".format(target, cfgPath))

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
    ctx.run("rm -rf ./bin/cluster-agent")
