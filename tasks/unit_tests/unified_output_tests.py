"""Tests for UTOF models, metadata, and report formatter."""

import json
import tempfile
import unittest
from unittest.mock import MagicMock

from tasks.libs.testing.utof import UTOFMetadata, format_report
from tasks.libs.testing.utof.metadata import generate_metadata
from tasks.libs.testing.utof.models import (
    UTOFAttempt,
    UTOFDocument,
    UTOFEnvironmentMetadata,
    UTOFFailure,
    UTOFSummary,
    UTOFTestResult,
)

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_pass_doc() -> UTOFDocument:
    """Build a minimal all-passing UTOFDocument without a converter."""
    return UTOFDocument(
        metadata=UTOFMetadata(test_system="unit"),
        summary=UTOFSummary(total=2, passed=2, failed=0, status="pass"),
        tests=[
            UTOFTestResult(id="a1", name="TestFoo", full_name="TestFoo", package="pkg/foo", type="unit", status="pass"),
            UTOFTestResult(id="b2", name="TestBar", full_name="TestBar", package="pkg/foo", type="unit", status="pass"),
        ],
    )


def _make_fail_doc() -> UTOFDocument:
    """Build a minimal failing UTOFDocument without a converter."""
    return UTOFDocument(
        metadata=UTOFMetadata(test_system="unit"),
        summary=UTOFSummary(total=3, passed=1, failed=1, skipped=1, status="fail"),
        tests=[
            UTOFTestResult(
                id="a1", name="TestFoo", full_name="TestFoo", package="testpackage1", type="unit", status="pass"
            ),
            UTOFTestResult(
                id="b2",
                name="TestBar",
                full_name="TestBar",
                package="testpackage1",
                type="unit",
                status="fail",
                attempts=[
                    UTOFAttempt(
                        attempt=1, status="fail", failure=UTOFFailure(message="expected 1, got 2", type="assertion")
                    )
                ],
            ),
            UTOFTestResult(
                id="c3", name="TestBaz", full_name="TestBaz", package="testpackage1", type="unit", status="skip"
            ),
        ],
    )


# ---------------------------------------------------------------------------
# Model serialization tests
# ---------------------------------------------------------------------------


class TestUTOFModels(unittest.TestCase):
    """Test UTOFDocument construction, serialization, and None stripping."""

    def test_to_dict_contains_required_fields(self):
        d = _make_pass_doc().to_dict()
        self.assertIn("version", d)
        self.assertIn("metadata", d)
        self.assertIn("summary", d)
        self.assertIn("tests", d)

    def test_version(self):
        self.assertEqual(_make_pass_doc().to_dict()["version"], "1.0.0")

    def test_none_values_stripped_on_passing_tests(self):
        d = _make_pass_doc().to_dict()
        for test in d["tests"]:
            self.assertNotIn("failure", test)
            self.assertNotIn("flaky", test)

    def test_failure_in_attempt_when_set(self):
        d = _make_fail_doc().to_dict()
        failing = next(t for t in d["tests"] if t["status"] == "fail")
        self.assertIn("attempts", failing)
        failed_attempt = next(a for a in failing["attempts"] if a["status"] == "fail")
        self.assertIn("failure", failed_attempt)
        self.assertEqual(failed_attempt["failure"]["message"], "expected 1, got 2")
        self.assertEqual(failed_attempt["failure"]["type"], "assertion")

    def test_write_json(self):
        doc = _make_pass_doc()
        with tempfile.NamedTemporaryFile(suffix=".json", delete=False, mode="w") as f:
            path = f.name
        try:
            doc.write_json(path)
            with open(path) as f:
                parsed = json.load(f)
            self.assertEqual(parsed["version"], "1.0.0")
            self.assertEqual(len(parsed["tests"]), 2)
        finally:
            import os

            os.unlink(path)

    def test_valid_json_roundtrip(self):
        doc = _make_fail_doc()
        parsed = json.loads(json.dumps(doc.to_dict()))
        self.assertEqual(parsed["summary"]["total"], 3)
        self.assertEqual(parsed["summary"]["failed"], 1)
        self.assertEqual(parsed["summary"]["status"], "fail")

    def test_attempts_stripped_when_empty(self):
        # An attempt list that is empty should be stripped from the dict
        doc = UTOFDocument(
            metadata=UTOFMetadata(test_system="unit"),
            summary=UTOFSummary(total=1, passed=1, status="pass"),
            tests=[
                UTOFTestResult(
                    id="x", name="TestX", full_name="TestX", package="pkg", type="unit", status="pass", attempts=[]
                ),
            ],
        )
        d = doc.to_dict()
        self.assertNotIn("attempts", d["tests"][0])

    def test_attempts_present_when_non_empty(self):
        doc = UTOFDocument(
            metadata=UTOFMetadata(test_system="unit"),
            summary=UTOFSummary(total=1, passed=1, status="pass"),
            tests=[
                UTOFTestResult(
                    id="x",
                    name="TestX",
                    full_name="TestX",
                    package="pkg",
                    type="unit",
                    status="pass",
                    attempts=[UTOFAttempt(attempt=1, status="pass", duration_seconds=0.1)],
                ),
            ],
        )
        d = doc.to_dict()
        self.assertIn("attempts", d["tests"][0])
        self.assertEqual(len(d["tests"][0]["attempts"]), 1)


