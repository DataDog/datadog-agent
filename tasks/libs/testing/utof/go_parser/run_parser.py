"""Shared utilities for converting Go test2json (ResultJson) output into UTOF structures.

These functions are format-agnostic — they work on the raw test2json event stream
and produce UTOFTestResult / UTOFAttempt objects.  Format-specific converters
(go_unit, e2e, …) call these functions and add their own metadata and fields.
"""

from __future__ import annotations

import hashlib
from collections.abc import Callable
from typing import TYPE_CHECKING

from tasks.libs.testing.result_json import ActionType, ResultJson, run_is_failing
from tasks.libs.testing.utof.go_parser.failure_parser import _extract_failure_info
from tasks.libs.testing.utof.models import UTOFAttempt, UTOFFlaky, UTOFSummary, UTOFTestResult

if TYPE_CHECKING:
    from tasks.libs.testing.result_json import ResultJsonLine
    from tasks.testwasher import TestWasher


def leaf_name(full_test_name: str) -> str:
    """Return the leaf segment of a hierarchical test name.

    E.g. "TestSketch/useStore=true/empty_flush" → "empty_flush"
    """
    idx = full_test_name.rfind("/")
    return full_test_name[idx + 1 :] if idx >= 0 else full_test_name


def generate_test_id(package: str, test_name: str) -> str:
    """Generate a deterministic test ID from package and test name."""
    raw = f"{package}/{test_name}"
    return hashlib.sha256(raw.encode()).hexdigest()[:16]


def compute_duration(lines: list[ResultJsonLine]) -> float:
    """Compute duration in seconds from first to last action timestamp."""
    if not lines:
        return 0.0
    times = [line.time for line in lines]
    return max(0.0, (max(times) - min(times)).total_seconds())


def compute_retry_count(lines: list[ResultJsonLine]) -> int:
    """Count the number of retries from the action lines.

    A retry happens whenever a test has more than one terminal action
    (FAIL or PASS). The retry count is the number of extra runs beyond the first:
    - FAIL           → 0 retries
    - FAIL, PASS     → 1 retry
    - FAIL, FAIL     → 1 retry
    - FAIL, FAIL, PASS → 2 retries
    """
    terminal_count = sum(1 for line in lines if line.action in (ActionType.FAIL, ActionType.PASS))
    return max(0, terminal_count - 1)


def split_into_attempts(lines: list[ResultJsonLine]) -> list[list[ResultJsonLine]]:
    """Split a test's action lines into per-attempt runs.

    Each attempt ends at a terminal action (FAIL or PASS).
    Returns a list of line-lists, one per attempt.
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
            attempts[-1].extend(current)
        else:
            attempts.append(current)

    return attempts if attempts else [lines]


def build_attempts(lines: list[ResultJsonLine]) -> list[UTOFAttempt]:
    """Build per-attempt detail from a test's action lines."""
    attempts: list[UTOFAttempt] = []
    for i, attempt_lines in enumerate(split_into_attempts(lines), start=1):
        status = determine_status(attempt_lines)
        failure = _extract_failure_info(attempt_lines) if status == "fail" else None
        attempts.append(
            UTOFAttempt(
                attempt=i,
                status=status,
                duration_seconds=round(compute_duration(attempt_lines), 6),
                failure=failure,
            )
        )
    return attempts


def determine_status(lines: list[ResultJsonLine]) -> str:
    """Determine test status: skip > fail > pass."""
    for line in lines:
        if line.action == ActionType.SKIP:
            return "skip"
    if run_is_failing(lines):
        return "fail"
    return "pass"


