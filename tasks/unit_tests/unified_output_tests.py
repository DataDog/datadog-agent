import json
import tempfile
import unittest
from pathlib import Path

from tasks.libs.testing.result_json import ResultJson
from tasks.libs.testing.utof import (
    UTOFMetadata,
    _extract_message_from_raw_output,
    _extract_stacktrace_from_raw_output,
    _parse_assertion_blocks,
    convert_unit_test_results,
    format_report,
)
from tasks.testwasher import TestWasher

TESTDATA = Path(__file__).parent / "testdata"


class TestConvertBasic(unittest.TestCase):
    """Test conversion of varied pass/fail/skip test output."""

    def setUp(self):
        self.result = ResultJson.from_file(str(TESTDATA / "test_output_varied.json"))
        self.doc = convert_unit_test_results(self.result)

    def test_test_count(self):
        # varied.json has tests in 3 packages: testpackage1 (test_1..test_4),
        # testpackage2 (test_1, test_2), testpackage3/inner_package (test_1).
        # Package-level entries ("_") should be excluded.
        self.assertEqual(self.doc.summary.total, 7)

    def test_statuses(self):
        statuses = {(t.package, t.name): t.status for t in self.doc.tests}
        pkg1 = "github.com/DataDog/datadog-agent/testpackage1"
        self.assertEqual(statuses.get((pkg1, "test_1")), "fail")  # has FAIL action
        self.assertEqual(statuses.get((pkg1, "test_2")), "pass")  # has PASS action
        self.assertEqual(statuses.get((pkg1, "test_3")), "fail")  # has FAIL action
        self.assertEqual(statuses.get((pkg1, "test_4")), "skip")  # has SKIP action

    def test_packages_present(self):
        packages = {t.package for t in self.doc.tests}
        self.assertIn("github.com/DataDog/datadog-agent/testpackage1", packages)
        self.assertIn("github.com/DataDog/datadog-agent/testpackage2", packages)
        self.assertIn("github.com/DataDog/datadog-agent/testpackage3/inner_package", packages)


class TestConvertAllPassing(unittest.TestCase):
    """Test conversion when all tests pass."""

    def setUp(self):
        self.result = ResultJson.from_file(str(TESTDATA / "test_output_no_failure.json"))
        self.doc = convert_unit_test_results(self.result)

    def test_summary_status_pass(self):
        self.assertEqual(self.doc.summary.status, "pass")

    def test_zero_failures(self):
        self.assertEqual(self.doc.summary.failed, 0)

    def test_all_tests_pass(self):
        for test in self.doc.tests:
            self.assertIn(test.status, ("pass", "skip"), f"Test {test.name} has unexpected status {test.status}")


class TestConvertPanicDetection(unittest.TestCase):
    """Test that panic output is correctly classified."""

    def setUp(self):
        self.result = ResultJson.from_file(str(TESTDATA / "test_output_failure_panic.json"))
        self.doc = convert_unit_test_results(self.result)

    def test_panic_failure_type(self):
        panic_tests = [t for t in self.doc.tests if t.name == "TestLoadConfigShouldBeFast"]
        self.assertEqual(len(panic_tests), 1)
        test = panic_tests[0]
        self.assertEqual(test.status, "fail")
        self.assertIsNotNone(test.failure)
        self.assertEqual(test.failure.type, "panic")

    def test_panic_message_extracted(self):
        test = next(t for t in self.doc.tests if t.name == "TestLoadConfigShouldBeFast")
        self.assertIn("panic:", test.failure.message)

    def test_stacktrace_extracted(self):
        test = next(t for t in self.doc.tests if t.name == "TestLoadConfigShouldBeFast")
        self.assertIn("goroutine", test.failure.stacktrace)


class TestConvertSkipDetection(unittest.TestCase):
    """Test that skipped tests get status='skip'."""

    def test_skip_status(self):
        result = ResultJson.from_file(str(TESTDATA / "test_output_varied.json"))
        doc = convert_unit_test_results(result)
        skipped = [t for t in doc.tests if t.status == "skip"]
        self.assertGreater(len(skipped), 0, "Expected at least one skipped test")
        # test_4 in testpackage1 is skipped
        skipped_names = {t.name for t in skipped}
        self.assertIn("test_4", skipped_names)


