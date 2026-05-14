"""Generic Go test → UTOFDocument converter.

Format-specific converters (``go.unit``, ``go.e2e``, …) call
``convert_go_test_results`` with their own ``test_type`` and
``custom_extractors``.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

from tasks.libs.testing.result_json import ResultJson
from tasks.libs.testing.utof.go.parser.failure_parser import FailureExtractor
from tasks.libs.testing.utof.go.parser.run_parser import (
    build_attempts,
    build_summary,
    build_test_tree,
    classify_flaky,
    compute_duration,
    compute_retry_count,
    compute_total_duration,
    count_leaves,
    determine_status,
    generate_test_id,
    leaf_name,
    suite_name,
)
from tasks.libs.testing.utof.metadata import generate_metadata
from tasks.libs.testing.utof.models import UTOFDocument, UTOFMetadata, UTOFTestResult

if TYPE_CHECKING:
    from invoke import Context

    from tasks.testwasher import TestWasher


def convert_go_test_results(
    ctx: Context,
    result_json: ResultJson,
    test_type: str,
    test_washer: TestWasher | None = None,
    metadata: UTOFMetadata | None = None,
    custom_extractors: list[FailureExtractor] | None = None,
) -> UTOFDocument:
    """Convert Go test2json results into a UTOFDocument.

    Args:
        result_json: Parsed test2json JSONL output.
        test_type: Value stored on each ``UTOFTestResult.type`` and the
            test_system metadata field (e.g. ``"unit"`` or ``"e2e"``).
        test_washer: Optional TestWasher instance for flaky test analysis.
        metadata: Optional pre-built metadata. If None, a default is generated.
        custom_extractors: Format-specific failure extractors forwarded to
            ``build_attempts`` (e.g. Pulumi for e2e infra errors).

    Returns:
        A UTOFDocument containing all test results.
    """
    if metadata is None:
        metadata = generate_metadata(ctx, test_system=test_type)
    metadata.duration_seconds = compute_total_duration(result_json)

    flaky_failures: dict[str, set[str]] = test_washer.get_flaky_failures() if test_washer else {}

    tests: list[UTOFTestResult] = []

    for package, package_tests in result_json.package_tests_dict.items():
        for test_name, actions in package_tests.items():
            if test_name == "_":
                continue

            status = determine_status(actions)
            duration = compute_duration(actions)
            retry_count = compute_retry_count(actions)
            attempts = build_attempts(actions, custom_extractors=custom_extractors)
            flaky = None
            if test_washer:
                status, flaky = classify_flaky(status, package, test_name, actions, flaky_failures, test_washer)

            tests.append(
                UTOFTestResult(
                    id=generate_test_id(package, test_name),
                    name=leaf_name(test_name),
                    full_name=test_name,
                    package=package,
                    suite=suite_name(test_name),
                    type=test_type,
                    status=status,
                    duration_seconds=round(duration, 6),
                    retry_count=retry_count,
                    flaky=flaky,
                    attempts=attempts,
                )
            )

    by_package: dict[str, list[UTOFTestResult]] = {}
    for t in tests:
        by_package.setdefault(t.package, []).append(t)
    rooted = []
    for pkg_tests in by_package.values():
        rooted.extend(build_test_tree(pkg_tests))

    return UTOFDocument(
        version="1.0.0",
        metadata=metadata,
        summary=build_summary(count_leaves(rooted)),
        tests=rooted,
    )
