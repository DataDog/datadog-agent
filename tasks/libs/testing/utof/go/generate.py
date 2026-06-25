"""Generate a UTOF JSON file from Go test output (unit or e2e)."""

from __future__ import annotations

import os
import traceback
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from invoke import Context

    from tasks.libs.testing.utof.models import UTOFDocument
    from tasks.testwasher import TestWasher


def generate_unified_output(
    ctx: Context,
    result_json_path: str,
    test_system: str,
    flavor_name: str,
    tw: TestWasher | None = None,
) -> UTOFDocument | None:
    """Convert Go test results to UTOF, write the file, and print the report.

    Dispatches to the e2e converter when test_system="e2e" so that Pulumi
    infrastructure failures are classified correctly; falls back to the unit
    converter for all other test systems.

    Returns the UTOFDocument, or None if the output could not be generated.
    """
    if not result_json_path or not os.path.exists(result_json_path):
        return None

    try:
        from tasks.libs.common.utils import gitlab_section
        from tasks.libs.testing.result_json import ResultJson
        from tasks.libs.testing.utof import format_report
        from tasks.libs.testing.utof.go.converter import convert_go_test_results
        from tasks.libs.testing.utof.metadata import generate_metadata

        result_json = ResultJson.from_file(result_json_path)
        metadata = generate_metadata(ctx, test_system=test_system, flavor=flavor_name)

        if test_system == "e2e":
            from tasks.libs.testing.utof.go.e2e.extractors import pulumi_extractor

            utof = convert_go_test_results(
                ctx,
                result_json,
                test_type="e2e",
                test_washer=tw,
                metadata=metadata,
                custom_extractors=[pulumi_extractor],
            )
        else:
            utof = convert_go_test_results(ctx, result_json, test_type="unit", test_washer=tw, metadata=metadata)

        utof_path = result_json_path.replace('.json', '_unified.json')
        utof.write_json(utof_path)
        print(f"Unified test output written to {utof_path}")
        with gitlab_section("Unified test report", collapsed=False):
            print(format_report(utof))
        return utof
    except Exception:
        print(f"Warning: Failed to generate unified test output:\n{traceback.format_exc()}")
        return None