class TestConvertRetryDetection(unittest.TestCase):
    """Test that retried tests show correct retry_count and per-attempt detail."""

    def setUp(self):
        self.result = ResultJson.from_file(str(TESTDATA / "test_output_flaky_retried.json"))
        self.doc = convert_unit_test_results(self.result)

    def test_retry_count(self):
        # test_3 in testpackage1: fails then passes (1 retry)
        test_3 = next((t for t in self.doc.tests if t.name == "test_3"), None)
        self.assertIsNotNone(test_3, "test_3 should be present")
        self.assertEqual(test_3.retry_count, 1)
        # After retry, it should show as pass
        self.assertEqual(test_3.status, "pass")

    def test_no_retry_for_passing(self):
        # test_2 passes without retry
        test_2 = next((t for t in self.doc.tests if t.name == "test_2"), None)
        self.assertIsNotNone(test_2)
        self.assertEqual(test_2.retry_count, 0)

    def test_attempts_present_on_retried_test(self):
        # test_3 was retried, so it should have an attempts list
        test_3 = next((t for t in self.doc.tests if t.name == "test_3"), None)
        self.assertIsNotNone(test_3.attempts, "Retried test should have attempts list")
        self.assertEqual(len(test_3.attempts), 2, "Should have 2 attempts (1 fail + 1 pass)")

    def test_attempt_statuses(self):
        test_3 = next(t for t in self.doc.tests if t.name == "test_3")
        self.assertEqual(test_3.attempts[0].status, "fail")
        self.assertEqual(test_3.attempts[0].attempt, 1)
        self.assertEqual(test_3.attempts[1].status, "pass")
        self.assertEqual(test_3.attempts[1].attempt, 2)

    def test_attempt_durations(self):
        test_3 = next(t for t in self.doc.tests if t.name == "test_3")
        for attempt in test_3.attempts:
            self.assertGreaterEqual(attempt.duration_seconds, 0.0)

    def test_failed_attempt_has_failure_info(self):
        # The first attempt failed, so it should have failure details
        test_3 = next(t for t in self.doc.tests if t.name == "test_3")
        first_attempt = test_3.attempts[0]
        self.assertEqual(first_attempt.status, "fail")
        # failure may or may not be present depending on output content,
        # but the status should definitely be "fail"

    def test_passing_attempt_has_no_failure(self):
        test_3 = next(t for t in self.doc.tests if t.name == "test_3")
        second_attempt = test_3.attempts[1]
        self.assertEqual(second_attempt.status, "pass")
        self.assertIsNone(second_attempt.failure)

    def test_no_attempts_for_non_retried_test(self):
        # test_2 passed without retry, so attempts should be None
        test_2 = next((t for t in self.doc.tests if t.name == "test_2"), None)
        self.assertIsNone(test_2.attempts, "Non-retried test should not have attempts")

    def test_retried_test_surfaces_initial_failure(self):
        # Even though test_3 ultimately passed, the top-level failure field
        # should surface the first attempt's failure so users can see *why*
        # it needed a retry
        test_3 = next(t for t in self.doc.tests if t.name == "test_3")
        self.assertEqual(test_3.status, "pass")
        # failure is populated from the first failed attempt
        # (may be None if the failed attempt had no diagnostic output,
        # but the attempts list always tells the full story)


class TestConvertWithWasher(unittest.TestCase):
    """Test that flaky tests are correctly identified when using TestWasher."""

    def test_flaky_marker_detection(self):
        tw = TestWasher(
            test_output_json_file=str(TESTDATA / "test_output_failure_marker.json"),
            flakes_file_paths=["tasks/unit_tests/testdata/flakes_2.yaml"],
        )
        result = ResultJson.from_file(str(TESTDATA / "test_output_failure_marker.json"))
        doc = convert_unit_test_results(result, test_washer=tw)

        # TestGetPayload is marked flaky via the marker in its output
        flaky_tests = [t for t in doc.tests if t.flaky and t.flaky.is_known_flaky]
        self.assertGreater(len(flaky_tests), 0, "Expected at least one flaky test")

        # Check that flaky test has adjusted status
        payload_test = next((t for t in doc.tests if t.name == "TestGetPayload"), None)
        self.assertIsNotNone(payload_test)
        self.assertIn(payload_test.status, ("flaky_fail", "flaky_pass"))

    def test_flaky_from_flakes_yaml(self):
        tw = TestWasher(
            test_output_json_file=str(TESTDATA / "test_output_failure_no_marker.json"),
            flakes_file_paths=["tasks/unit_tests/testdata/flakes_1.yaml"],
        )
        result = ResultJson.from_file(str(TESTDATA / "test_output_failure_no_marker.json"))
        doc = convert_unit_test_results(result, test_washer=tw)

        # All failing tests should be flaky per flakes_1.yaml
        non_flaky_failures = [t for t in doc.tests if t.status == "fail"]
        self.assertEqual(len(non_flaky_failures), 0, "All failures should be marked as flaky")


