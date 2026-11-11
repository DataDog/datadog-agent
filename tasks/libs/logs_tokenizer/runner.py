"""
Runner behind `dda inv logs-tokenizer.bench-compare` and `dda inv logs-tokenizer.bench-check-verdict` (tasks/logs_tokenizer.py): builds both
sides, runs the benchmarks in alternating rounds, turns benchstat's verdict into an exit code, and
reports it. The pass/fail rules live in compare.py, which has no invoke dependency so it can be
unit-tested on its own.
"""

import contextlib
import json
import os
import random
import shutil
import tempfile
import time
from pathlib import Path

from invoke.exceptions import Exit

from tasks.libs.common.constants import ORIGIN_CATEGORY, ORIGIN_PRODUCT, ORIGIN_SERVICE
from tasks.libs.common.utils import bin_name, get_metric_origin, join_command
from tasks.libs.logs_tokenizer.compare import (
    BYPASS_LABEL,
    EXIT_CANNOT_COMPARE,
    EXIT_REGRESSION,
    benchmark_metrics,
    bypassed,
    find_regressions,
    markdown_report,
    parse_benchstat_csv,
)


def _cannot_compare(message):
    return Exit(message=message, code=EXIT_CANNOT_COMPARE)


def _checked(ctx, args, **kwargs):
    # Without this, a failing git or go command would exit with its own code, and exit 1 would look
    # like a regression. Raising here turns every tool failure into exit 2, "cannot compare".
    result = ctx.run(join_command(args), warn=True, **kwargs)
    if result.failed:
        raise _cannot_compare(f"`{join_command(args)}` failed: {result.stderr.strip()}")
    return result


def _find_benchstat(ctx):
    # dda inv install-tools puts benchstat in GOPATH/bin, which often isn't on PATH, so look there too.
    if found := shutil.which("benchstat"):
        return found
    gopath = ctx.run("go env GOPATH", hide=True).stdout.strip()
    candidate = Path(gopath, "bin", bin_name("benchstat"))
    if candidate.exists():
        return str(candidate)
    raise _cannot_compare("benchstat not found: run dda inv install-tools")


