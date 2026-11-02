"""
The pass/fail rules of `dda inv bench.compare`: it reads benchstat's CSV output and decides whether
a change made a benchmark slower or allocate more. Nothing here imports invoke, so the rules can be
unit-tested without running a task.
"""

from __future__ import annotations

import csv
import io
from dataclasses import dataclass

# Exit codes of `dda inv bench.compare`, besides 0 for a pass.
EXIT_REGRESSION = 1
EXIT_CANNOT_COMPARE = 2


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
    """
    messages = []
    for row in rows:
        if row.unit == "sec/op" and row.change.startswith("+"):
            if float(row.change.strip("+%")) > max_slowdown:
                messages.append(f"{row.name} sec/op: {row.change}")
        elif row.unit == "allocs/op" and row.name != "geomean":
            if row.base is not None and row.head is not None and row.head > row.base:
                messages.append(f"{row.name} allocs/op: {row.base:g} -> {row.head:g}")
    return messages
