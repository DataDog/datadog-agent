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

    def test_should_mark_parent_flaky(self):
        test_washer = TestWasher(
            test_output_json_file="test_output_failure_parent.json",
            flakes_file_path="tasks/unit-tests/testdata/flakes_2.yaml",
        )
        module_path = "tasks/unit-tests/testdata"
        non_flaky_failing_tests = test_washer.get_non_flaky_failing_tests(module_path)
        self.assertEqual(
            non_flaky_failing_tests,
            {"github.com/DataDog/datadog-agent/test/new-e2e/tests/containers": {"TestEKSSuite/TestMemory"}},
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
        print(merged_flakes)
        self.assertEqual(
            merged_flakes,
            {"nintendo": {"mario", "luigi"}},
        )


class TestGetTestParents(unittest.TestCase):
    def test_get_tests_parents(self):
        test_washer = TestWasher()
        parents = test_washer.get_tests_family(["TestEKSSuite/TestCPU/TestCPUUtilization", "TestKindSuite/TestKind"])
        self.assertEqual(
            parents,
            {
                "TestEKSSuite",
                "TestEKSSuite/TestCPU",
                "TestEKSSuite/TestCPU/TestCPUUtilization",
                "TestKindSuite",
                "TestKindSuite/TestKind",
            },
        )

    def test_get_test_parents_empty(self):
        test_washer = TestWasher()
        parents = test_washer.get_tests_family([])
        self.assertEqual(
            parents,
            set(),
        )


class TestIsKnownFlake(unittest.TestCase):
    def test_known_flake(self):
        test_washer = TestWasher()
        is_known_flaky = test_washer.is_known_flaky_test(
            "TestEKSSuite/mario", {"TestEKSSuite/mario"}, {"TestEKSSuite", "TestEKSSuite/mario"}
        )
        self.assertTrue(is_known_flaky)

    def test_known_flake_parent_failing(self):
        test_washer = TestWasher()
        is_known_flaky = test_washer.is_known_flaky_test(
            "TestEKSSuite", {"TestEKSSuite/mario"}, {"TestEKSSuite", "TestEKSSuite/mario"}
        )
        self.assertTrue(is_known_flaky)

    def test_known_flake_parent_failing_2(self):
        test_washer = TestWasher()
        is_known_flaky = test_washer.is_known_flaky_test(
            "TestEKSSuite/mario",
            {"TestEKSSuite/mario/luigi"},
            {"TestEKSSuite", "TestEKSSuite/mario", "TestEKSSuite/mario/luigi"},
        )
        self.assertTrue(is_known_flaky)

    def test_not_known_flake(self):
        test_washer = TestWasher()
        is_known_flaky = test_washer.is_known_flaky_test(
            "TestEKSSuite/luigi", {"TestEKSSuite/mario"}, {"TestEKSSuite", "TestEKSSuite/mario"}
        )
        self.assertFalse(is_known_flaky)

    def test_not_known_flake_ambiguous_start(self):
        test_washer = TestWasher()
        is_known_flaky = test_washer.is_known_flaky_test(
            "TestEKSSuiteVM/mario", {"TestEKSSuite/mario"}, {"TestEKSSuite"}
        )
        self.assertFalse(is_known_flaky)

    def test_not_known_flake_ambiguous_start_2(self):
        test_washer = TestWasher()
        is_known_flaky = test_washer.is_known_flaky_test(
            "TestEKSSuite/mario", {"TestEKSSuiteVM/mario"}, {"TestEKSSuiteVM"}
        )
        self.assertFalse(is_known_flaky)
