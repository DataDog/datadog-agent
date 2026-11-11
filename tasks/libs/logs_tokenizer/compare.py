"""
The pass/fail rules of `dda inv logs-tokenizer.bench-compare`: it reads benchstat's CSV output and decides whether
a change made a benchmark slower or allocate more. Nothing here imports invoke, so the rules can be
unit-tested without running a task.
"""

from __future__ import annotations

import csv
import io
import math
import statistics
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
    base_ci: float | None = None  # benchstat's ± band on the base side, in percent; None when it printed none
    head_ci: float | None = None  # the same on the head side

    @property
    def band(self) -> float | None:
        """The wider of the two ± bands, in percent: run-to-run spread this row's change has to beat."""
        bands = [ci for ci in (self.base_ci, self.head_ci) if ci is not None]
        return max(bands) if bands else None


def _number(cell: str) -> float | None:
    return float(cell) if cell else None


def _percent(cell: str) -> float | None:
    if cell == "∞":
        # benchstat's band when it has too few samples to bound it (fewer than 6 per side, as with
        # --rounds=4 or 5): the uncertainty is unbounded, not unknown.
        return math.inf
    try:
        return float(cell.rstrip("%"))
    except ValueError:  # "" for the geomean
        return None


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
            rows.append(
                Row(
                    unit,
                    record[0],
                    _number(record[1]),
                    _number(record[3]),
                    record[5],
                    _percent(record[2]),
                    _percent(record[4]),
                )
            )
    return rows


def find_regressions(rows: list[Row], max_slowdown: float) -> list[str]:
    """
    Returns one message per regression, like "Consumer/aad_log_signature/applog sec/op: +8.18%".

    Time: only judged when the run is quiet (see is_noisy). On a noisy runner a ±60% band makes a 10%
    slowdown invisible and a 10% "slowdown" of untouched code likely, so time neither fails nor passes
    the run => only allocations and bytes decide it. On a quiet run, a benchmark fails when it's more
    than max_slowdown percent slower and benchstat says the change is real (p < 0.05; otherwise it
    prints "~"). The geomean has no p-value, so it fails on the percentage alone => benchmarks that are
    each 6% slower but too noisy to be significant still fail together.

    Allocations: any higher median fails. This compares the medians, not the percentage, because
    benchstat prints "?" instead of a percentage when the base is 0 => going from 0 to 1 allocation
    still fails.

    Bytes: a benchmark fails when its median B/op grows by more than max_slowdown percent, or from
    0 to anything. This catches an allocation that keeps its count but gets bigger, say 50 B to 5 KB
    per line.

    Both need benchstat to call the change significant (not "~"). Counts are exact per allocation,
    but an allocation that happens on only some iterations (a buffer that grows now and then) makes
    the per-call value fractional, and that wobbles: a CI run of identical code measured one
    benchmark at 2.5 B/op on one side and 3 on the other (+20%, p=0.350).
    """
    judge_time = not is_noisy(rows, max_slowdown)
    messages = []
    for row in rows:
        if row.unit == "sec/op":
            if judge_time and row.change.startswith("+") and float(row.change.strip("+%")) > max_slowdown:
                messages.append(f"{row.name} sec/op: {row.change}")
        elif row.unit == "allocs/op" and row.name != "geomean" and row.change != "~":
            if row.base is not None and row.head is not None and row.head > row.base:
                messages.append(f"{row.name} allocs/op: {row.base:g} -> {row.head:g}")
        elif row.unit == "B/op" and row.name != "geomean" and row.change != "~":
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


def median_band(rows: list[Row]) -> float | None:
    """
    Returns the median ± band, in percent, over the per-benchmark time rows (infinite when benchstat
    couldn't bound most of them); None when there are none.
    """
    bands = [row.band for row in rows if row.unit == "sec/op" and row.name != "geomean" and row.band is not None]
    return statistics.median(bands) if bands else None


def is_noisy(rows: list[Row], threshold: float) -> bool:
    """
    True when the typical time row's ± band is above threshold percent (max_slowdown): the run can't
    tell a change of that size from noise, so its times say nothing either way.
    """
    band = median_band(rows)
    return band is not None and band > threshold


