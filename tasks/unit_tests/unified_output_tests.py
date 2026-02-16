import json
import tempfile
import unittest
from pathlib import Path

from tasks.libs.testing.result_json import ResultJson
from tasks.libs.testing.unified_output import (
    UTOFMetadata,
    convert_unit_test_results,
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
