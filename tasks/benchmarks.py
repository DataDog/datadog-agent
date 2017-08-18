"""
Benchmarking tasks
"""
from __future__ import print_function
import os

import invoke
from invoke import task

from .build_tags import get_build_tags
from .utils import bin_name
from .utils import REPO_PATH


# constants
BENCHMARKS_BIN_PATH = os.path.join(".", "bin", "benchmarks")


@task
def build_aggregator(ctx, incremental=None):
    """
    Build the Aggregator benchmarks.
    """
    incremental = incremental or ctx.benchmarks.incremental
    build_tags = get_build_tags()  # pass all the build flags

    ldflags = ""
    gcflags = ""

    if os.environ.get("DELVE"):
        gcflags = "-N -l"
        if invoke.platform.WINDOWS:
            # On windows, need to build with the extra argument -ldflags="-linkmode internal"
            # if you want to be able to use the delve debugger.
            ldflags += " -linkmode internal"

    cmd = "go build {build_type} -tags \"{build_tags}\" -o {bin_name} "
    cmd += "{ldflags} {gcflags} {REPO_PATH}/test/benchmarks/aggregator"
    args = {
        "build_type": "-i" if incremental else "-a",
        "build_tags": " ".join(build_tags),
        "bin_name": os.path.join(BENCHMARKS_BIN_PATH, bin_name("aggregator")),
        "ldflags": ldflags,
        "gcflags": gcflags,
        "REPO_PATH": REPO_PATH
    }
    ctx.run(cmd.format(**args))


@task
def build_dogstatsd(ctx):
    """
    Build Dogstatsd benchmarks.
    """
    build_tags = get_build_tags()  # pass all the build flags

    cmd = "go build -tags \"{build_tags}\" -o {bin_name} {REPO_PATH}/test/benchmarks/dogstatsd"
    args = {
        "build_tags": " ".join(build_tags),
        "bin_name": os.path.join(BENCHMARKS_BIN_PATH, bin_name("dogstatsd")),
        "REPO_PATH": REPO_PATH,
    }
    ctx.run(cmd.format(**args))


@task(pre=[build_dogstatsd])
def dogstastd(ctx):
    """
    Run Dogstatsd Benchmarks.
    """
    bin_path = os.path.join(BENCHMARKS_BIN_PATH, bin_name("dogstatsd"))
    ctx.run("{} -pps=5000 -dur 45 -ser 5 -brk -inc 1000".format(bin_path))