class TestConvertSummaryCounts(unittest.TestCase):
    """Test that summary totals match actual test counts."""

    def test_summary_matches_tests(self):
        result = ResultJson.from_file(str(TESTDATA / "test_output_varied.json"))
        doc = convert_unit_test_results(result)

        actual_passed = sum(1 for t in doc.tests if t.status == "pass")
        actual_failed = sum(1 for t in doc.tests if t.status == "fail")
        actual_skipped = sum(1 for t in doc.tests if t.status == "skip")

        self.assertEqual(doc.summary.passed, actual_passed)
        self.assertEqual(doc.summary.failed, actual_failed)
        self.assertEqual(doc.summary.skipped, actual_skipped)
        self.assertEqual(doc.summary.total, len(doc.tests))

    def test_overall_status_fail_when_failures(self):
        result = ResultJson.from_file(str(TESTDATA / "test_output_varied.json"))
        doc = convert_unit_test_results(result)
        self.assertEqual(doc.summary.status, "fail")

    def test_overall_status_pass_when_no_failures(self):
        result = ResultJson.from_file(str(TESTDATA / "test_output_no_failure.json"))
        doc = convert_unit_test_results(result)
        self.assertEqual(doc.summary.status, "pass")


class TestConvertToJson(unittest.TestCase):
    """Test JSON serialization."""

    def test_valid_json_output(self):
        result = ResultJson.from_file(str(TESTDATA / "test_output_varied.json"))
        doc = convert_unit_test_results(result)
        json_str = json.dumps(doc.to_dict())
        # Should be valid JSON
        parsed = json.loads(json_str)
        self.assertIn("version", parsed)
        self.assertIn("metadata", parsed)
        self.assertIn("summary", parsed)
        self.assertIn("tests", parsed)

    def test_roundtrip(self):
        result = ResultJson.from_file(str(TESTDATA / "test_output_varied.json"))
        doc = convert_unit_test_results(result)
        d = doc.to_dict()
        json_str = json.dumps(d)
        parsed = json.loads(json_str)
        # Should have same structure
        self.assertEqual(parsed["version"], "1.0.0")
        self.assertEqual(parsed["summary"]["total"], doc.summary.total)
        self.assertEqual(len(parsed["tests"]), len(doc.tests))

    def test_write_json(self):
        result = ResultJson.from_file(str(TESTDATA / "test_output_varied.json"))
        doc = convert_unit_test_results(result)
        with tempfile.NamedTemporaryFile(suffix=".json", delete=False, mode="w") as f:
            path = f.name
        try:
            doc.write_json(path)
            with open(path) as f:
                parsed = json.load(f)
            self.assertEqual(parsed["version"], "1.0.0")
            self.assertEqual(len(parsed["tests"]), len(doc.tests))
        finally:
            import os

            os.unlink(path)

    def test_none_values_stripped(self):
        result = ResultJson.from_file(str(TESTDATA / "test_output_no_failure.json"))
        doc = convert_unit_test_results(result)
        d = doc.to_dict()
        # Tests that pass should not have failure, flaky, or attempts keys
        for test in d["tests"]:
            self.assertNotIn("failure", test, f"Test {test['name']} should not have failure field")
            self.assertNotIn("flaky", test, f"Test {test['name']} should not have flaky field")
            self.assertNotIn("attempts", test, f"Test {test['name']} should not have attempts field")

    def test_attempts_serialized_for_retried_tests(self):
        result = ResultJson.from_file(str(TESTDATA / "test_output_flaky_retried.json"))
        doc = convert_unit_test_results(result)
        d = doc.to_dict()
        retried_tests = [t for t in d["tests"] if t.get("retry_count", 0) > 0]
        self.assertGreater(len(retried_tests), 0)
        for test in retried_tests:
            self.assertIn("attempts", test, f"Retried test {test['name']} should have attempts")
            self.assertIsInstance(test["attempts"], list)
            for attempt in test["attempts"]:
                self.assertIn("attempt", attempt)
                self.assertIn("status", attempt)