def _band_text(band: float | None) -> str:
    if band is None:
        return ""
    return "∞" if math.isinf(band) else f"{band:.0f}%"


def _change_cell(row: Row) -> str:
    change = row.change if row.change != "?" else ("new allocations" if row.unit == "allocs/op" else "was 0")
    if row.unit == "sec/op" and row.band is not None:
        percent = _percent(row.change.replace("+", ""))
        if percent is not None and abs(percent) < row.band:
            return f"{change} (within noise)"
    return change


def markdown_report(
    title: str, rows: list[Row], regressions: list[str], base: str, max_slowdown: float, benchstat: str
) -> str:
    """
    Writes the PR comment: the verdict first, then the exact changes (allocations and bytes, the same on
    machine), then the time changes with their ± bands, then benchstat's tables. On a noisy run the
    time section is one line saying time wasn't judged, with the numbers left in benchstat's tables.
    """
    noisy = is_noisy(rows, max_slowdown)
    verdict = "Regression" if regressions else "No regression"
    rules = f"when it allocates more, or when its bytes per call grow by more than {max_slowdown:g}% (both p < 0.05)"
    if noisy:
        rules = f"Time was not judged on this run (see below). A benchmark fails {rules}."
    else:
        rules = (
            f"A benchmark fails when it is more than {max_slowdown:g}% slower (p < 0.05), when the geomean is more "
            f"than {max_slowdown:g}% slower, {rules}. Time is only judged when the run is quiet: its median ± band "
            f"is at most {max_slowdown:g}%."
        )
    lines = [
        f"### {title}",
        "",
        f"**{verdict}** against `{base[:11]}`, with the sides alternating on one runner (which side goes first flips "
        f"every round). {rules}",
    ]
    if regressions:
        lines += ["", *[f"- `{message}`" for message in regressions]]
        lines += [
            "",
            f"If the regression is expected, for example from a crash or security fix, add the `{BYPASS_LABEL}` label.",
        ]

    def changed(*units: str) -> list[Row]:
        return [
            row
            for unit in units
            for row in rows
            if row.unit == unit and row.change not in ("", "~", "+0.00%", "-0.00%")
        ]

    # The geomeans of allocations and bytes aren't judged, and with zeros in them benchstat can't
    # compute them => they'd only be clutter here.
    exact = [row for row in changed("allocs/op", "B/op") if row.name != "geomean"]
    if exact:
        lines += [
            "",
            "**Allocations and bytes per call** (they don't depend on the machine)",
            "",
            "| Benchmark | | Base | Head | Change |",
            "| --- | --- | --- | --- | --- |",
        ]
        for row in exact:
            lines.append(
                f"| {row.name} | {row.unit} | {_format(row.unit, row.base)} | {_format(row.unit, row.head)} | {_change_cell(row)} |"
            )
    timed = changed("sec/op")
    if noisy:
        # No table: a -37% row that beats its own ±31% band still means nothing when the typical band is
        # ±60%, and a table invites reading it as a result.
        band = median_band(rows)
        if math.isinf(band):
            reason = "with fewer than 6 rounds, benchstat can't bound the timings (± ∞)"
        else:
            reason = f"this runner was too noisy (median ± band {_band_text(band)}, above {max_slowdown:g}%)"
        lines += [
            "",
            "**Time**",
            "",
            f"Not judged: {reason}, so the timings can't tell a change from noise. They're in the benchstat output "
            "below. For real timings, run `dda inv logs-tokenizer.bench-compare` on a quiet machine.",
        ]
    elif timed:
        lines += [
            "",
            "**Time**",
            "",
            "| Benchmark | | Base | Head | ± | Change |",
            "| --- | --- | --- | --- | --- | --- |",
        ]
        for row in timed:
            lines.append(
                f"| {row.name} | {row.unit} | {_format(row.unit, row.base)} | {_format(row.unit, row.head)} | {_band_text(row.band)} | {_change_cell(row)} |"
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
