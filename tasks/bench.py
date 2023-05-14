"""
Benchmarking tasks
"""


import os
import sys

from invoke import task

from .build_tags import get_default_build_tags
from .utils import REPO_PATH, bin_name, get_git_branch_name

# constants
BENCHMARKS_BIN_PATH = os.path.join(".", "bin", "benchmarks")


@task
def build_aggregator(ctx, rebuild=False, arch="x64"):
    """
    Build the Aggregator benchmarks.
    """
    build_tags = get_default_build_tags(build="test", arch=arch)  # pass all the build flags

    ldflags = ""
    gcflags = ""

    if os.environ.get("DELVE"):
        gcflags = "-N -l"
        if sys.platform == 'win32':
            # On windows, need to build with the extra argument -ldflags="-linkmode internal"
            # if you want to be able to use the delve debugger.
            ldflags += " -linkmode internal"

    cmd = "go build -mod={go_mod} {build_type} -tags \"{build_tags}\" -o {bin_name} "
    cmd += "{ldflags} {gcflags} {REPO_PATH}/test/benchmarks/aggregator"
    args = {
        "go_mod": "mod",
        "build_type": "-a" if rebuild else "",
        "build_tags": " ".join(build_tags),
        "bin_name": os.path.join(BENCHMARKS_BIN_PATH, bin_name("aggregator")),
        "ldflags": ldflags,
        "gcflags": gcflags,
        "REPO_PATH": REPO_PATH,
    }
    ctx.run(cmd.format(**args))


@task
def build_dogstatsd(ctx, arch="x64"):
    """
    Build Dogstatsd benchmarks.
    """
    build_tags = get_default_build_tags(build="test", arch=arch)  # pass all the build flags

    cmd = "go build -mod={go_mod} -tags \"{build_tags}\" -o {bin_name} {REPO_PATH}/test/benchmarks/dogstatsd"
    args = {
        "go_mod": "mod",
        "build_tags": " ".join(build_tags),
        "bin_name": os.path.join(BENCHMARKS_BIN_PATH, bin_name("dogstatsd")),
        "REPO_PATH": REPO_PATH,
    }
    ctx.run(cmd.format(**args))


@task
def build_kubernetes_state(ctx, arch="x64"):
    """
    Build Kubernetes_State benchmarks.
    """
    build_tags = get_default_build_tags(build="test", arch=arch)  # pass all the build flags

    cmd = "go build -mod={go_mod} -tags \"{build_tags}\" -o {bin_name} {REPO_PATH}/test/benchmarks/kubernetes_state"
    args = {
        "go_mod": "mod",
        "build_tags": " ".join(build_tags),
        "bin_name": os.path.join(BENCHMARKS_BIN_PATH, bin_name("kubernetes_state")),
        "REPO_PATH": REPO_PATH,
    }
    ctx.run(cmd.format(**args))


@task(pre=[build_dogstatsd])
def dogstatsd(ctx):
    """
    Run Dogstatsd Benchmarks.
    """
    bin_path = os.path.join(BENCHMARKS_BIN_PATH, bin_name("dogstatsd"))
    branch_name = os.environ.get("DD_REPO_BRANCH_NAME") or get_git_branch_name()
    options = f"-branch {branch_name}"

    key = os.environ.get("DD_AGENT_API_KEY")
    if key:
        options += f" -api-key {key}"

    ctx.run(f"{bin_path} -pps=5000 -dur 45 -ser 5 -brk -inc 1000 {options}")


# Temporarily keep compatibility after typo fix
@task(pre=[build_dogstatsd])
def dogstastd(ctx):
    dogstatsd(ctx)


@task(pre=[build_aggregator])
def aggregator(ctx):
    """
    Run the Aggregator Benchmarks.
    """
    bin_path = os.path.join(BENCHMARKS_BIN_PATH, bin_name("aggregator"))
    branch_name = os.environ.get("DD_REPO_BRANCH_NAME") or get_git_branch_name()
    options = f"-branch {branch_name}"

    key = os.environ.get("DD_AGENT_API_KEY")
    if key:
        options += f" -api-key {key}"

    ctx.run(f"{bin_path} -points 2,10,100,500,1000 -series 10,100,1000 -log-level info -json {options}")
    ctx.run(
        f"{bin_path} -points 2,10,100,500,1000 -series 10,100,1000 -log-level info -json -memory -duration 10 {options}"
    )


@task(pre=[build_kubernetes_state])
def kubernetes_state(ctx):
    """
    Run Kubernetes_State Benchmarks.
    """
    bin_path = os.path.join(BENCHMARKS_BIN_PATH, bin_name("kubernetes_state"))

    ctx.run(f"{bin_path}")