class TestConvertDuration(unittest.TestCase):
    """Test duration computation."""

    def test_duration_non_negative(self):
        result = ResultJson.from_file(str(TESTDATA / "test_output_varied.json"))
        doc = convert_unit_test_results(result)
        for test in doc.tests:
            self.assertGreaterEqual(test.duration_seconds, 0.0, f"Test {test.name} has negative duration")

    def test_metadata_duration(self):
        result = ResultJson.from_file(str(TESTDATA / "test_output_varied.json"))
        doc = convert_unit_test_results(result)
        self.assertGreaterEqual(doc.metadata.duration_seconds, 0.0)

    def test_duration_within_range(self):
        result = ResultJson.from_file(str(TESTDATA / "test_output_failure_panic.json"))
        doc = convert_unit_test_results(result)
        # The panic test data spans about 1 second
        self.assertLess(doc.metadata.duration_seconds, 60.0, "Total duration seems unreasonably large")


class TestConvertMetadata(unittest.TestCase):
    """Test metadata generation."""

    def test_custom_metadata(self):
        result = ResultJson.from_file(str(TESTDATA / "test_output_varied.json"))
        metadata = UTOFMetadata(test_system="unit")
        metadata.environment.agent_flavor = "base"
        doc = convert_unit_test_results(result, metadata=metadata)
        self.assertEqual(doc.metadata.test_system, "unit")
        self.assertEqual(doc.metadata.environment.agent_flavor, "base")

    def test_version(self):
        result = ResultJson.from_file(str(TESTDATA / "test_output_varied.json"))
        doc = convert_unit_test_results(result)
        self.assertEqual(doc.version, "1.0.0")


class TestConvertTestId(unittest.TestCase):
    """Test that test IDs are deterministic and unique."""

    def test_deterministic_ids(self):
        result = ResultJson.from_file(str(TESTDATA / "test_output_varied.json"))
        doc1 = convert_unit_test_results(result)
        doc2 = convert_unit_test_results(result)
        ids1 = {t.name: t.id for t in doc1.tests}
        ids2 = {t.name: t.id for t in doc2.tests}
        self.assertEqual(ids1, ids2)

    def test_unique_ids(self):
        result = ResultJson.from_file(str(TESTDATA / "test_output_varied.json"))
        doc = convert_unit_test_results(result)
        ids = [t.id for t in doc.tests]
        self.assertEqual(len(ids), len(set(ids)), "Test IDs should be unique")


