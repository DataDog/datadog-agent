#!/usr/bin/env python3
"""
Take a SMP v1 JSON report and
- render the github markdown comment
- compute the job pass/fail decision
- extract intermediary pass/fail hints for CI monitoring

CI job pass/fail
================
Job is to fail if
- any `quality_gate_*` bounds checks fail
- any optimization analysis is missing

Ignored from this criteria
- Detected regressions do not fail the job.
- any `erratic` experiment results
- jobs detected as `is_erratic` at runtime
- bounds checks for non `quality_gate_*` jobs

CI monitoring tags
=========================
- `job`: CI job pass/fail criteria (see above)
- `regression`: fails if any experiment reports a significant regression
- `bounds_checks`: fails if any bound check fails

Exit code
=========
A successful invocation returns zero for both passing and failing policy results.
Input, rendering, and file-write errors return nonzero immediately.
"""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Literal, TypedDict

GATE_PREFIX = "quality_gate_"

LOGS = "logs"
BOUNDS_DASHBOARD = "bounds_dashboard"

ReportData = dict[str, Any]
Verdict = Literal["passed", "failed"]


class ExperimentLinkTemplates(TypedDict):
    """Symbolic link templates for one experiment and its checks."""

    default: tuple[str, ...]
    optimization_goal: tuple[str, ...]
    checks: dict[str, tuple[str, ...]]


class ReportLinks(TypedDict):
    header: tuple[str, ...]
    experiments: dict[str, ExperimentLinkTemplates]
    failed_replicates: tuple[str, ...]


class Extra(TypedDict):
    """Context expected by report_template.md.j2."""

    links: ReportLinks
    tail_section: str


class Decision(TypedDict):
    regressions: Verdict
    all_bounds_checks: Verdict
    job: Verdict


@dataclass(frozen=True)
class DecisionResult:
    decision: Decision
    tail_section: str


def get_links_for_xp(experiment: ReportData) -> ExperimentLinkTemplates:
    """Use the Agent link configuration for every team."""
    name = experiment["name"]
    dashboard = (BOUNDS_DASHBOARD,) if name.startswith(GATE_PREFIX) or name == "python_openmetrics" else ()
    check_names = {check["name"] for check in experiment["bounds_checks"]}
    check_names.update(check["bounds_check_name"] for check in experiment["quantile_checks"])
    return {
        "default": (LOGS,) + dashboard,
        "optimization_goal": (LOGS,) + dashboard,
        "checks": dict.fromkeys(sorted(check_names), dashboard),
    }


def build_extra(report: ReportData, tail: str) -> Extra:
    return {
        "links": {
            "header": ("metrics_dashboard", "profiles"),
            "experiments": {experiment["name"]: get_links_for_xp(experiment) for experiment in report["experiments"]},
            "failed_replicates": ("debug_dashboard",),
        },
        "tail_section": tail,
    }


def render_report(smp: Path, template: Path, report_path: Path, output: Path, extra: Extra) -> str:
    """Render Markdown with SMP and write it directly to the requested path."""
    extra_path = output.parent / "extra.json"
    extra_path.write_text(json.dumps(extra, indent=2), encoding="utf-8")
    command = [
        str(smp.resolve()),
        "report",
        "render",
        "--template-file",
        str(template),
        "--report",
        str(report_path),
        "--extra",
        str(extra_path),
    ]
    result = subprocess.run(command, capture_output=True, text=True, encoding="utf-8", check=False)
    if result.stderr:
        print(result.stderr, file=sys.stderr, end="")
    result.check_returncode()
    output.write_text(result.stdout, encoding="utf-8")
    return result.stdout


def render_comment_payload(commit: str, markdown_text: str, output: Path) -> None:
    payload = {
        "org": "DataDog",
        "repo": "datadog-agent",
        "commit": commit,
        "header": "Regression Detector",
        "message": markdown_text,
    }
    output.write_text(json.dumps(payload), encoding="utf-8")


def render_decision(report: ReportData) -> DecisionResult:
    """Compute the three tag values and the baseline-compatible CI section."""
    regressions_failed = False
    bounds_failed = False
    job_failed = False
    record_lines = []

    for experiment in report["experiments"]:
        is_quality_gate = experiment["name"].startswith(GATE_PREFIX)

        goal = experiment.get("optimization_goal")
        if goal is not None:
            result = goal.get("result")
            if result is None and is_quality_gate:
                job_failed = True

            regressions_failed |= result is None or (
                not experiment["erratic"] and result["is_significant_change"] and not result["is_improvement"]
            )

        for check in experiment["bounds_checks"]:
            total_count = check["total_count"]
            pass_count = check["pass_count"]
            check_failed = pass_count < total_count
            bounds_failed |= not experiment["erratic"] and check_failed

            if is_quality_gate:
                failed_count = total_count - pass_count
                record_base = (
                    f"- **{experiment['name']}**, bounds check **{check['name']}**: "
                    f"{pass_count}/{total_count} replicas passed. "
                )
                if check_failed:
                    record_verdict = f"Failed {failed_count} which is > 0. Gate **FAILED**."
                else:
                    record_verdict = "Gate passed."
                record_lines.append(record_base + record_verdict)
                job_failed |= check_failed

    decision: Decision = {
        "regressions": "failed" if regressions_failed else "passed",
        "all_bounds_checks": "failed" if bounds_failed else "passed",
        "job": "failed" if job_failed else "passed",
    }
    if job_failed:
        summary = "\u274c **Failed.** Some Quality Gates were violated."
    elif record_lines:
        summary = "\u2705 **Passed.** All Quality Gates passed."
    else:
        summary = "\u2705 **Passed.** This suite configures no quality gates."
    tail_section = "\n".join(["## CI Pass/Fail Decision\n", summary, "", *record_lines])
    return DecisionResult(decision, tail_section)


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Render an SMP v1 report and compute the Agent CI decision.")
    parser.add_argument("--smp", type=Path, required=True, help="Path to the SMP executable.")
    parser.add_argument("--commit", required=True, help="Commit SHA consumed by the PR-comment service.")
    parser.add_argument("--template", type=Path, required=True, help="Path to the Jinja report template.")
    parser.add_argument("--report", type=Path, required=True, help="Path to report.v1.json.")
    parser.add_argument("--output-report-md", type=Path, required=True, help="Where to write the Markdown report.")
    parser.add_argument(
        "--output-pr-comment", type=Path, required=True, help="Where to write the PR-comment JSON payload."
    )
    parser.add_argument("--output-decision", type=Path, required=True, help="Where to write the decision JSON.")
    parser.add_argument(
        "--output-failure-mark", type=Path, required=True, help="Marker present when the job decision fails."
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    try:
        report_data: ReportData = json.loads(args.report.read_text(encoding="utf-8"))
        result = render_decision(report_data)
        extra = build_extra(report_data, result.tail_section)
        markdown_text = render_report(args.smp, args.template, args.report, args.output_report_md, extra)
        args.output_decision.write_text(json.dumps(result.decision), encoding="utf-8")
        if result.decision["job"] == "failed":
            args.output_failure_mark.write_bytes(b"")
        else:
            args.output_failure_mark.unlink(missing_ok=True)
        render_comment_payload(args.commit, markdown_text, args.output_pr_comment)
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        print(f"error: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
