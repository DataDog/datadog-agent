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
from datetime import UTC, datetime, timedelta
from pathlib import Path
from typing import Any
from urllib.parse import quote

# The pr-commenter service finds the earlier comment through this header, so it must not change between runs.
HEADER = "K8s Regression Detector Results"
LOGS_URL = (
    "https://app.datadoghq.com/logs?query={query}&index=single-machine-performance-target-logs"
    "&messageDisplay=inline&stream_sort=time%2Cdesc&viz=stream&from_ts={from_ms}&to_ts={to_ms}&live=false"
)
# The KJob Run Metrics Explorer notebook has a job_id template variable.
METRICS_URL = (
    "https://app.datadoghq.com/notebook/15648287?tpl_var_job_id={job_id}&from_ts={from_ms}&to_ts={to_ms}&live=false"
)
COMMIT_URL = "https://github.com/DataDog/datadog-agent/commit/{commit}"
MIB = 1024 * 1024
NANO = 1_000_000_000


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
    """Show bytes in MiB for memory series, and CPU time per second in cores. Report values use native units."""
    if value is None:
        return "—"
    if not isinstance(value, int | float):
        return str(value)
    if "memory" in series:
        return f"{value / MIB:,.1f} MiB"
    if "usage_nanocores" in series or ("usage_core_nanoseconds" in series and series.startswith("rate(")):
        return f"{value / NANO:,.3f} cores"
    if abs(value) >= 1000:
        return f"{value:,.0f}"
    return f"{value:.4g}"


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
    series = check.get("series", "")
    expected, passed = check.get("expected_replicates"), check.get("passed_replicates")
    failed = expected - passed if isinstance(expected, int) and isinstance(passed, int) else "?"
    lower, upper = check.get("lower_bound"), check.get("upper_bound")
    low, high = check.get("min_observed"), check.get("max_observed")
    if upper is not None and lower is None:
        relation = f"max {format_value(high, series)} > {format_value(upper, series)}"
    elif lower is not None and upper is None:
        relation = f"min {format_value(low, series)} < {format_value(lower, series)}"
    else:
        relation = f"observed {observed_text(check)}, bound {bound_text(check)}"
    reasons = check.get("reasons") or []
    if "no_data" in reasons:
        relation = "no data"
    return f"- ❌ `{experiment}`: `{check.get('name')}` failed in {failed} of {expected} replicates, {relation}"


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


def failed_runs(status: Any) -> tuple[list[str], list[str], int, int]:
    """Return one terse line for each experiment with failed replicates, the raw errors, and the run counts."""
    lines, errors, failed_count, total_count = [], [], 0, 0
    for experiment in status.get("experiments", []) if isinstance(status, dict) else []:
        replicates = experiment.get("replicates", [])
        failed = [r for r in replicates if r.get("state") == "failed"]
        total_count += len(replicates)
        failed_count += len(failed)
        if not failed:
            continue
        name = experiment.get("name")
        lines.append(f"- ❌ `{name}`: {len(failed)} of {len(replicates)} replicates did not complete")
        for replicate in failed:
            message = (replicate.get("error_message") or "no error message").replace("\n", " ")
            errors.append(f"- `{name}` replicate {replicate.get('index')}: {message}")
    return lines, errors, failed_count, total_count


def bounds_count(checks: list[tuple[str, dict[str, Any]]]) -> str:
    if not checks:
        return "no bounds checks"
    return f"{sum(1 for _, c in checks if c.get('passed'))}/{len(checks)} bounds checks passed"


def status_line(
    result_code: str | None,
    report: Any,
    checks: list[tuple[str, dict[str, Any]]],
    run_counts: tuple[int, int],
    submitted: bool,
) -> str:
    failed_replicates, total_replicates = run_counts
    if result_code in ("0", "1") and isinstance(report, dict):
        details = []
        # A replicate that did not complete matters more than its checks, so name it first.
        if failed_replicates:
            details.append(f"{failed_replicates} of {total_replicates} replicates did not complete")
        if not failed_replicates or any(not c.get("passed") for _, c in checks):
            details.append(bounds_count(checks))
        passed = result_code == "0" and report.get("passed") is True and not failed_replicates
        return f"**{'✅ Passed' if passed else '❌ Failed'}** - {', '.join(details)}"
    if result_code == "3":
        return "**⚠️ No result:** the run did not finish before the time limit."
    if result_code is not None:
        return "**⚠️ No result:** the CI job could not read the result of the run."
    if submitted:
        return "**⚠️ No result:** the CI job stopped before the run finished."
    return "**⚠️ No result:** the CI job failed before the run started."


