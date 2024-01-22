"""
Benchmarking tasks
"""


import os

from invoke import task

from .build_tags import get_default_build_tags
from .libs.common.utils import REPO_PATH, bin_name

# constants
BENCHMARKS_BIN_PATH = os.path.join(".", "bin", "benchmarks")


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


@task(pre=[build_kubernetes_state])
def kubernetes_state(ctx):
    """
    Run Kubernetes_State Benchmarks.
    """
    bin_path = os.path.join(BENCHMARKS_BIN_PATH, bin_name("kubernetes_state"))

    ctx.run(f"{bin_path}")
