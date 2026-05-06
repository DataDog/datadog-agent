"""Convert Go e2e test results into a UTOFDocument.

Reuses the unit converter and overrides the test type, suite extractor, and
failure extractors so Pulumi infrastructure errors are surfaced.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

from tasks.libs.testing.result_json import ResultJson
from tasks.libs.testing.utof.go.e2e.extractors import pulumi_extractor
from tasks.libs.testing.utof.go.unit.converter import convert_unit_test_results
from tasks.libs.testing.utof.models import UTOFDocument, UTOFMetadata

if TYPE_CHECKING:
    from invoke import Context

    from tasks.testwasher import TestWasher

_TEST_TYPE = "e2e"
_E2E_EXTRACTORS = [pulumi_extractor]


def _suite_name(full_test_name: str) -> str:
    """Return the top-level suite name from a hierarchical test name.

    E.g. "TestWindowsTestSuite/TestAPIKeyRefresh" → "TestWindowsTestSuite"
         "TestWindowsTestSuite"                   → ""
    """
    idx = full_test_name.find("/")
    return full_test_name[:idx] if idx >= 0 else ""


def convert_e2e_test_results(
    ctx: Context,
    result_json: ResultJson,
    test_washer: TestWasher | None = None,
    metadata: UTOFMetadata | None = None,
) -> UTOFDocument:
    """Convert e2e test results from test2json output into a UTOFDocument."""
    return convert_unit_test_results(
        ctx,
        result_json,
        test_washer=test_washer,
        metadata=metadata,
        test_type=_TEST_TYPE,
        suite_fn=_suite_name,
        custom_extractors=_E2E_EXTRACTORS,
    )
