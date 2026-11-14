"""
Tasks for the shared Logs tokenizer (pkg/logs/tokenizer), owned by agent-log-pipelines.
"""

from invoke import task

from tasks.libs.logs_tokenizer.runner import run_check_verdict, run_compare


@task(
    help={
        "targets": "Package directory to benchmark, e.g. pkg/logs/tokenizer. It must import only the standard library.",
        "bench": "Regular expression of the benchmarks to run, as for go test -bench.",
        "bench_files": "Comma-separated test files of the package that hold those benchmarks. This tree's copies run on both sides.",
        "base": "Revision to compare against. Defaults to the merge base of HEAD and origin/main.",
        "rounds": "Rounds per side, alternating between the sides. At least 4: fewer cannot reach p < 0.05.",
        "benchtime": "The -benchtime of each run.",
        "max_growth": "Growth in bytes per call, in percent, above which a benchmark fails the comparison.",
        "report": "Also write the result as Markdown to this file, for a PR comment.",
        "verdict_file": "Also write the verdict as JSON to this file, for logs-tokenizer.bench-check-verdict.",
        "send_metrics": "Also send each benchmark's result to Datadog (needs DD_API_KEY). Failing to send does not change the result.",
    }
)
def bench_compare(
    ctx,
    targets,
    bench,
    bench_files,
    base=None,
    rounds=10,
    benchtime="200ms",
    max_growth="5",
    report=None,
    verdict_file=None,
    send_metrics=False,
):
    """
    Tells you whether your change made a package's benchmarks allocate more or use more memory, by
    running them on this machine at a base revision (the merge base by default) and in your working
    tree, and shows how their timings moved.

    Exits 0 on a pass, and 1 if a benchmark allocates more, or its bytes per call grow by more than
    max_growth percent (both p < 0.05). Exits 2 if it couldn't compare. Time is not judged: read the
    timings yourself, on a quiet machine.

    Each side builds as a throwaway module with the package's sources and this tree's benchmark
    files, so the benchmark code is identical and only the package differs. The runs alternate
    between the sides in a random order, so background noise on the machine hits both equally.
    """
    run_compare(
        ctx,
        targets,
        bench,
        bench_files,
        base,
        rounds,
        benchtime,
        max_growth,
        report,
        verdict_file,
        send_metrics,
    )


@task(help={"verdict_file": "The file logs-tokenizer.bench-compare wrote with --verdict-file."})
def bench_check_verdict(_, verdict_file):
    """
    Fails when logs-tokenizer.bench-compare found a regression, unless the branch's pull request has the
    performance/ignore-performance-regression label. Needs GITHUB_TOKEN to read the labels.
    """
    run_check_verdict(verdict_file)