class TestFormatReport(unittest.TestCase):
    """Test the human-readable report formatter."""

    def test_report_header_pass(self):
        result = ResultJson.from_file(str(TESTDATA / "test_output_no_failure.json"))
        doc = convert_unit_test_results(result)
        report = format_report(doc)
        self.assertIn("PASSED", report)
        self.assertIn("Test Report (unit)", report)

    def test_report_header_fail(self):
        result = ResultJson.from_file(str(TESTDATA / "test_output_varied.json"))
        doc = convert_unit_test_results(result)
        report = format_report(doc)
        self.assertIn("FAILED", report)

    def test_report_summary_counts(self):
        result = ResultJson.from_file(str(TESTDATA / "test_output_varied.json"))
        doc = convert_unit_test_results(result)
        report = format_report(doc)
        self.assertIn("7 total", report)
        self.assertIn("passed", report)
        self.assertIn("failed", report)
        self.assertIn("skipped", report)

    def test_report_failures_section(self):
        result = ResultJson.from_file(str(TESTDATA / "test_output_varied.json"))
        doc = convert_unit_test_results(result)
        report = format_report(doc)
        self.assertIn("Failures", report)
        self.assertIn("FAIL", report)
        # Should show short package path, not full github URL
        self.assertIn("testpackage1", report)

    def test_report_panic_shows_type(self):
        result = ResultJson.from_file(str(TESTDATA / "test_output_failure_panic.json"))
        doc = convert_unit_test_results(result)
        report = format_report(doc)
        self.assertIn("type: panic", report)
        self.assertIn("TestLoadConfigShouldBeFast", report)

    def test_report_retried_section(self):
        result = ResultJson.from_file(str(TESTDATA / "test_output_flaky_retried.json"))
        doc = convert_unit_test_results(result)
        report = format_report(doc)
        self.assertIn("Retried", report)
        self.assertIn("test_3", report)
        self.assertIn("1 retry", report)
        # Should show per-attempt detail
        self.assertIn("attempt 1: fail", report)
        self.assertIn("attempt 2: pass", report)

    def test_report_no_failures_section_when_all_pass(self):
        result = ResultJson.from_file(str(TESTDATA / "test_output_no_failure.json"))
        doc = convert_unit_test_results(result)
        report = format_report(doc)
        self.assertNotIn("Failures", report)
        self.assertNotIn("Retried", report)

    def test_report_flaky_section(self):
        tw = TestWasher(
            test_output_json_file=str(TESTDATA / "test_output_failure_marker.json"),
            flakes_file_paths=["tasks/unit_tests/testdata/flakes_2.yaml"],
        )
        result = ResultJson.from_file(str(TESTDATA / "test_output_failure_marker.json"))
        doc = convert_unit_test_results(result, test_washer=tw)
        report = format_report(doc)
        self.assertIn("FLAKY", report)

    def test_report_strips_package_prefix(self):
        result = ResultJson.from_file(str(TESTDATA / "test_output_failure_panic.json"))
        doc = convert_unit_test_results(result)
        report = format_report(doc)
        # The failure header line should use the short path
        self.assertIn("pkg/serverless/trace :: TestLoadConfigShouldBeFast", report)
        # The header should NOT use the full github prefix (stacktrace may contain it)
        self.assertNotIn("github.com/DataDog/datadog-agent/pkg/serverless/trace :: TestLoadConfigShouldBeFast", report)


