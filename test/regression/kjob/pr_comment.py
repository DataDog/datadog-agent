"""Write the PR comment payload for the single_machine_performance-kjob CI job.

The job saves its outputs in one directory. This script reads them and writes a
JSON payload for the pr-commenter service. The layout follows the regression
detector report in test/regression/report_template.md.j2. `smp report render`
does not support the KJob report, so this script renders it.

The script always writes a payload, also when the job stopped before the KJob
report was ready, so that the PR shows each run.
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any
from urllib.parse import quote

HEADER = "KJob"
LOGS_URL = (
    "https://app.datadoghq.com/logs?query={query}&index=single-machine-performance-target-logs"
    "&messageDisplay=inline&stream_sort=time%2Cdesc&viz=stream"
)
MIB = 1024 * 1024


def read_json(path: Path) -> Any:
    """Return the parsed file, or None when the file is missing or not valid JSON."""
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, ValueError):
        return None


def read_text(path: Path) -> str | None:
    try:
        return path.read_text(encoding="utf-8").strip() or None
    except OSError:
        return None


def format_value(value: Any, series: str) -> str:
    """Show bytes in MiB for memory series. Report values use native units."""
    if value is None:
        return "—"
    if isinstance(value, int | float) and "memory" in series:
        return f"{value / MIB:.1f} MiB"
    if isinstance(value, float):
        return f"{value:.4g}"
    return str(value)


def bound_cell(check: dict[str, Any]) -> str:
    series = check.get("series", "")
    lower, upper = check.get("lower_bound"), check.get("upper_bound")
    if lower is not None and upper is not None:
        return f"{format_value(lower, series)} – {format_value(upper, series)}"
    if upper is not None:
        return f"≤ {format_value(upper, series)}"
    if lower is not None:
        return f"≥ {format_value(lower, series)}"
    return "—"


def observed_cell(check: dict[str, Any]) -> str:
    """Show the observed value that the bound compares: the max for an upper bound, the min for a lower bound."""
    series = check.get("series", "")
    low, high = check.get("min_observed"), check.get("max_observed")
    if check.get("upper_bound") is not None and check.get("lower_bound") is None:
        return f"max {format_value(high, series)}"
    if check.get("lower_bound") is not None and check.get("upper_bound") is None:
        return f"min {format_value(low, series)}"
    return f"{format_value(low, series)} – {format_value(high, series)}"


def check_table(rows: list[tuple[str, dict[str, Any]]]) -> list[str]:
    lines = [
        "| perf | experiment | check | series | bound | observed | replicates passed | reasons |",
        "| --- | --- | --- | --- | --- | --- | --- | --- |",
    ]
    for experiment, check in rows:
        symbol = "✅" if check.get("passed") else "❌"
        reasons = ", ".join(check.get("reasons", [])) or "—"
        lines.append(
            f"| {symbol} | {experiment} | {check.get('name')} | `{check.get('series')}` "
            f"| {bound_cell(check)} | {observed_cell(check)} "
            f"| {check.get('passed_replicates')}/{check.get('expected_replicates')} | {reasons} |"
        )
    return lines


def details(summary: str, body: list[str]) -> list[str]:
    return ["<details>", f"<summary><h2>{summary}</h2></summary>", "", *body, "", "</details>"]


def failed_replicates(status: Any) -> list[str]:
    rows = []
    for experiment in status.get("experiments", []) if isinstance(status, dict) else []:
        for replicate in experiment.get("replicates", []):
            if replicate.get("state") == "failed":
                message = (replicate.get("error_message") or "—").replace("|", "\\|").replace("\n", " ")
                rows.append(f"| {experiment.get('name')} | {replicate.get('index')} | {message} |")
    return rows


def headline(result_code: str | None, report: Any, state: str | None) -> str:
    # A failed replicate can still give a valid capture with passing checks, so check the job state too.
    if result_code in ("0", "1") and state == "failed":
        return "## ❌ Failed: the KJob job state is `failed`"
    if result_code == "0" and isinstance(report, dict) and report.get("passed") is True:
        return "## ✅ Passed"
    if result_code == "1" and isinstance(report, dict):
        return "## ❌ Failed: one or more bounds checks failed"
    if result_code == "3":
        return "## ⚠️ No result: the KJob report was not ready before the wait limit"
    if result_code is not None:
        return f"## ⚠️ No result: `smp kjob result` failed with exit code {result_code}"
    return "## ⚠️ No result: the CI job stopped before it read the KJob report"


def render(outputs: Path, job_url: str, commit: str, team: str, directory: str) -> str:
    submit = read_json(outputs / "submit.json")
    status = read_json(outputs / "status.json")
    report = read_json(outputs / "report.v1.json")
    result_code = read_text(outputs / "result_code")
    if not (isinstance(report, dict) and result_code in ("0", "1")):
        report = None

    job_id = submit.get("id") if isinstance(submit, dict) else None
    state = status.get("state") if isinstance(status, dict) else None

    lines = ["# KJob Results", "", headline(result_code, report, state), ""]
    lines.append(f"Job ID: `{job_id}`" if job_id else "Job ID: not submitted")
    if job_id:
        lines.append(f"[Target logs]({LOGS_URL.format(query=quote(f'job_id:{job_id}'))})")
    lines += [
        "",
        f"SMP team: `{team}`, job directory: `{directory}`, job state: `{state or 'unknown'}`",
        f"Commit: `{commit}`, CI job: {job_url}",
        "",
        "This job uses the fixed images of the fixture, not the images of this PR.",
    ]

    replicates = failed_replicates(status)
    if replicates:
        lines += [
            "",
            "## ❌ Experiments with failed replicates",
            "",
            "| experiment | replicate | error |",
            "| --- | --- | --- |",
            *replicates,
        ]

    if report is not None:
        checks = [(e.get("name"), c) for e in report.get("experiments", []) for c in e.get("checks", [])]
        failed = [(name, check) for name, check in checks if not check.get("passed")]
        passed = [(name, check) for name, check in checks if check.get("passed")]
        if failed:
            failed_experiments = sorted({name for name, _ in failed})
            lines += ["", "## Bounds Checks: ❌ Failed", ""]
            lines += [f"* {name}" for name in failed_experiments]
            lines += ["", *check_table(failed)]
            if passed:
                lines += ["", *details("Bounds Checks: ✅ Passed", check_table(passed))]
        elif passed:
            lines += ["", *details("Bounds Checks: ✅ Passed", check_table(passed))]

        without_checks = sorted(e.get("name") for e in report.get("experiments", []) if not e.get("checks"))
        if without_checks:
            lines += [
                "",
                *details(
                    "Experiments with no bounds checks",
                    ["These experiments pass when their run completes.", "", *[f"* {name}" for name in without_checks]],
                ),
            ]

    return "\n".join(lines)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--outputs", type=Path, required=True, help="Directory with the outputs of the KJob CI job.")
    parser.add_argument("--commit", required=True, help="Commit SHA that the pr-commenter service uses to find the PR.")
    parser.add_argument("--job-url", required=True, help="URL of the KJob CI job.")
    parser.add_argument("--team", required=True, help="SMP team name.")
    parser.add_argument("--directory", required=True, help="KJob job directory.")
    parser.add_argument("--output", type=Path, required=True, help="Where to write the payload.")
    args = parser.parse_args()

    payload = {
        "org": "DataDog",
        "repo": "datadog-agent",
        "commit": args.commit,
        "header": HEADER,
        "message": render(args.outputs, args.job_url, args.commit, args.team, args.directory),
    }
    args.output.write_text(json.dumps(payload), encoding="utf-8")


if __name__ == "__main__":
    main()
