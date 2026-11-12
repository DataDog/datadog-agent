"""
The pass/fail rules of `dda inv logs-tokenizer.bench-compare`: it reads benchstat's CSV output and decides whether
a change made a benchmark slower or allocate more. Nothing here imports invoke, so the rules can be
unit-tested without running a task.
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


def find_regressions(rows: list[Row], max_slowdown: float) -> list[str]:
    """
    Returns one message per regression, like "Consumer/aad_log_signature/applog sec/op: +8.18%".

    Time: a benchmark fails when it's more than max_slowdown percent slower and benchstat says the
    change is real (p < 0.05; otherwise it prints "~"). The geomean has no p-value, so it fails on the
    percentage alone => benchmarks that are each 6% slower but too noisy to be significant still
    fail together.

    Allocations: counts are exact, so any higher median fails. This compares the medians, not the
    percentage, because benchstat prints "?" instead of a percentage when the base is 0 => going from
    0 to 1 allocation still fails.

    Bytes: a benchmark fails when its median B/op grows by more than max_slowdown percent, or from
    0 to anything. Byte counts barely move between runs, so no p-value is needed. This catches an
    allocation that keeps its count but gets bigger, say 50 B to 5 KB per line.
    """
    messages = []
    for row in rows:
        if row.unit == "sec/op" and row.change.startswith("+"):
            if float(row.change.strip("+%")) > max_slowdown:
                messages.append(f"{row.name} sec/op: {row.change}")
        elif row.unit == "allocs/op" and row.name != "geomean":
            if row.base is not None and row.head is not None and row.head > row.base:
                messages.append(f"{row.name} allocs/op: {row.base:g} -> {row.head:g}")
        elif row.unit == "B/op" and row.name != "geomean":
            if row.base is None or row.head is None or row.head <= row.base:
                continue
            if row.base == 0 or (row.head / row.base - 1) * 100 > max_slowdown:
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


def markdown_report(
    title: str, rows: list[Row], regressions: list[str], base: str, max_slowdown: float, benchstat: str
) -> str:
    """Writes the PR comment: the verdict first, then the changes that matter, then benchstat's tables."""
    verdict = "Regression" if regressions else "No regression"
    lines = [
        f"### {title}",
        "",
        f"**{verdict}** against `{base[:11]}`, with the sides alternating on one runner. A benchmark fails when it is "
        f"more than {max_slowdown:g}% slower (p < 0.05), when the geomean is more than {max_slowdown:g}% slower, "
        f"when it allocates more, or when its bytes per call grow by more than {max_slowdown:g}%.",
    ]
    if regressions:
        lines += ["", *[f"- `{message}`" for message in regressions]]
        lines += [
            "",
            f"If the regression is expected, for example from a crash or security fix, add the `{BYPASS_LABEL}` label.",
        ]
    changed = [
        row for row in rows if row.unit in ("sec/op", "B/op", "allocs/op") and row.change not in ("", "~", "+0.00%")
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
