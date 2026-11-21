"""
The pass/fail rules of `dda inv logs-tokenizer.bench-compare`: it reads benchstat's CSV output and decides whether
a change made a benchmark allocate more or use more bytes per call (time is not judged, see find_regressions).
Nothing here imports invoke, so the rules can be unit-tested without running a task.
"""

from __future__ import annotations

import csv
import io
from dataclasses import dataclass

# Exit codes of `dda inv logs-tokenizer.bench-compare`, besides 0 for a pass.
EXIT_REGRESSION = 1
EXIT_CANNOT_COMPARE = 2

# Add this label to a PR when a slowdown is expected, for example from a crash or security fix.
# It's the same label the Benchmarking Platform's PR gates use.
BYPASS_LABEL = "performance/ignore-performance-regression"


@dataclass(frozen=True)
class Row:
    """One benchmark, or the geomean, in one unit table of benchstat's CSV output."""

    unit: str  # "sec/op", "B/op", "allocs/op", "B/s", ...
    name: str  # the benchmark name, or "geomean"
    base: float | None  # median on the base side; None when benchstat printed nothing
    head: float | None  # median on the head side
    change: str  # the "vs base" cell: "+7.79%", "-5.81%", "~" (not significant), "?" or ""


def _number(cell: str) -> float | None:
    return float(cell) if cell else None


def parse_benchstat_csv(text: str) -> list[Row]:
    """
    Parses the output of `benchstat -format csv base=<file> head=<file>`.

    Each unit table starts with a row naming the columns (",base,,head,,,") and a row naming the
    unit (",sec/op,CI,sec/op,CI,vs base,P"), then one row per benchmark and a geomean row. Blank
    lines separate the tables, and the configuration lines before them ("goos: linux") have no
    commas.
    """
    rows = []
    unit = None
    for record in csv.reader(io.StringIO(text)):
        if not record:
            unit = None
            continue
        if len(record) < 6:
            continue
        if record[0] == "":
            if record[1] != "base":
                unit = record[1]
            continue
        if unit is not None:
            rows.append(Row(unit, record[0], _number(record[1]), _number(record[3]), record[5]))
    return rows


def find_regressions(rows: list[Row], max_growth: float) -> list[str]:
    """
    Returns one message per regression, like "Consumer/aad_log_signature/applog allocs/op: 1 -> 2".

    Allocations: any higher median fails. This compares the medians, not the percentage, because
    benchstat prints "?" instead of a percentage when the base is 0 => going from 0 to 1 allocation
    still fails.

    Bytes: a benchmark fails when its median B/op grows by more than max_growth percent, or from 0 to
    anything. This catches an allocation that keeps its count but gets bigger, say 50 B to 5 KB per
    line.

    Both need benchstat to call the change significant (not "~"). Counts are exact per allocation,
    but an allocation that happens on only some iterations (a buffer that grows now and then) makes
    the per-call value fractional, and that wobbles: a CI run of identical code measured one
    benchmark at 2.5 B/op on one side and 3 on the other (+20%, p=0.350).

    Time is not judged. On CI runners, A/A runs (identical code) have ± bands of 11% to 60%, and a change
    to TokensToString made benchmarks that never call it 11% to 19% faster (p < 0.05) on CI but not on a
    laptop: code layout, not the change (go.dev/issue/8717). The timings stay in the report's benchstat
    output and in the nightly metrics.
    """
    messages = []
    for row in rows:
        if row.name == "geomean" or row.change == "~":
            continue
        if row.unit == "allocs/op":
            if row.base is not None and row.head is not None and row.head > row.base:
                messages.append(f"{row.name} allocs/op: {row.base:g} -> {row.head:g}")
        elif row.unit == "B/op":
            if row.base is None or row.head is None or row.head <= row.base:
                continue
            if row.base == 0 or (row.head / row.base - 1) * 100 > max_growth:
                messages.append(f"{row.name} B/op: {row.base:g} -> {row.head:g}")
    return messages