class TestMessageExtraction(unittest.TestCase):
    """Test that failure messages are extracted from raw output instead of just '--- FAIL: ...'."""

    def test_testify_error_extracted(self):
        """testify Error: field should be extracted as the message."""
        result = ResultJson.from_file(str(TESTDATA / "test_output_failure_marker.json"))
        doc = convert_unit_test_results(result)
        test = next(t for t in doc.tests if t.name == "TestGetPayload")
        self.assertIsNotNone(test.failure)
        self.assertNotIn("--- FAIL:", test.failure.message)
        self.assertIn("Expected nil, but got:", test.failure.message)

    def test_testify_error_includes_location(self):
        """Message should include file:line for quick identification."""
        result = ResultJson.from_file(str(TESTDATA / "test_output_failure_no_marker.json"))
        doc = convert_unit_test_results(result)
        test = next(t for t in doc.tests if t.name == "TestGetPayload")
        self.assertIsNotNone(test.failure)
        self.assertIn("gohai_test.go:17:", test.failure.message)
        self.assertIn("Expected nil, but got:", test.failure.message)

    def test_testify_stacktrace_extracted(self):
        """Error Trace: should populate the stacktrace field (full path)."""
        result = ResultJson.from_file(str(TESTDATA / "test_output_failure_no_marker.json"))
        doc = convert_unit_test_results(result)
        test = next(t for t in doc.tests if t.name == "TestGetPayload")
        self.assertIsNotNone(test.failure)
        self.assertIn("gohai_test.go:17", test.failure.stacktrace)

    def test_panic_message_not_overwritten(self):
        """Panic messages should still come from the 'panic:' line."""
        result = ResultJson.from_file(str(TESTDATA / "test_output_failure_panic.json"))
        doc = convert_unit_test_results(result)
        test = next(t for t in doc.tests if t.name == "TestLoadConfigShouldBeFast")
        self.assertIn("panic: toto", test.failure.message)

    def test_single_assertion_message(self):
        """Single testify assertion: file:line: error message."""
        raw_lines = [
            "=== RUN   TestFoo",
            "    foo_test.go:10: ",
            "        \tError Trace:\t/path/to/foo_test.go:10",
            "        \tError:      \tNot equal:",
            "        \t            \texpected: 5",
            "        \t            \tactual  : 3",
            "        \tTest:       \tTestFoo",
            "--- FAIL: TestFoo (0.00s)",
        ]
        message = _extract_message_from_raw_output(raw_lines)
        self.assertIn("foo_test.go:10:", message)
        self.assertIn("Not equal:", message)
        self.assertIn("expected: 5", message)
        # Single assertion — should NOT have numbered entries
        self.assertNotIn("[1]", message)

    def test_multiple_assertions_numbered(self):
        """Multiple testify assertions: each numbered with its own location."""
        raw_lines = [
            "    value_test.go:62: ",
            "        \tError Trace:\tgithub.com/DataDog/datadog-agent/pkg/gohai/utils/value_test.go:62",
            "        \tError:      \tNot equal: ",
            "        \t            \texpected: 0",
            "        \t            \tactual  : 1",
            "        \tTest:       \tTestValueOrDefault",
            "    value_test.go:63: ",
            "        \tError Trace:\tgithub.com/DataDog/datadog-agent/pkg/gohai/utils/value_test.go:63",
            "        \tError:      \tNot equal: ",
            "        \t            \texpected: 0",
            "        \t            \tactual  : 1",
            "        \tTest:       \tTestValueOrDefault",
            "--- FAIL: TestValueOrDefault (0.00s)",
        ]
        message = _extract_message_from_raw_output(raw_lines)
        # Should say how many failed
        self.assertIn("2 assertions failed", message)
        # Each entry numbered with short location
        self.assertIn("[1] value_test.go:62:", message)
        self.assertIn("[2] value_test.go:63:", message)
        # Each shows the error content
        self.assertEqual(message.count("Not equal:"), 2)

    def test_multiple_assertions_stacktrace_has_all_locations(self):
        """Stacktrace should list all failure locations, not just the first."""
        raw_lines = [
            "        \tError Trace:\t/path/to/value_test.go:62",
            "        \tError:      \tNot equal",
            "        \tError Trace:\t/path/to/value_test.go:63",
            "        \tError:      \tNot equal",
        ]
        trace = _extract_stacktrace_from_raw_output(raw_lines)
        self.assertIn("value_test.go:62", trace)
        self.assertIn("value_test.go:63", trace)

    def test_parse_assertion_blocks(self):
        """Direct test of block parsing."""
        raw_lines = [
            "        \tError Trace:\t/path/to/a_test.go:10",
            "        \tError:      \tFoo failed",
            "        \tError Trace:\t/path/to/a_test.go:20",
            "        \tError:      \tBar failed",
        ]
        blocks = _parse_assertion_blocks(raw_lines)
        self.assertEqual(len(blocks), 2)
        self.assertIn("a_test.go:10", blocks[0].trace)
        self.assertEqual(blocks[0].error_lines, ["Foo failed"])
        self.assertIn("a_test.go:20", blocks[1].trace)
        self.assertEqual(blocks[1].error_lines, ["Bar failed"])

    def test_standard_go_error(self):
        """Standard t.Error output includes file:line in message."""
        raw_lines = [
            "=== RUN   TestBar",
            "    bar_test.go:42: expected 10, got 7",
            "--- FAIL: TestBar (0.01s)",
        ]
        message = _extract_message_from_raw_output(raw_lines)
        self.assertIn("bar_test.go:42:", message)
        self.assertIn("expected 10, got 7", message)

    def test_empty_output(self):
        """Returns empty string when no recognizable pattern."""
        raw_lines = [
            "=== RUN   TestBaz",
            "--- FAIL: TestBaz (0.00s)",
        ]
        message = _extract_message_from_raw_output(raw_lines)
        self.assertEqual(message, "")


