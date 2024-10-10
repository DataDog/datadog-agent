import unittest

from tasks.testwasher import TestWasher


class TestUtils(unittest.TestCase):
    def test_flaky_marked_failing_test(self):
        test_washer_1 = TestWasher(
            test_output_json_file="test_output_failure_marker.json",
            flakes_file_path="tasks/unit_tests/testdata/flakes_2.yaml",
        )
        module_path = "tasks/unit_tests/testdata"
        failing_tests, marked_flaky_tests = test_washer_1.parse_test_results(module_path)
        non_flaky_failing_tests = test_washer_1.get_non_flaky_failing_tests(
            failing_tests=failing_tests, flaky_marked_tests=marked_flaky_tests
        )
        self.assertEqual(non_flaky_failing_tests, {})

    def test_flakes_file_failing_test(self):
        test_washer_2 = TestWasher(
            test_output_json_file="test_output_failure_no_marker.json",
            flakes_file_path="tasks/unit_tests/testdata/flakes_1.yaml",
        )
        module_path = "tasks/unit_tests/testdata"
        failing_tests, marked_flaky_tests = test_washer_2.parse_test_results(module_path)
        non_flaky_failing_tests = test_washer_2.get_non_flaky_failing_tests(
            failing_tests=failing_tests, flaky_marked_tests=marked_flaky_tests
        )
        self.assertEqual(non_flaky_failing_tests, {})

    def test_should_fail_failing_tests(self):
        test_washer_3 = TestWasher(
            test_output_json_file="test_output_failure_no_marker.json",
            flakes_file_path="tasks/unit_tests/testdata/flakes_2.yaml",
        )
        module_path = "tasks/unit_tests/testdata"
        failing_tests, marked_flaky_tests = test_washer_3.parse_test_results(module_path)
        non_flaky_failing_tests = test_washer_3.get_non_flaky_failing_tests(
            failing_tests=failing_tests, flaky_marked_tests=marked_flaky_tests
        )
        self.assertEqual(non_flaky_failing_tests, {"github.com/DataDog/datadog-agent/pkg/gohai": {"TestGetPayload"}})

    def test_should_mark_parent_flaky(self):
        test_washer = TestWasher(
            test_output_json_file="test_output_failure_parent.json",
            flakes_file_path="tasks/unit_tests/testdata/flakes_2.yaml",
        )
        module_path = "tasks/unit_tests/testdata"
        failing_tests, marked_flaky_tests = test_washer.parse_test_results(module_path)
        non_flaky_failing_tests = test_washer.get_non_flaky_failing_tests(
            failing_tests=failing_tests, flaky_marked_tests=marked_flaky_tests
        )
        self.assertEqual(
            non_flaky_failing_tests,
            {"github.com/DataDog/datadog-agent/test/new-e2e/tests/containers": {"TestEKSSuite/TestMemory"}},
        )

    def test_should_not_be_considered_flaky(self):
        test_washer = TestWasher(
            test_output_json_file="test_output_failure_only_parent.json",
            flakes_file_path="tasks/unit_tests/testdata/flakes_3.yaml",
        )
        module_path = "tasks/unit_tests/testdata"
        failing_tests, marked_flaky_tests = test_washer.parse_test_results(module_path)
        non_flaky_failing_tests = test_washer.get_non_flaky_failing_tests(
            failing_tests=failing_tests, flaky_marked_tests=marked_flaky_tests
        )
        self.assertEqual(
            non_flaky_failing_tests,
            {"github.com/DataDog/datadog-agent/test/new-e2e/tests/containers": {"TestEKSSuite"}},
        )


class TestMergeKnownFlakes(unittest.TestCase):
    def test_with_shared_keys(self):
        test_washer = TestWasher()
        marked_flakes = {
            "nintendo": {"mario", "luigi"},
            "sega": {"sonic"},
        }
        test_washer.known_flaky_tests = {
            "nintendo": {"peach"},
            "sony": {"crashbandicoot"},
        }
        merged_flakes = test_washer.merge_known_flakes(marked_flakes)
        self.assertEqual(
            merged_flakes,
            {
                "nintendo": {"mario", "luigi", "peach"},
                "sega": {"sonic"},
                "sony": {"crashbandicoot"},
            },
        )

    def test_no_shared_keys(self):
        test_washer = TestWasher()
        marked_flakes = {
            "nintendo": {"mario", "luigi"},
        }
        test_washer.known_flaky_tests = {
            "sega": {"sonic"},
        }
        merged_flakes = test_washer.merge_known_flakes(marked_flakes)
        self.assertEqual(
            merged_flakes,
            {
                "nintendo": {"mario", "luigi"},
                "sega": {"sonic"},
            },
        )

    def test_empty_marked(self):
        test_washer = TestWasher()
        marked_flakes = {}
        test_washer.known_flaky_tests = {
            "sega": {"sonic"},
        }
        merged_flakes = test_washer.merge_known_flakes(marked_flakes)
        self.assertEqual(
            merged_flakes,
            {"sega": {"sonic"}},
        )

    def test_empty_yaml(self):
        test_washer = TestWasher()
        marked_flakes = {
            "nintendo": {"mario", "luigi"},
        }
        test_washer.known_flaky_tests = {}
        merged_flakes = test_washer.merge_known_flakes(marked_flakes)
        self.assertEqual(
            merged_flakes,
            {"nintendo": {"mario", "luigi"}},
        )
