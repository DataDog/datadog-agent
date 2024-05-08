import unittest

from tasks.testwasher import TestWasher


class TestUtils(unittest.TestCase):
    def test_flaky_marked_failing_test(self):
        test_washer_1 = TestWasher(
            test_output_json_file="test_output_failure_marker.json",
            flakes_file_path="tasks/unit-tests/testdata/flakes_2.yaml",
        )
        module_path = "tasks/unit-tests/testdata"
        non_flaky_failing_tests = test_washer_1.get_non_flaky_failing_tests(module_path)
        self.assertEqual(non_flaky_failing_tests, {})

    def test_flakes_file_failing_test(self):
        test_washer_2 = TestWasher(
            test_output_json_file="test_output_failure_no_marker.json",
            flakes_file_path="tasks/unit-tests/testdata/flakes_1.yaml",
        )
        module_path = "tasks/unit-tests/testdata"
        non_flaky_failing_tests = test_washer_2.get_non_flaky_failing_tests(module_path)
        self.assertEqual(non_flaky_failing_tests, {})

    def test_should_fail_failing_tests(self):
        test_washer_3 = TestWasher(
            test_output_json_file="test_output_failure_no_marker.json",
            flakes_file_path="tasks/unit-tests/testdata/flakes_2.yaml",
        )
        module_path = "tasks/unit-tests/testdata"
        non_flaky_failing_tests = test_washer_3.get_non_flaky_failing_tests(module_path)
        self.assertEqual(non_flaky_failing_tests, {"github.com/DataDog/datadog-agent/pkg/gohai": {"TestGetPayload"}})