def run_compare(
    ctx, targets, bench, bench_files, base, rounds, benchtime, max_growth, report, verdict_file, send_metrics, show_time
):
    """Runs `dda inv logs-tokenizer.bench-compare`; see the task's docstring for what it checks."""
    if rounds < 4:
        raise _cannot_compare(f"--rounds must be at least 4, got {rounds}")
    try:
        max_growth = float(max_growth)
    except ValueError:
        raise _cannot_compare(f"--max-growth must be a number of percent like 5, got '{max_growth}'") from None
    benchstat = _find_benchstat(ctx)

    root = Path(_checked(ctx, ["git", "rev-parse", "--show-toplevel"], hide=True).stdout.strip())
    pkg = targets.strip().removeprefix("./").rstrip("/")
    if base is None:
        base = _checked(ctx, ["git", "-C", str(root), "merge-base", "HEAD", "origin/main"], hide=True).stdout.strip()

    go_mod = (root / "go.mod").read_text(encoding="utf-8").splitlines()
    module_line = next(line for line in go_mod if line.startswith("module "))
    go_line = next(line for line in go_mod if line.startswith("go "))

    files = {"base": {}, "head": {}}
    for path in sorted((root / pkg).glob("*.go")):
        if not path.name.endswith("_test.go"):
            files["head"][path.name] = path.read_text(encoding="utf-8")
    if not files["head"]:
        raise _cannot_compare(f"no Go sources in {root / pkg}")
    listing = _checked(ctx, ["git", "-C", str(root), "ls-tree", "--name-only", base, "--", f"{pkg}/"], hide=True)
    for name in listing.stdout.split():
        if name.endswith(".go") and not name.endswith("_test.go"):
            show = _checked(ctx, ["git", "-C", str(root), "show", f"{base}:{name}"], hide=True, encoding="utf-8")
            files["base"][Path(name).name] = show.stdout
    if not files["base"]:
        raise _cannot_compare(f"cannot read {pkg} at {base}")
    for name in bench_files.split(","):
        source = root / pkg / name.strip()
        if not source.exists():
            raise _cannot_compare(f"benchmark file {source} not found")
        for side in files.values():
            side[source.name] = source.read_text(encoding="utf-8")

    work = Path(tempfile.mkdtemp(prefix="bench-compare-"))
    try:
        binaries = {}
        for side, sources in files.items():
            package_dir = work / side / pkg
            package_dir.mkdir(parents=True)
            (work / side / "go.mod").write_text(f"{module_line}\n\n{go_line}\n", encoding="utf-8")
            for name, text in sources.items():
                (package_dir / name).write_text(text, encoding="utf-8")
            binaries[side] = work / bin_name(f"{side}.test")
            # -trimpath strips the temp directory from the binary => if both sides have the same
            # sources, they build the exact same binary, and nothing but the code can differ.
            build = ["go", "test", "-c", "-trimpath", "-o", str(binaries[side]), f"./{pkg}"]
            with contextlib.chdir(work / side):
                result = ctx.run(
                    join_command(build), env={"CGO_ENABLED": "0", "GOWORK": "off", "GOFLAGS": ""}, warn=True
                )
            if result.failed:
                raise _cannot_compare(
                    f"cannot build the {side} side: does {base} have the API the benchmarks use, and does {pkg} import only the standard library?"
                )

        print(f"Comparing {base[:11]} with the working tree at {root}: {rounds} rounds of {bench}")
        outputs = {side: work / f"{side}.txt" for side in files}
        # Which side goes first is random each round, with each side first in half the rounds: the side
        # that runs second gets a warmer (or busier) machine, so base-first every round would favor one
        # side, and a fixed pattern like even/odd can line up with interference that comes and goes on
        # a schedule (Abedi and Brecht, ICPE 2017).
        base_first = [True] * (rounds // 2) + [False] * (rounds - rounds // 2)
        random.shuffle(base_first)
        for first in base_first:
            for side in ("base", "head") if first else ("head", "base"):
                run = [str(binaries[side]), "-test.run", "^$", "-test.bench", bench]
                run += ["-test.benchmem", "-test.benchtime", benchtime, "-test.cpu", "1"]
                with contextlib.chdir(work / side / pkg):
                    result = ctx.run(join_command(run), hide=True, warn=True)
                with outputs[side].open("a", encoding="utf-8") as f:
                    f.write(result.stdout)
                if result.failed:
                    print("\n".join((result.stdout + result.stderr).splitlines()[-20:]))
                    raise _cannot_compare(f"the {side} benchmarks failed")
        if not any(line.startswith("Benchmark") for line in outputs["head"].read_text(encoding="utf-8").splitlines()):
            raise _cannot_compare(f"no benchmark matches {bench}: nothing was compared")

        sides = [f"base={outputs['base']}", f"head={outputs['head']}"]
        text = _checked(ctx, [benchstat, *sides]).stdout
        table = _checked(ctx, [benchstat, "-format", "csv", *sides], hide=True).stdout
    finally:
        shutil.rmtree(work, ignore_errors=True)

    rows = parse_benchstat_csv(table)
    regressions = find_regressions(rows, max_growth)
    if report:
        markdown = markdown_report(f"Benchmarks of `{pkg}`", rows, regressions, base, max_growth, text, show_time)
        Path(report).write_text(markdown, encoding="utf-8")
    if verdict_file:
        verdict = {"verdict": "regression" if regressions else "pass", "regressions": regressions, "base": base}
        Path(verdict_file).write_text(json.dumps(verdict, indent=2), encoding="utf-8")
    if send_metrics:
        _send_benchmark_metrics(ctx, pkg, rows)
    for message in regressions:
        print(f"REGRESSION {message}")
    if regressions:
        raise Exit(
            message="FAIL: a benchmark allocates more or uses more bytes per call (see REGRESSION lines)",
            code=EXIT_REGRESSION,
        )
    print(
        f"PASS: no benchmark allocates more or uses more than {max_growth:g}% more bytes per call. "
        "Time is not judged: read the sec/op table above"
    )


def _send_benchmark_metrics(ctx, pkg, rows):
    from tasks.libs.common.datadog_api import create_gauge, send_metrics

    git_ref = (
        os.environ.get("CI_COMMIT_REF_NAME") or ctx.run("git rev-parse --abbrev-ref HEAD", hide=True).stdout.strip()
    )
    timestamp = int(time.time())
    origin = get_metric_origin(ORIGIN_PRODUCT, ORIGIN_CATEGORY, ORIGIN_SERVICE)
    series = [
        create_gauge(
            metric,
            timestamp,
            value,
            tags=[f"package:{pkg}", f"benchmark:{benchmark}", f"git_ref:{git_ref}"],
            unit=unit,
            metric_origin=origin,
        )
        for metric, value, benchmark, unit in benchmark_metrics(rows)
    ]
    try:
        send_metrics(series)
    except Exception as e:  # Best effort: if Datadog is unreachable, the comparison still passes.
        print(f"Could not send the benchmark metrics to Datadog: {e}")
    else:
        print(f"Sent {len(series)} benchmark metrics to Datadog")


def run_check_verdict(verdict_file):
    """Runs `dda inv logs-tokenizer.bench-check-verdict`; see the task's docstring."""
    verdict = json.loads(Path(verdict_file).read_text(encoding="utf-8"))
    if verdict["verdict"] == "pass":
        print("No regression")
        return
    from tasks.libs.ciproviders.github_api import GithubAPI

    labels = []
    if branch := os.environ.get("CI_COMMIT_BRANCH"):
        prs = list(GithubAPI().get_pr_for_branch(branch))
        if prs:
            labels = [label.name for label in prs[0].get_labels()]
    if bypassed(verdict, labels):
        print(f"Regression accepted by the {BYPASS_LABEL} label: {'; '.join(verdict['regressions'])}")
        return
    raise Exit(
        message=f"Regression: {'; '.join(verdict['regressions'])}. If it is expected, add the {BYPASS_LABEL} label.",
        code=EXIT_REGRESSION,
    )