# ---------------------------------------------------------------------------
# Metadata tests
# ---------------------------------------------------------------------------


class TestGenerateMetadata(unittest.TestCase):
    """Test metadata generation from environment context."""

    def setUp(self):
        self.ctx = MagicMock()
        result = MagicMock()
        result.ok = True
        result.stdout = ""
        self.ctx.run.return_value = result

    def test_test_system_unit(self):
        self.assertEqual(generate_metadata(self.ctx, "unit").test_system, "unit")

    def test_test_system_e2e(self):
        self.assertEqual(generate_metadata(self.ctx, "e2e").test_system, "e2e")

    def test_test_system_kmt(self):
        self.assertEqual(generate_metadata(self.ctx, "kmt").test_system, "kmt")

    def test_test_system_smp(self):
        self.assertEqual(generate_metadata(self.ctx, "smp").test_system, "smp")

    def test_environment_populated(self):
        meta = generate_metadata(self.ctx, "unit")
        self.assertIsInstance(meta.environment, UTOFEnvironmentMetadata)
        self.assertGreater(len(meta.environment.os), 0)

    def test_timestamp_set(self):
        self.assertGreater(len(generate_metadata(self.ctx, "unit").timestamp), 0)

    def test_flavor_stored(self):
        meta = generate_metadata(self.ctx, "unit", flavor="heroku")
        self.assertEqual(meta.environment.agent_flavor, "heroku")


# ---------------------------------------------------------------------------
# Report formatter tests
# ---------------------------------------------------------------------------


class TestFormatReport(unittest.TestCase):
    """Test the human-readable report formatter with manually-constructed docs."""

    def test_report_header_pass(self):
        report = format_report(_make_pass_doc())
        self.assertIn("PASSED", report)
        self.assertIn("Test Report (unit)", report)

    def test_report_header_fail(self):
        self.assertIn("FAILED", format_report(_make_fail_doc()))

    def test_report_summary_counts(self):
        report = format_report(_make_fail_doc())
        self.assertIn("3 total", report)
        self.assertIn("passed", report)
        self.assertIn("failed", report)
        self.assertIn("skipped", report)

    def test_report_failures_section(self):
        report = format_report(_make_fail_doc())
        self.assertIn("Failures", report)
        self.assertIn("TestBar", report)
        self.assertIn("testpackage1", report)

    def test_report_no_failures_section_when_all_pass(self):
        report = format_report(_make_pass_doc())
        self.assertNotIn("Failures", report)
        self.assertNotIn("Retried", report)

    def test_report_e2e_system_label(self):
        doc = UTOFDocument(
            metadata=UTOFMetadata(test_system="e2e"),
            summary=UTOFSummary(total=1, passed=1, status="pass"),
            tests=[
                UTOFTestResult(
                    id="x1", name="TestSuite", full_name="TestSuite", package="test/new-e2e", type="e2e", status="pass"
                )
            ],
        )
        self.assertIn("Test Report (e2e)", format_report(doc))

    def test_report_direct_failure_shown_even_when_subtests_also_fail(self):
        """A direct failure (e.g. panic) on a parent is rendered even when subtests also fail."""
        parent = UTOFTestResult(
            id="p1",
            name="TestSuite",
            full_name="TestSuite",
            package="pkg/example",
            type="unit",
            status="fail",
            attempts=[
                UTOFAttempt(
                    attempt=1,
                    status="fail",
                    failure=UTOFFailure(message="panic: nil pointer dereference", type="panic", direct=True),
                )
            ],
            subtests=[
                UTOFTestResult(
                    id="s1",
                    name="TestSuite/Sub",
                    full_name="TestSuite/Sub",
                    package="pkg/example",
                    type="unit",
                    status="fail",
                    attempts=[
                        UTOFAttempt(
                            attempt=1,
                            status="fail",
                            failure=UTOFFailure(message="expected 1 got 2", type="assertion", direct=True),
                        )
                    ],
                ),
            ],
        )
        doc = UTOFDocument(
            metadata=UTOFMetadata(test_system="unit"),
            summary=UTOFSummary(total=1, passed=0, failed=1, status="fail"),
            tests=[parent],
        )
        report = format_report(doc)
        self.assertIn("panic: nil pointer dereference", report)
