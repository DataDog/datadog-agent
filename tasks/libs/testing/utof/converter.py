"""Orchestration layer: convert ResultJson + TestWasher into a UTOFDocument."""

from __future__ import annotations

import hashlib
import os
import platform
from datetime import datetime, timezone
from typing import TYPE_CHECKING

from tasks.libs.testing.result_json import ActionType, ResultJson, run_is_failing
from tasks.libs.testing.utof.failure_parser import _extract_failure_info
from tasks.libs.testing.utof.models import (
    UTOFAttempt,
    UTOFCIMetadata,
    UTOFDocument,
    UTOFEnvironmentMetadata,
    UTOFFlaky,
    UTOFGitMetadata,
    UTOFMetadata,
    UTOFSummary,
    UTOFTestResult,
)

if TYPE_CHECKING:
    from tasks.libs.testing.result_json import ResultJsonLine
    from tasks.testwasher import TestWasher


def _generate_test_id(package: str, test_name: str) -> str:
    """Generate a deterministic test ID from package and test name."""
    raw = f"{package}/{test_name}"
    return hashlib.sha256(raw.encode()).hexdigest()[:16]


def _compute_duration(lines: list[ResultJsonLine]) -> float:
    """Compute duration in seconds from first to last action timestamp."""
    if not lines:
        return 0.0

    times = [line.time for line in lines]
    first = min(times)
    last = max(times)
    duration = (last - first).total_seconds()
    return max(0.0, duration)


def _compute_retry_count(lines: list[ResultJsonLine]) -> int:
    """Count the number of retries from the action lines.

    A retry happens whenever a test has more than one terminal action
    (FAIL or PASS). The retry count is the number of extra runs beyond
    the first:
    - FAIL → 0 retries (ran once, failed)
    - FAIL, PASS → 1 retry (failed, retried, passed)
    - FAIL, FAIL → 1 retry (failed, retried, still failed)
    - FAIL, FAIL, PASS → 2 retries
    - FAIL, FAIL, FAIL → 2 retries
    """
    terminal_count = sum(1 for line in lines if line.action in (ActionType.FAIL, ActionType.PASS))
    return max(0, terminal_count - 1)


def _split_into_attempts(lines: list[ResultJsonLine]) -> list[list[ResultJsonLine]]:
    """Split a test's action lines into per-attempt runs.

    Each attempt ends at a terminal action (FAIL or PASS). Lines after
    a terminal FAIL that are followed by more actions form the next attempt.

    Returns a list of line-lists, one per attempt. If there are no retries
    the result is a single-element list.
    """
    terminal_actions = {ActionType.FAIL, ActionType.PASS}
    attempts: list[list[ResultJsonLine]] = []
    current: list[ResultJsonLine] = []

    for line in lines:
        current.append(line)
        if line.action in terminal_actions:
            attempts.append(current)
            current = []

    # Leftover lines without a terminal action (e.g. incomplete output)
    if current:
        if attempts:
            # Append trailing lines to the last attempt
            attempts[-1].extend(current)
        else:
            attempts.append(current)

    return attempts if attempts else [lines]


def _build_attempts(lines: list[ResultJsonLine]) -> list[UTOFAttempt]:
    """Build per-attempt detail from a test's action lines."""
    attempt_groups = _split_into_attempts(lines)
    attempts: list[UTOFAttempt] = []

    for i, attempt_lines in enumerate(attempt_groups, start=1):
        status = _determine_status(attempt_lines)
        duration = _compute_duration(attempt_lines)
        failure = _extract_failure_info(attempt_lines) if status == "fail" else None

        attempts.append(
            UTOFAttempt(
                attempt=i,
                status=status,
                duration_seconds=round(duration, 6),
                failure=failure,
            )
        )

    return attempts


def _determine_status(lines: list[ResultJsonLine]) -> str:
    """Determine test status using same logic as run_is_failing plus skip detection."""
    # Check for skip first
    for line in lines:
        if line.action == ActionType.SKIP:
            return "skip"

    if run_is_failing(lines):
        return "fail"

    # Not failing — either explicitly passed or incomplete output (no terminal action).
    # Align with run_is_failing: if it says "not failing", treat as pass.
    return "pass"


