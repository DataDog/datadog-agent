"""
Unified Test Output Format (UTOF) converter for unit tests.

Converts test2json JSONL output (ResultJson) + TestWasher flaky analysis
into a single self-contained UTOF JSON document.
"""

from __future__ import annotations

import hashlib
import json
import os
import platform
import re
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone
from typing import TYPE_CHECKING

from tasks.libs.testing.result_json import ActionType, ResultJson, run_is_failing

if TYPE_CHECKING:
    from tasks.libs.testing.result_json import ResultJsonLine
    from tasks.testwasher import TestWasher


# --- UTOF Dataclasses ---


@dataclass
class UTOFGitMetadata:
    branch: str = ""
    commit_sha: str = ""
    commit_author: str = ""  # noqa: F841
    base_branch: str = ""
    base_sha: str = ""  # noqa: F841


@dataclass
class UTOFCIMetadata:
    pipeline_id: str = ""
    job_id: str = ""
    job_name: str = ""
    job_url: str = ""


@dataclass
class UTOFEnvironmentMetadata:
    os: str = ""
    os_version: str = ""
    arch: str = ""
    kernel: str = ""
    go_version: str = ""
    agent_flavor: str = ""


@dataclass
class UTOFMetadata:
    test_system: str = "unit"
    timestamp: str = ""
    duration_seconds: float = 0.0
    git: UTOFGitMetadata = field(default_factory=UTOFGitMetadata)
    ci: UTOFCIMetadata = field(default_factory=UTOFCIMetadata)
    environment: UTOFEnvironmentMetadata = field(default_factory=UTOFEnvironmentMetadata)
    build_tags: list[str] = field(default_factory=list)


@dataclass
class UTOFSummary:
    total: int = 0
    passed: int = 0
    failed: int = 0
    skipped: int = 0
    flaky: int = 0
    retried: int = 0  # noqa: F841
    status: str = "pass"


@dataclass
class UTOFFailure:
    message: str = ""
    type: str = ""  # assertion, panic, timeout, build
    stacktrace: str = ""
    raw_output: str = ""  # noqa: F841


@dataclass
class UTOFFlaky:
    is_known_flaky: bool = False
    source: str = ""  # "marker", "washer", "log_pattern"
    pattern: str = ""


@dataclass
class UTOFAttempt:
    """A single execution attempt of a test. Present only when the test was retried."""

    attempt: int = 1
    status: str = "pass"  # pass, fail
    duration_seconds: float = 0.0
    failure: UTOFFailure | None = None


@dataclass
class UTOFTestResult:
    id: str = ""
    name: str = ""
    package: str = ""
    suite: str = ""
    type: str = "unit"
    status: str = "pass"  # pass, fail, skip, flaky_pass, flaky_fail
    duration_seconds: float = 0.0
    retry_count: int = 0
    failure: UTOFFailure | None = None
    flaky: UTOFFlaky | None = None
    attempts: list[UTOFAttempt] | None = None
    tags: list[str] = field(default_factory=list)


@dataclass
class UTOFLink:
    label: str = ""
    url: str = ""


@dataclass
class UTOFDocument:
    version: str = "1.0.0"
    metadata: UTOFMetadata = field(default_factory=UTOFMetadata)
    summary: UTOFSummary = field(default_factory=UTOFSummary)
    tests: list[UTOFTestResult] = field(default_factory=list)
    links: list[UTOFLink] = field(default_factory=list)

    def to_dict(self) -> dict:
        """Convert the document to a dictionary, stripping None values."""
        return _strip_none(asdict(self))

    def write_json(self, path: str) -> None:
        """Write the document as formatted JSON to the given path."""
        with open(path, "w") as f:
            json.dump(self.to_dict(), f, indent=2)


def _strip_none(obj):
    """Recursively remove keys with None values from dicts."""
    if isinstance(obj, dict):
        return {k: _strip_none(v) for k, v in obj.items() if v is not None}
    if isinstance(obj, list):
        return [_strip_none(item) for item in obj]
    return obj


