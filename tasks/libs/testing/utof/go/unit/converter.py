"""Convert Go unit test results (ResultJson + TestWasher) into a UTOFDocument."""

from __future__ import annotations

from typing import TYPE_CHECKING

from tasks.libs.testing.result_json import ResultJson
from tasks.libs.testing.utof.go.converter import convert_go_test_results
from tasks.libs.testing.utof.models import UTOFDocument, UTOFMetadata

if TYPE_CHECKING:
    from invoke import Context

    from tasks.testwasher import TestWasher

_TEST_TYPE = "unit"


def convert_unit_test_results(
    ctx: Context,
    result_json: ResultJson,
    test_washer: TestWasher | None = None,
    metadata: UTOFMetadata | None = None,
) -> UTOFDocument:
    """Convert Go unit test results into a UTOFDocument."""
    return convert_go_test_results(
        ctx,
        result_json,
        test_type=_TEST_TYPE,
        test_washer=test_washer,
        metadata=metadata,
    )