def build_test_tree(
    flat_results: list[UTOFTestResult],
    suite_fn: Callable[[str], str] = lambda _: "",
) -> list[UTOFTestResult]:
    """Organize flat test results into a tree based on '/' hierarchy.

    Returns only root-level tests; subtests are nested inside via the
    ``subtests`` field.  When a parent entry is missing from the results,
    a synthetic node is created using ``suite_fn`` to derive its suite field.
    Handles arbitrary nesting depth.
    """
    by_full_name: dict[str, UTOFTestResult] = {r.full_name: r for r in flat_results}
    attached: set[str] = set()

    def _get_or_create(full_name: str, ref_result: UTOFTestResult) -> UTOFTestResult:
        """Return an existing node or create a synthetic one."""
        existing = by_full_name.get(full_name)
        if existing is not None:
            return existing
        synthetic = UTOFTestResult(
            id=generate_test_id(ref_result.package, full_name),
            name=leaf_name(full_name),
            full_name=full_name,
            package=ref_result.package,
            suite=suite_fn(full_name),
            type=ref_result.type,
            status="pass",
        )
        by_full_name[full_name] = synthetic
        return synthetic

    # Attach each result to its parent, creating synthetic ancestors as needed.
    # Process all names (including newly created synthetics) by iterating until
    # no new attachments are made.
    to_attach = [r.full_name for r in flat_results]
    while to_attach:
        next_round: list[str] = []
        for full in to_attach:
            idx = full.rfind("/")
            if idx < 0:
                continue  # root node
            parent_full = full[:idx]
            parent = _get_or_create(parent_full, by_full_name[full])
            if parent.subtests is None:
                parent.subtests = []
            child = by_full_name[full]
            if child not in parent.subtests:
                parent.subtests.append(child)
            attached.add(full)
            # If the parent was just created, it also needs attaching
            if parent_full not in attached and "/" in parent_full:
                next_round.append(parent_full)
        to_attach = next_round

    return [r for r in by_full_name.values() if r.full_name not in attached]


def count_leaves(tests: list[UTOFTestResult]) -> dict[str, int]:
    """Count leaf tests (those without subtests) by status category.

    Returns a dict with keys: passed, failed, skipped, flaky, retried, total.
    """
    counts: dict[str, int] = {"passed": 0, "failed": 0, "skipped": 0, "flaky": 0, "retried": 0, "total": 0}

    def _walk(t: UTOFTestResult):
        if t.subtests:
            for sub in t.subtests:
                _walk(sub)
        else:
            counts["total"] += 1
            if t.status in ("pass", "flaky_pass"):
                counts["passed"] += 1
            elif t.status in ("fail", "flaky_fail"):
                counts["failed"] += 1
            elif t.status == "skip":
                counts["skipped"] += 1
            if t.status in ("flaky_pass", "flaky_fail"):
                counts["flaky"] += 1
            if t.retry_count > 0:
                counts["retried"] += 1

    for test in tests:
        _walk(test)
    return counts


def set_total_duration(metadata, result_json: ResultJson) -> None:
    """Set metadata.duration_seconds from the full span of the result lines."""
    if result_json.lines:
        all_times = [line.time for line in result_json.lines]
        metadata.duration_seconds = (max(all_times) - min(all_times)).total_seconds()


def build_summary(counts: dict[str, int]) -> UTOFSummary:
    """Build a UTOFSummary from a counts dict produced by count_leaves()."""
    return UTOFSummary(
        total=counts["total"],
        passed=counts["passed"],
        failed=counts["failed"],
        skipped=counts["skipped"],
        flaky=counts["flaky"],
        retried=counts["retried"],
        status="fail" if counts["failed"] > 0 else "pass",
    )


def classify_flaky(
    status: str,
    package: str,
    test_name: str,
    actions: list[ResultJsonLine],
    flaky_failures: dict[str, set[str]],
    test_washer: TestWasher,
) -> tuple[str, UTOFFlaky | None]:
    """Apply TestWasher flaky classification to a test result.

    Returns the (possibly updated) status and a UTOFFlaky instance, or None
    if the test is not known to be flaky.
    """
    if package not in flaky_failures or test_name not in flaky_failures[package]:
        return status, None

    source = "washer"
    for action in actions:
        if action.output and test_washer.flaky_test_indicator in action.output:
            source = "marker"
            break

    flaky = UTOFFlaky(is_known_flaky=True, source=source)

    if status == "fail":
        status = "flaky_fail"
    elif status == "pass":
        status = "flaky_pass"

    return status, flaky