# --- Metadata generation ---


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


# --- Converter ---


def _generate_test_id(package: str, test_name: str) -> str:
    """Generate a deterministic test ID from package and test name."""
    raw = f"{package}/{test_name}"
    return hashlib.sha256(raw.encode()).hexdigest()[:16]


def _extract_failure_info(lines: list[ResultJsonLine]) -> UTOFFailure | None:
    """Extract structured failure information from test output lines."""
    has_failure = False
    failure_type = ""
    message = ""
    stacktrace_lines: list[str] = []
    raw_output_lines: list[str] = []
    in_stacktrace = False

    for line in lines:
        if line.action == ActionType.FAIL:
            has_failure = True
        if line.action == ActionType.BUILD_FAIL:
            has_failure = True
            failure_type = "build"

        if not line.output:
            continue

        output = line.output

        # Detect panic
        if "panic:" in output:
            has_failure = True
            failure_type = "panic"
            message = output.strip()
            in_stacktrace = True
            raw_output_lines.append(output.rstrip("\n"))
            continue

        # Collect stacktrace lines (goroutine headers, file:line patterns)
        if in_stacktrace:
            stripped = output.strip()
            if stripped and (
                stripped.startswith("goroutine ")
                or re.match(r'\t.*:\d+', stripped)
                or stripped.startswith("created by ")
                or re.match(r'.*\.\w+\(', stripped)
            ):
                stacktrace_lines.append(output.rstrip("\n"))
                raw_output_lines.append(output.rstrip("\n"))
                continue
            elif stripped == "":
                # Empty line can be part of the stacktrace
                stacktrace_lines.append("")
                raw_output_lines.append("")
                continue
            else:
                in_stacktrace = False

        # Detect assertion failure from --- FAIL: lines
        if "--- FAIL:" in output and not failure_type:
            has_failure = True
            failure_type = "assertion"
            if not message:
                message = output.strip()
            raw_output_lines.append(output.rstrip("\n"))
            continue

        # Collect output lines that look like test failure output
        if line.action == ActionType.OUTPUT:
            raw_output_lines.append(output.rstrip("\n"))

    if not has_failure:
        return None

    if not failure_type:
        failure_type = "assertion"

    return UTOFFailure(
        message=message,
        type=failure_type,
        stacktrace="\n".join(stacktrace_lines) if stacktrace_lines else "",
        raw_output="\n".join(raw_output_lines) if raw_output_lines else "",
    )


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
    """Count the number of FAIL actions before a final PASS (i.e. retries)."""
    fail_count = 0
    passed = False
    for line in lines:
        if line.action == ActionType.FAIL:
            fail_count += 1
        elif line.action == ActionType.PASS:
            passed = True
            break
    # If it eventually passed after failures, those failures are retries
    if passed and fail_count > 0:
        return fail_count
    return 0


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

            # Extract failure info — for retried tests that ultimately passed,
            # surface the first failed attempt's failure so users can see why it
            # needed a retry.
            failure = None
            if status == "fail":
                failure = _extract_failure_info(actions)
            elif retry_count > 0 and attempts:
                # Test passed on retry — grab failure from the first failed attempt
                for attempt in attempts:
                    if attempt.status == "fail" and attempt.failure:
                        failure = attempt.failure
                        break

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
    flaky = sum(1 for t in tests if t.status in ("flaky_pass", "flaky_fail"))
    retried = sum(1 for t in tests if t.retry_count > 0)

    summary = UTOFSummary(
        total=len(tests),
        passed=passed,
        failed=failed,
        skipped=skipped,
        flaky=flaky,
        retried=retried,
        status="fail" if failed > 0 else "pass",
    )

    return UTOFDocument(
        version="1.0.0",
        metadata=metadata,
        summary=summary,
        tests=tests,
    )
