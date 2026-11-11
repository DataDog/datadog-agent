"""
The pass/fail rules of `dda inv logs-tokenizer.bench-compare`: it reads benchstat's CSV output and decides whether
a change made a benchmark allocate more or use more bytes per call. Time is reported but never judged: on a
shared CI runner one run can't tell a 5-10% change from noise, and a change to one function can move the
timings of functions it didn't touch by more than that (code layout). Nothing here imports invoke, so the
rules can be unit-tested without running a task.
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

    Time never fails the run, on any machine. On CI runners A/A runs (identical code) have ± bands of
    11% to 60%, and a change to TokensToString made benchmarks that never call it 11% to 19% faster
    (p < 0.05) on CI but not on a laptop: code layout, not the change. Timings are in the report and
    the nightly metrics; judge them yourself on a quiet machine.
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


def median_band(rows: list[Row]) -> float | None:
    """
    Returns the median ± band, in percent, over the per-benchmark time rows (infinite when benchstat
    couldn't bound most of them); None when there are none.
    """
    bands = [row.band for row in rows if row.unit == "sec/op" and row.name != "geomean" and row.band is not None]
    return statistics.median(bands) if bands else None


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
    title: str,
    rows: list[Row],
    regressions: list[str],
    base: str,
    max_growth: float,
    benchstat: str,
    show_time: bool = True,
) -> str:
    """
    Writes the report: the verdict first, then the allocation and byte changes, then the time
    changes (never judged), then benchstat's tables.

    show_time=False replaces the time section with one line, for CI runners: their timings can't be
    trusted, and a table of them reads as a result anyway. On a noisy run (median ± band above
    max_growth) the time section is one line too, with the numbers left in benchstat's tables. On a
    quiet run it's a table with each row's ± band, and a change smaller than its band is marked.
    """
    verdict = "Regression" if regressions else "No regression"
    lines = [
        f"### {title}",
        "",
        f"**{verdict}** against `{base[:11]}`. A benchmark fails when it allocates more, or when its bytes per "
        f"call grow by more than {max_growth:g}% (both p < 0.05). Time is not judged (see below).",
    ]
    if regressions:
        lines += ["", *[f"- `{message}`" for message in regressions]]
        lines += [
            "",
            f"If the regression is expected, for example from a crash or security fix, add the `{BYPASS_LABEL}` label.",
        ]

    def changed(unit: str) -> list[Row]:
        return [row for row in rows if row.unit == unit and row.change not in ("", "~", "+0.00%", "-0.00%")]

    # The geomeans of allocations and bytes aren't judged, and with zeros in them benchstat can't
    # compute them => they'd only be clutter here.
    exact = [row for row in changed("allocs/op") + changed("B/op") if row.name != "geomean"]
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
    lines += ["", "**Time** (not judged)", ""]
    band = median_band(rows)
    if not show_time:
        lines.append(
            "Not shown on CI: shared runners are too noisy to time a change (A/A runs have ± bands of 11% to 60%), "
            "and code layout moves benchmarks the change didn't touch. To compare timings, run "
            "`dda inv logs-tokenizer.bench-compare` on a quiet machine. They're in the benchstat output below."
        )
    elif band is not None and band > max_growth:
        if math.isinf(band):
            reason = "with fewer than 6 rounds, benchstat can't bound the timings (± ∞)"
        else:
            reason = f"this machine was too noisy (median ± band {_band_text(band)}, above {max_growth:g}%)"
        lines.append(
            f"Inconclusive: {reason}. The numbers are in the benchstat output below. Run "
            "`dda inv logs-tokenizer.bench-compare` on a quiet machine to compare timings."
        )
    else:
        timed = changed("sec/op")
        if timed:
            lines += ["| Benchmark | | Base | Head | ± | Change |", "| --- | --- | --- | --- | --- | --- |"]
            for row in timed:
                lines.append(
                    f"| {row.name} | {row.unit} | {_format(row.unit, row.base)} | {_format(row.unit, row.head)} | {_band_text(row.band)} | {_change_cell(row)} |"
                )
        else:
            lines.append("No significant change.")
        lines += [
            "",
            "Even on a quiet machine a change can move the timing of code it didn't touch, through code layout: "
            "trust a speedup or slowdown in the code the change touched, or one you see on two machines.",
        ]
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