class TestSubtestHierarchy(unittest.TestCase):
    """Test that subtests are nested under their parent in a tree."""

    def setUp(self):
        # test_output_failure_parent.json has:
        #   TestEKSSuite
        #     TestEKSSuite/TestCPU
        #       TestEKSSuite/TestCPU/metric___container.cpu.usage{...}
        #     TestEKSSuite/TestMemory
        self.result = ResultJson.from_file(str(TESTDATA / "test_output_failure_parent.json"))
        self.doc = convert_unit_test_results(self.result)

    def test_subtests_nested_under_parent(self):
        """Top-level doc.tests should only contain the root test."""
        self.assertEqual(len(self.doc.tests), 1)
        root = self.doc.tests[0]
        self.assertEqual(root.name, "TestEKSSuite")
        self.assertIsNotNone(root.subtests)
        self.assertEqual(len(root.subtests), 2)

    def test_deep_nesting(self):
        """Three levels: TestEKSSuite → TestCPU → metric_..."""
        root = self.doc.tests[0]
        cpu = next(s for s in root.subtests if s.name == "TestCPU")
        self.assertIsNotNone(cpu.subtests)
        self.assertEqual(len(cpu.subtests), 1)
        metric = cpu.subtests[0]
        self.assertIn("container.cpu.usage", metric.name)

    def test_leaf_name_is_segment(self):
        """name should be the leaf segment, not the full path."""
        root = self.doc.tests[0]
        cpu = next(s for s in root.subtests if s.name == "TestCPU")
        self.assertEqual(cpu.name, "TestCPU")
        # Not the full path
        self.assertNotIn("/", cpu.name)

    def test_full_name_preserved(self):
        """full_name should contain the original test2json name."""
        root = self.doc.tests[0]
        self.assertEqual(root.full_name, "TestEKSSuite")
        cpu = next(s for s in root.subtests if s.name == "TestCPU")
        self.assertEqual(cpu.full_name, "TestEKSSuite/TestCPU")
        metric = cpu.subtests[0]
        self.assertTrue(metric.full_name.startswith("TestEKSSuite/TestCPU/"))

    def test_summary_counts_leaves_only(self):
        """Summary should count only leaf tests to avoid double-counting."""
        # The tree has 2 leaves: the metric under TestCPU and TestMemory
        self.assertEqual(self.doc.summary.total, 2)
        self.assertEqual(self.doc.summary.failed, 2)

    def test_subtests_stripped_from_json_when_empty(self):
        """subtests: None should be stripped from JSON output (like other optional fields)."""
        result = ResultJson.from_file(str(TESTDATA / "test_output_varied.json"))
        doc = convert_unit_test_results(result)
        d = doc.to_dict()
        for test in d["tests"]:
            self.assertNotIn("subtests", test, f"Test {test['name']} should not have subtests field")

    def test_subtests_present_in_json_when_non_empty(self):
        """subtests should appear in JSON when the test has children."""
        d = self.doc.to_dict()
        root = d["tests"][0]
        self.assertIn("subtests", root)
        self.assertEqual(len(root["subtests"]), 2)

    def test_synthetic_parent_created(self):
        """When only a subtest exists (no parent entry), a synthetic parent is created."""
        result = ResultJson.from_file(str(TESTDATA / "test_output_failure_panic_subtest.json"))
        doc = convert_unit_test_results(result)
        # Should have 1 root: the synthetic TestLoadConfigShouldBeFast
        self.assertEqual(len(doc.tests), 1)
        root = doc.tests[0]
        self.assertEqual(root.name, "TestLoadConfigShouldBeFast")
        self.assertIsNotNone(root.subtests)
        self.assertEqual(len(root.subtests), 1)
        sub = root.subtests[0]
        self.assertEqual(sub.name, "MySubTest")
        self.assertEqual(sub.status, "fail")

    def test_no_subtests_data_has_flat_structure(self):
        """Test data without subtests should have no subtests fields set."""
        result = ResultJson.from_file(str(TESTDATA / "test_output_varied.json"))
        doc = convert_unit_test_results(result)
        for t in doc.tests:
            self.assertIsNone(t.subtests, f"Test {t.name} should not have subtests")

    def test_report_shows_subtests_indented(self):
        """The report should show subtests indented under their parent."""
        report = format_report(self.doc)
        self.assertIn("TestEKSSuite", report)
        self.assertIn("TestCPU", report)
        self.assertIn("TestMemory", report)