def _format(unit: str, value: float | None) -> str:
    if value is None:
        return ""
    if unit != "sec/op":
        return f"{value:g}"
    for scale, suffix in ((1e-6, "ns"), (1e-3, "µs"), (1, "ms")):
        if value < scale:
            return f"{value * 1e3 / scale:.4g} {suffix}"
    return f"{value:.4g} s"


# Shown in every report: on CI and on a laptop alike, the timings are for a person to read, not a check.
TIME_NOTE = (
    "Time is not judged: one run on a shared machine can't time a 5-10% change, and changing one function can "
    "move the timing of others through code layout. The timings are in the benchstat output below. To compare "
    "them, run `dda inv logs-tokenizer.bench-compare` on a quiet machine, and trust a change in the code you touched."
)


def markdown_report(
    title: str, rows: list[Row], regressions: list[str], base: str, max_growth: float, benchstat: str
) -> str:
    """Writes the PR comment: the verdict first, then the allocation and byte changes, then benchstat's tables."""
    verdict = "Regression" if regressions else "No regression"
    lines = [
        f"### {title}",
        "",
        f"**{verdict}** against `{base[:11]}`. A benchmark fails when it allocates more, or when its bytes per "
        f"call grow by more than {max_growth:g}% (both p < 0.05).",
    ]
    if regressions:
        lines += ["", *[f"- `{message}`" for message in regressions]]
        lines += [
            "",
            f"If the regression is expected, for example from a crash or security fix, add the `{BYPASS_LABEL}` label.",
        ]
    # The geomeans aren't judged, and with zeros in them benchstat can't compute them.
    changed = [
        row
        for row in rows
        if row.unit in ("B/op", "allocs/op")
        and row.name != "geomean"
        and row.change not in ("", "~", "+0.00%", "-0.00%")
    ]
    if changed:
        lines += ["", "| Benchmark | | Base | Head | Change |", "| --- | --- | --- | --- | --- |"]
        for row in changed:
            change = row.change if row.change != "?" else ("new allocations" if row.unit == "allocs/op" else "was 0")
            lines.append(
                f"| {row.name} | {row.unit} | {_format(row.unit, row.base)} | {_format(row.unit, row.head)} | {change} |"
            )
    lines += [
        "",
        TIME_NOTE,
        "",
        "<details><summary>benchstat output</summary>",
        "",
        "```",
        benchstat.strip(),
        "```",
        "",
        "</details>",
        "",
    ]
    return "\n".join(lines)


def benchmark_metrics(rows: list[Row]) -> list[tuple[str, float, str, str | None]]:
    """
    Returns the metrics to send to Datadog, as (metric, value, benchmark, unit): each benchmark's time
    allocations and bytes per call on the head side, and its time change against the base. The change is
    sent even when it isn't significant => the dashboard also shows how noisy the runner is.
    """
    metrics = []
    for row in rows:
        if row.head is None:
            continue
        if row.unit == "sec/op":
            metrics.append(("datadog.agent.benchmark.time_per_op", row.head * 1e9, row.name, "nanosecond"))
            if row.base:
                metrics.append(
                    ("datadog.agent.benchmark.time_change", (row.head / row.base - 1) * 100, row.name, "percent")
                )
        elif row.unit == "allocs/op" and row.name != "geomean":
            metrics.append(("datadog.agent.benchmark.allocs_per_op", row.head, row.name, None))
        elif row.unit == "B/op" and row.name != "geomean":
            metrics.append(("datadog.agent.benchmark.bytes_per_op", row.head, row.name, "byte"))
    return metrics


def bypassed(verdict: dict, labels: list[str]) -> bool:
    """Returns True when there's a regression but the PR has the bypass label, so it shouldn't fail."""
    return verdict["verdict"] == "regression" and BYPASS_LABEL in labels
