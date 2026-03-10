"""Shared Go test2json output parsing — used by all Go-based UTOF converters."""

from tasks.libs.testing.utof.go_parser.failure_parser import (
    _extract_failure_info,
    _extract_message_from_raw_output,
    _extract_stacktrace_from_raw_output,
    _parse_assertion_blocks,
)
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
    resolve_failure,
    set_total_duration,
    split_into_attempts,
)

__all__ = [
    # failure_parser
    "_extract_failure_info",
    "_extract_message_from_raw_output",
    "_extract_stacktrace_from_raw_output",
    "_parse_assertion_blocks",
    # run_parser
    "build_attempts",
    "build_summary",
    "build_test_tree",
    "classify_flaky",
    "compute_duration",
    "compute_retry_count",
    "count_leaves",
    "determine_status",
    "generate_test_id",
    "leaf_name",
    "resolve_failure",
    "set_total_duration",
    "split_into_attempts",
]
