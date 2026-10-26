"""
Benchmarking tasks
"""

import os

from invoke import task

from tasks.build_tags import get_default_build_tags
from tasks.libs.bench.runner import run_check_verdict, run_compare
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


@task(
    help={
        "targets": "Package directory to benchmark, e.g. pkg/logs/tokenizer. It must import only the standard library.",
        "bench": "Regular expression of the benchmarks to run, as for go test -bench.",
        "bench_files": "Comma-separated test files of the package that hold those benchmarks. This tree's copies run on both sides.",
        "base": "Revision to compare against. Defaults to the merge base of HEAD and origin/main.",
        "rounds": "Rounds per side, alternating between the sides. At least 4: fewer cannot reach p < 0.05.",
        "benchtime": "The -benchtime of each run.",
        "max_slowdown": "Slowdown, in percent, above which a benchmark or the geomean fails the comparison.",
        "report": "Also write the result as Markdown to this file, for a PR comment.",
        "verdict_file": "Also write the verdict as JSON to this file, for bench.check-verdict.",
        "send_metrics": "Also send each benchmark's result to Datadog (needs DD_API_KEY). Failing to send does not change the result.",
    }
)
def compare(
    ctx,
    targets,
    bench,
    bench_files,
    base=None,
    rounds=10,
    benchtime="200ms",
    max_slowdown="5",
    report=None,
    verdict_file=None,
    send_metrics=False,
):
    """
    Tells you whether your change made a package's benchmarks slower, by running them on this machine
    at a base revision (the merge base by default) and in your working tree.

    Exits 0 on a pass, 1 if a benchmark is more than max_slowdown percent slower (p < 0.05), the
    geomean is, or a benchmark allocates more, and 2 if it couldn't compare. Each side builds as a
    throwaway module with the package's sources and this tree's benchmark files, so the benchmark
    code is identical and only the package differs. The runs alternate between the sides, so
    background noise on the machine hits both equally.
    """
    run_compare(
        ctx, targets, bench, bench_files, base, rounds, benchtime, max_slowdown, report, verdict_file, send_metrics
    )


@task(help={"verdict_file": "The file bench.compare wrote with --verdict-file."})
def check_verdict(_, verdict_file):
    """
    Fails when bench.compare found a regression, unless the branch's pull request has the
    performance/ignore-performance-regression label. Needs GITHUB_TOKEN to read the labels.
    """
    run_check_verdict(verdict_file)