def time_range(started_at: str | None, now: datetime) -> tuple[int, int]:
    """Fixed time range of the job, so that the links still work later."""
    try:
        start = datetime.fromisoformat(started_at) if started_at else now - timedelta(hours=3)
    except ValueError:
        start = now - timedelta(hours=3)
    # KJob delivers the log archives and captures after the run, so keep one hour after the comment.
    return int((start - timedelta(minutes=30)).timestamp() * 1000), int((now + timedelta(hours=1)).timestamp() * 1000)


def telemetry_line(job_id: str, started_at: str | None, now: datetime) -> str:
    from_ms, to_ms = time_range(started_at, now)
    logs = LOGS_URL.format(query=quote(f"job_id:{job_id}"), from_ms=from_ms, to_ms=to_ms)
    metrics = METRICS_URL.format(job_id=quote(job_id), from_ms=from_ms, to_ms=to_ms)
    return f"**Available Telemetry:** [Target Logs]({logs}) · [Metrics]({metrics})"


def plural(count: int, word: str) -> str:
    return f"{count} {word}" if count == 1 else f"{count} {word}s"


def render(outputs: Path, job_url: str, commit: str, started_at: str | None, now: datetime) -> str:
    submit = read_json(outputs / "submit.json")
    status = read_json(outputs / "status.json")
    report = read_json(outputs / "report.v1.json")
    result_code = read_text(outputs / "result_code")
    if not (isinstance(report, dict) and result_code in ("0", "1")):
        report = None

    job_id = submit.get("id") if isinstance(submit, dict) else None
    experiments = report.get("experiments", []) if report is not None else []
    checks = [(e.get("name"), c) for e in experiments for c in e.get("checks", [])]
    run_lines, run_errors, *run_counts = failed_runs(status)

    lines = [status_line(result_code, report, checks, (run_counts[0], run_counts[1]), job_id is not None)]

    failed = [(name, check) for name, check in checks if not check.get("passed")]
    failure_lines = run_lines + [failed_check_line(name, check) for name, check in failed]
    if failure_lines:
        lines += ["", *failure_lines]
    if run_errors:
        lines += ["", "<details>", "<summary>Run errors</summary>", "", *run_errors, "", "</details>"]

    if report is not None:
        if checks:
            summary = f"Detailed Bounds Checks ({plural(len(experiments), 'experiment')})"
            body = check_table(checks)
            without_checks = sorted(e.get("name") for e in experiments if not e.get("checks"))
            if without_checks:
                body += ["", "Experiments without bounds checks: " + ", ".join(f"`{name}`" for name in without_checks)]
        else:
            summary = f"Detailed Bounds Checks ({plural(len(experiments), 'experiment')})"
            body = ["There are no bounds checks in the experiments run."]
        lines += ["", "<details>", f"<summary>{summary}</summary>", "", *body, "", "</details>"]

    if job_id:
        lines += ["", telemetry_line(job_id, started_at, now)]

    commit_link = f'<a href="{COMMIT_URL.format(commit=commit)}">{commit[:11]}</a>'
    lines += ["", f'<sub><i>Commit {commit_link} · <a href="{job_url}">CI job</a></i></sub>']
    return "\n".join(lines)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--outputs", type=Path, required=True, help="Directory with the outputs of the KJob CI job.")
    parser.add_argument("--commit", required=True, help="Commit SHA that the pr-commenter service uses to find the PR.")
    parser.add_argument("--job-url", required=True, help="URL of the KJob CI job.")
    parser.add_argument("--started-at", help="Start time of the KJob CI job in ISO 8601, such as CI_JOB_STARTED_AT.")
    parser.add_argument("--output", type=Path, required=True, help="Where to write the payload.")
    args = parser.parse_args()

    payload = {
        "org": "DataDog",
        "repo": "datadog-agent",
        "commit": args.commit,
        "header": HEADER,
        "message": render(args.outputs, args.job_url, args.commit, args.started_at, datetime.now(UTC)),
    }
    args.output.write_text(json.dumps(payload), encoding="utf-8")


if __name__ == "__main__":
    main()