def generate_metadata(test_system: str = "unit", flavor: str = "") -> UTOFMetadata:
    """Generate UTOF metadata from the current environment."""
    git = UTOFGitMetadata(
        branch=os.environ.get("CI_COMMIT_REF_NAME", ""),
        commit_sha=os.environ.get("CI_COMMIT_SHA", ""),
        commit_author=os.environ.get("CI_COMMIT_AUTHOR", ""),
    )
    ci = UTOFCIMetadata(
        pipeline_id=os.environ.get("CI_PIPELINE_ID", ""),
        job_id=os.environ.get("CI_JOB_ID", ""),
        job_name=os.environ.get("CI_JOB_NAME", ""),
        job_url=os.environ.get("CI_JOB_URL", ""),
    )
    env = UTOFEnvironmentMetadata(
        os=platform.system(),
        os_version=platform.release(),
        arch=platform.machine(),
        kernel=platform.version() if platform.system() == "Linux" else "",
        agent_flavor=flavor,
    )

    return UTOFMetadata(
        test_system=test_system,
        timestamp=datetime.now(timezone.utc).isoformat(),
        git=git,
        ci=ci,
        environment=env,
    )


def convert_unit_test_results(
    result_json: ResultJson,
    test_washer: TestWasher | None = None,
    metadata: UTOFMetadata | None = None,
) -> UTOFDocument:
    """Convert unit test results from ResultJson + TestWasher into a UTOFDocument.

    Args:
        result_json: Parsed test2json JSONL output.
        test_washer: Optional TestWasher instance for flaky test analysis.
        metadata: Optional pre-built metadata. If None, a default is generated.

    Returns:
        A UTOFDocument containing all test results.
    """
    if metadata is None:
        metadata = generate_metadata()

    tests: list[UTOFTestResult] = []
    flaky_failures: dict[str, set[str]] = {}
    if test_washer:
        flaky_failures = test_washer.get_flaky_failures()

    # Compute total duration from all lines
    if result_json.lines:
        all_times = [line.time for line in result_json.lines]
        metadata.duration_seconds = (max(all_times) - min(all_times)).total_seconds()

    for package, package_tests in result_json.package_tests_dict.items():
        for test_name, actions in package_tests.items():
            # Skip package-level entries
            if test_name == "_":
                continue

            status = _determine_status(actions)
            duration = _compute_duration(actions)
            retry_count = _compute_retry_count(actions)

            # Build per-attempt detail when retries occurred
            attempts = None
            if retry_count > 0:
                attempts = _build_attempts(actions)

            # Extract failure info.
            # When there are retries, use per-attempt failures to avoid merging
            # assertion blocks from different runs into one confusing blob.
            failure = None
            if attempts:
                if status == "fail":
                    # All retries failed — use the last attempt's failure (final outcome)
                    for attempt in reversed(attempts):
                        if attempt.status == "fail" and attempt.failure:
                            failure = attempt.failure
                            break
                else:
                    # Test passed on retry — surface the first failed attempt's
                    # failure so users can see why it needed a retry
                    for attempt in attempts:
                        if attempt.status == "fail" and attempt.failure:
                            failure = attempt.failure
                            break
            elif status == "fail":
                # No retries — extract from all lines directly
                failure = _extract_failure_info(actions)

            # Determine flaky status
            flaky = None
            if test_washer:
                is_flaky = package in flaky_failures and test_name in flaky_failures[package]
                if is_flaky:
                    # Determine the source of flaky classification
                    source = "washer"
                    for action in actions:
                        if action.output and test_washer.flaky_test_indicator in action.output:
                            source = "marker"
                            break

                    flaky = UTOFFlaky(is_known_flaky=True, source=source)

                    # Adjust status
                    if status == "fail":
                        status = "flaky_fail"
                        # Also extract failure info for flaky failures
                        if failure is None:
                            failure = _extract_failure_info(actions)
                    elif status == "pass":
                        status = "flaky_pass"

            test_result = UTOFTestResult(
                id=_generate_test_id(package, test_name),
                name=test_name,
                package=package,
                suite="",
                type="unit",
                status=status,
                duration_seconds=round(duration, 6),
                retry_count=retry_count,
                failure=failure,
                flaky=flaky,
                attempts=attempts,
            )
            tests.append(test_result)

    # Compute summary
    passed = sum(1 for t in tests if t.status == "pass")
    failed = sum(1 for t in tests if t.status == "fail")
    skipped = sum(1 for t in tests if t.status == "skip")
    flaky_count = sum(1 for t in tests if t.status in ("flaky_pass", "flaky_fail"))
    retried = sum(1 for t in tests if t.retry_count > 0)

    summary = UTOFSummary(
        total=len(tests),
        passed=passed,
        failed=failed,
        skipped=skipped,
        flaky=flaky_count,
        retried=retried,
        status="fail" if failed > 0 else "pass",
    )

    return UTOFDocument(
        version="1.0.0",
        metadata=metadata,
        summary=summary,
        tests=tests,
    )
