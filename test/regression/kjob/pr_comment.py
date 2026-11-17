"""Write the PR comment payload for the single_machine_performance-kjob CI job.

The job saves its outputs in one directory. This script reads them and writes a
JSON payload for the pr-commenter service. `smp report render` does not support
the KJob report, so this script renders it.

The script always writes a payload, also when the job stopped before the KJob
report was ready, so that the PR shows each run.
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any
from urllib.parse import quote

# The pr-commenter service finds the earlier comment through this header, so it must not change between runs.
HEADER = "K8s Regression Detector Results"
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


def bound_text(check: dict[str, Any]) -> str:
    series = check.get("series", "")
    lower, upper = check.get("lower_bound"), check.get("upper_bound")
    if lower is not None and upper is not None:
        return f"{format_value(lower, series)} – {format_value(upper, series)}"
    if upper is not None:
        return f"≤ {format_value(upper, series)}"
    if lower is not None:
        return f"≥ {format_value(lower, series)}"
    return "—"


def observed_text(check: dict[str, Any]) -> str:
    """Show the value that the bound compares: the max for an upper bound, the min for a lower bound."""
    series = check.get("series", "")
    low, high = check.get("min_observed"), check.get("max_observed")
    if check.get("upper_bound") is not None and check.get("lower_bound") is None:
        return f"max {format_value(high, series)}"
    if check.get("lower_bound") is not None and check.get("upper_bound") is None:
        return f"min {format_value(low, series)}"
    return f"{format_value(low, series)} – {format_value(high, series)}"


def replicates_text(check: dict[str, Any]) -> str:
    return f"{check.get('passed_replicates')}/{check.get('expected_replicates')}"


def failed_check_line(experiment: str, check: dict[str, Any]) -> str:
    expected, passed = check.get("expected_replicates"), check.get("passed_replicates")
    failed = expected - passed if isinstance(expected, int) and isinstance(passed, int) else "?"
    return (
        f"- ❌ `{experiment}`: `{check.get('name')}` failed in {failed} of {expected} replicates, "
        f"{observed_text(check)}, bound {bound_text(check)}"
    )


def check_table(rows: list[tuple[str, dict[str, Any]]]) -> list[str]:
    lines = [
        "| result | experiment | check | bound | observed | replicates passed |",
        "| --- | --- | --- | --- | --- | --- |",
    ]
    for experiment, check in rows:
        symbol = "✅" if check.get("passed") else "❌"
        lines.append(
            f"| {symbol} | {experiment} | {check.get('name')} | {bound_text(check)} "
            f"| {observed_text(check)} | {replicates_text(check)} |"
        )
    return lines


def failed_runs(status: Any) -> tuple[list[str], list[str]]:
    """Return one terse line for each experiment with failed replicates, and the raw errors."""
    lines, errors = [], []
    for experiment in status.get("experiments", []) if isinstance(status, dict) else []:
        replicates = experiment.get("replicates", [])
        failed = [r for r in replicates if r.get("state") == "failed"]
        if not failed:
            continue
        name = experiment.get("name")
        lines.append(f"- ❌ `{name}`: {len(failed)} of {len(replicates)} replicates did not complete")
        for replicate in failed:
            message = (replicate.get("error_message") or "no error message").replace("\n", " ")
            errors.append(f"- `{name}` replicate {replicate.get('index')}: {message}")
    return lines, errors


def status_line(result_code: str | None, report: Any, state: str | None, submitted: bool) -> str:
    if result_code in ("0", "1") and state == "failed":
        return "**❌ Failed**"
    if result_code == "0" and isinstance(report, dict) and report.get("passed") is True:
        return "**✅ Passed**"
    if result_code == "1" and isinstance(report, dict):
        return "**❌ Failed**"
    if result_code == "3":
        return "**⚠️ No result:** the run did not finish before the time limit."
    if result_code is not None:
        return "**⚠️ No result:** the CI job could not read the result of the run."
    if submitted:
        return "**⚠️ No result:** the CI job stopped before the run finished."
    return "**⚠️ No result:** the CI job failed before the run started."


def render(outputs: Path, job_url: str, commit: str) -> str:
    submit = read_json(outputs / "submit.json")
    status = read_json(outputs / "status.json")
    report = read_json(outputs / "report.v1.json")
    result_code = read_text(outputs / "result_code")
    if not (isinstance(report, dict) and result_code in ("0", "1")):
        report = None

    job_id = submit.get("id") if isinstance(submit, dict) else None
    state = status.get("state") if isinstance(status, dict) else None

    lines = [status_line(result_code, report, state, job_id is not None)]

    run_lines, run_errors = failed_runs(status)
    if run_lines:
        lines += ["", *run_lines, "", "<details>", "<summary>Run errors</summary>", "", *run_errors, "", "</details>"]

    if report is not None:
        checks = [(e.get("name"), c) for e in report.get("experiments", []) for c in e.get("checks", [])]
        failed = [(name, check) for name, check in checks if not check.get("passed")]
        if failed:
            lines += ["", *[failed_check_line(name, check) for name, check in failed]]
        if checks:
            lines += [
                "",
                "<details>",
                "<summary>Detailed Bounds Checks</summary>",
                "",
                *check_table(checks),
                "",
                "</details>",
            ]
        else:
            lines += [
                "",
                "<details open>",
                "<summary>Detailed Bounds Checks</summary>",
                "",
                "There are no bounds checks in the experiments run.",
                "",
                "</details>",
            ]

    if job_id:
        logs = LOGS_URL.format(query=quote(f"job_id:{job_id}"))
        lines += ["", "Available Telemetry:", "", f"- [Target Logs]({logs})"]

    lines += ["", f"<sub><i>Commit {commit} · <a href=\"{job_url}\">CI job</a></i></sub>"]
    return "\n".join(lines)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--outputs", type=Path, required=True, help="Directory with the outputs of the KJob CI job.")
    parser.add_argument("--commit", required=True, help="Commit SHA that the pr-commenter service uses to find the PR.")
    parser.add_argument("--job-url", required=True, help="URL of the KJob CI job.")
    parser.add_argument("--output", type=Path, required=True, help="Where to write the payload.")
    args = parser.parse_args()

    payload = {
        "org": "DataDog",
        "repo": "datadog-agent",
        "commit": args.commit,
        "header": HEADER,
        "message": render(args.outputs, args.job_url, args.commit),
    }
    args.output.write_text(json.dumps(payload), encoding="utf-8")


if __name__ == "__main__":
    main()
