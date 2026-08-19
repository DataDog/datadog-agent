"""Human-readable report formatter for a pipeline-wide UTOF aggregate."""

from __future__ import annotations

from tasks.libs.common.color import color_message
from tasks.libs.testing.utof.models import UTOFTestResult
from tasks.libs.testing.utof.pipeline import JobUTOFResult, PipelineUTOFAggregate
from tasks.libs.testing.utof.report import _get_test_failure, _render_failure, _short_package, _status_badge


def _test_line(job: JobUTOFResult, t: UTOFTestResult) -> list[str]:
    out = [
        f"{color_message(f'[{job.job_name}]', 'bold')} {_status_badge(t.status)}  {_short_package(t.package)} :: {t.name}"
    ]
    failure = _get_test_failure(t)
    if failure:
        out.extend(_render_failure(failure, "    "))
    return out


def format_pipeline_report(agg: PipelineUTOFAggregate) -> str:
    """Produce a human-friendly, color-coded overview of every UTOF-emitting
    job's results across an entire pipeline, for triaging pipeline failures
    without opening each job individually.
    """
    lines: list[str] = []
    s = agg.summary

    separator = "=" * 60
    color = "red" if s.status == "fail" else "green"
    title = f"  Pipeline #{agg.pipeline_id} — {'FAILURES FOUND' if s.status == 'fail' else 'ALL PASSED'}  "
    lines.append(color_message(separator, color))
    lines.append(color_message(title, color))
    lines.append(color_message(separator, color))
    lines.append(agg.pipeline_url)
    lines.append("")

    lines.append(
        f"{len(agg.jobs)} job(s) checked, {len({j.job_name for j, _ in agg.failures})} with failures, "
        f"{len(agg.no_data_jobs)} with no test data"
    )
    parts = [color_message(f"{s.total} total", "bold")]
    if s.passed:
        parts.append(color_message(f"{s.passed} passed", "green"))
    if s.failed:
        parts.append(color_message(f"{s.failed} failed", "red"))
    if s.skipped:
        parts.append(color_message(f"{s.skipped} skipped", "grey"))
    if s.flaky:
        parts.append(color_message(f"{s.flaky} flaky", "orange"))
    lines.append("  ".join(parts))
    lines.append("")

    if agg.failures:
        lines.append(color_message(f"--- Failures ({len(agg.failures)}) ---", "red"))
        lines.append("")
        for job, t in sorted(agg.failures, key=lambda jt: (jt[0].job_name, jt[1].package, jt[1].full_name)):
            lines.extend(_test_line(job, t))
        lines.append("")

    if agg.flaky:
        lines.append(color_message(f"--- Flaky ({len(agg.flaky)}) ---", "orange"))
        lines.append("")
        for job, t in sorted(agg.flaky, key=lambda jt: (jt[0].job_name, jt[1].package, jt[1].full_name)):
            lines.extend(_test_line(job, t))
        lines.append("")

    if agg.no_data_jobs:
        lines.append(color_message(f"--- Jobs with no test data ({len(agg.no_data_jobs)}) ---", "grey"))
        lines.append("")
        for job in sorted(agg.no_data_jobs, key=lambda j: j.job_name):
            lines.append(f"[{job.job_name}] {job.job_status} — {job.job_url} ({job.error})")
        lines.append("")

    return "\n".join(lines)
