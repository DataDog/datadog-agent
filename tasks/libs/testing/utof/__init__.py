"""Unified Test Output Format (UTOF) — public API re-exports."""

from tasks.libs.testing.utof.converter import convert_unit_test_results, generate_metadata
from tasks.libs.testing.utof.failure_parser import (
    _extract_message_from_raw_output,
    _extract_stacktrace_from_raw_output,
    _parse_assertion_blocks,
)
from tasks.libs.testing.utof.models import (
    UTOFAttempt,
    UTOFCIMetadata,
    UTOFDocument,
    UTOFEnvironmentMetadata,
    UTOFFlaky,
    UTOFGitMetadata,
    UTOFLink,
    UTOFMetadata,
    UTOFSummary,
    UTOFTestResult,
)
from tasks.libs.testing.utof.report import format_report

__all__ = [
    # converter
    "convert_unit_test_results",
    "generate_metadata",
    # models
    "UTOFAttempt",
    "UTOFCIMetadata",
    "UTOFDocument",
    "UTOFEnvironmentMetadata",
    "UTOFFlaky",
    "UTOFGitMetadata",
    "UTOFLink",
    "UTOFMetadata",
    "UTOFSummary",
    "UTOFTestResult",
    # report
    "format_report",
    # failure_parser (used by tests)
    "_extract_message_from_raw_output",
    "_extract_stacktrace_from_raw_output",
    "_parse_assertion_blocks",
]
