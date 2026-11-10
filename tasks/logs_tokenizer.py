"""
Tasks for the shared Logs tokenizer (pkg/logs/tokenizer), owned by agent-log-pipelines.
"""

from invoke import task

from tasks.libs.logs_tokenizer.runner import run_compare


@task(
    help={
        "targets": "Package directory to benchmark, e.g. pkg/logs/tokenizer. It must import only the standard library.",
        "bench": "Regular expression of the benchmarks to run, as for go test -bench.",
        "bench_files": "Comma-separated test files of the package that hold those benchmarks. This tree's copies run on both sides.",
        "base": "Revision to compare against. Defaults to the merge base of HEAD and origin/main.",
        "rounds": "Rounds per side, alternating between the sides. At least 4: fewer cannot reach p < 0.05.",
        "benchtime": "The -benchtime of each run.",
        "max_slowdown": "Slowdown, or growth in bytes per call, in percent, above which a benchmark (or the geomean, for time) fails the comparison.",
    }
)
def bench_compare(ctx, targets, bench, bench_files, base=None, rounds=10, benchtime="200ms", max_slowdown="5"):
    """
    Tells you whether your change made a package's benchmarks slower or use more memory, by running
    them on this machine at a base revision (the merge base by default) and in your working tree.

    Exits 0 on a pass, and 1 if a benchmark is more than max_slowdown percent slower (p < 0.05), the
    geomean is, a benchmark allocates more, or its bytes per call grow by more than max_slowdown
    percent. Exits 2 if it couldn't compare. Each side builds as a throwaway module with the
    package's sources and this tree's benchmark files, so the benchmark code is identical and only
    the package differs. The runs alternate between the sides, so background noise on the machine
    hits both equally.
    """
    run_compare(ctx, targets, bench, bench_files, base, rounds, benchtime, max_slowdown)
