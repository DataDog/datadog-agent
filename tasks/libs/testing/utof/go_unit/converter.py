"""Convert Go unit test results (ResultJson + TestWasher) into a UTOFDocument."""

from __future__ import annotations

from typing import TYPE_CHECKING

from tasks.libs.testing.result_json import ResultJson
from tasks.libs.testing.utof.go_parser.run_parser import (
    build_attempts,
    build_summary,
    build_test_tree,
    classify_flaky,
    compute_duration,
    compute_retry_count,
    count_leaves,
    determine_status,
    generate_test_id,
    leaf_name,
    set_total_duration,
)
from tasks.libs.testing.utof.metadata import generate_metadata
from tasks.libs.testing.utof.models import UTOFDocument, UTOFMetadata, UTOFTestResult

if TYPE_CHECKING:
    from tasks.testwasher import TestWasher

_TEST_TYPE = "unit"


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
        metadata = generate_metadata(test_system=_TEST_TYPE)

    set_total_duration(metadata, result_json)

    flaky_failures: dict[str, set[str]] = test_washer.get_flaky_failures() if test_washer else {}

    tests: list[UTOFTestResult] = []

    for package, package_tests in result_json.package_tests_dict.items():
        for test_name, actions in package_tests.items():
            if test_name == "_":
                continue

            status = determine_status(actions)
            duration = compute_duration(actions)
            retry_count = compute_retry_count(actions)
            attempts = build_attempts(actions)
            flaky = None
            if test_washer:
                status, flaky = classify_flaky(status, package, test_name, actions, flaky_failures, test_washer)

            tests.append(
                UTOFTestResult(
                    id=generate_test_id(package, test_name),
                    name=leaf_name(test_name),
                    full_name=test_name,
                    package=package,
                    suite="",
                    type=_TEST_TYPE,
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
