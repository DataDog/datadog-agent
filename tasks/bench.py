"""
Benchmarking tasks
"""

import os

from invoke import task

from tasks.build_tags import get_default_build_tags
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import REPO_PATH, bin_name

# constants
BENCHMARKS_BIN_PATH = os.path.join(".", "bin", "benchmarks")


@task
def build_kubernetes_state(ctx):
    """
    Build Kubernetes_State benchmarks.
    """
    build_tags = get_default_build_tags(build="test")  # pass all the build flags

    go_build(
        ctx,
        f"{REPO_PATH}/test/benchmarks/kubernetes_state",
        mod="readonly",
        build_tags=build_tags,
        bin_path=os.path.join(BENCHMARKS_BIN_PATH, bin_name("kubernetes_state")),
    )


@task(pre=[build_kubernetes_state])
def kubernetes_state(ctx):
    """
    Run Kubernetes_State Benchmarks.
    """
    bin_path = os.path.join(BENCHMARKS_BIN_PATH, bin_name("kubernetes_state"))

    ctx.run(f"{bin_path}")
