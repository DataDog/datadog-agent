import unittest

from tasks.testwasher import TestWasher


class TestUtils(unittest.TestCase):
    def test_flaky_marked_failing_test(self):
        test_washer_1 = TestWasher(
            test_output_json_file="test_output_failure_marker.json",
            flakes_file_paths=["tasks/unit_tests/testdata/flakes_2.yaml"],
        )
        module_path = "tasks/unit_tests/testdata"
        non_flaky_failing_tests = test_washer_1.get_non_flaky_failing_tests(module_path)
        self.assertEqual(non_flaky_failing_tests, {})

    def test_flakes_file_failing_test(self):
        test_washer_2 = TestWasher(
            test_output_json_file="test_output_failure_no_marker.json",
            flakes_file_paths=["tasks/unit_tests/testdata/flakes_1.yaml"],
        )
        module_path = "tasks/unit_tests/testdata"
        non_flaky_failing_tests = test_washer_2.get_non_flaky_failing_tests(module_path)
        self.assertEqual(non_flaky_failing_tests, {})

    def test_should_fail_failing_tests(self):
        test_washer_3 = TestWasher(
            test_output_json_file="test_output_failure_no_marker.json",
            flakes_file_paths=["tasks/unit_tests/testdata/flakes_2.yaml"],
        )
        module_path = "tasks/unit_tests/testdata"
        non_flaky_failing_tests = test_washer_3.get_non_flaky_failing_tests(module_path)
        self.assertEqual(non_flaky_failing_tests, {"github.com/DataDog/datadog-agent/pkg/gohai": {"TestGetPayload"}})

    def test_should_mark_parent_flaky(self):
        test_washer = TestWasher(
            test_output_json_file="test_output_failure_parent.json",
            flakes_file_paths=["tasks/unit_tests/testdata/flakes_2.yaml"],
        )
        module_path = "tasks/unit_tests/testdata"
        print("A", test_washer.get_flaky_failures(module_path))
        print("B", test_washer.get_failing_tests(module_path))
        print("C", test_washer.get_non_flaky_failing_tests(module_path))
        non_flaky_failing_tests = test_washer.get_non_flaky_failing_tests(module_path)
        self.assertEqual(
            non_flaky_failing_tests,
            {
                "github.com/DataDog/datadog-agent/test/new-e2e/tests/containers": {
                    "TestEKSSuite/TestMemory",
                    "TestEKSSuite",
                }
            },
        )

    def test_should_not_be_considered_flaky(self):
        test_washer = TestWasher(
            test_output_json_file="test_output_failure_only_parent.json",
            flakes_file_paths=["tasks/unit_tests/testdata/flakes_3.yaml"],
        )
        module_path = "tasks/unit_tests/testdata"
        non_flaky_failing_tests = test_washer.get_non_flaky_failing_tests(module_path)
        self.assertEqual(
            non_flaky_failing_tests,
            {"github.com/DataDog/datadog-agent/test/new-e2e/tests/containers": {"TestEKSSuite"}},
        )

    def test_flaky_panicking_test(self):
        test_washer = TestWasher(
            test_output_json_file="test_output_failure_flaky_panic.json",
            flakes_file_paths=["tasks/unit_tests/testdata/flakes_2.yaml"],
        )
        module_path = "tasks/unit_tests/testdata"
        non_flaky_failing_tests = test_washer.get_non_flaky_failing_tests(module_path)
        self.assertEqual(non_flaky_failing_tests, {})

    def test_non_flaky_panicking_test(self):
        test_washer = TestWasher(
            test_output_json_file="test_output_failure_panic.json",
            flakes_file_paths=["tasks/unit_tests/testdata/flakes_2.yaml"],
        )
        module_path = "tasks/unit_tests/testdata"
        non_flaky_failing_tests = test_washer.get_non_flaky_failing_tests(module_path)
        self.assertEqual(
            non_flaky_failing_tests,
            {'github.com/DataDog/datadog-agent/pkg/serverless/trace': {'TestLoadConfigShouldBeFast'}},
        )

    def test_flaky_panicking_flakesyaml_test(self):
        test_washer = TestWasher(
            test_output_json_file="test_output_failure_panic.json",
            flakes_file_paths=["tasks/unit_tests/testdata/flakes_4.yaml"],
        )
        module_path = "tasks/unit_tests/testdata"
        non_flaky_failing_tests = test_washer.get_non_flaky_failing_tests(module_path)
        self.assertEqual(non_flaky_failing_tests, {})

    def test_flaky_on_log(self):
        test_washer = TestWasher(
            test_output_json_file="test_output_failure_panic.json",
            flakes_file_paths=["tasks/unit_tests/testdata/flakes_5.yaml"],
        )
        module_path = "tasks/unit_tests/testdata"
        flaky_tests = test_washer.get_flaky_failures(module_path)
        self.assertEqual(flaky_tests, {})

    def test_flaky_on_log2(self):
        test_washer = TestWasher(
            test_output_json_file="test_output_failure_panic.json",
            flakes_file_paths=["tasks/unit_tests/testdata/flakes_6.yaml"],
        )
        module_path = "tasks/unit_tests/testdata"
        flaky_tests = test_washer.get_flaky_failures(module_path)
        self.assertEqual(
            flaky_tests,
            {'github.com/DataDog/datadog-agent/pkg/serverless/trace': {'TestLoadConfigShouldBeFast'}},
        )

    def test_flaky_on_log3(self):
        test_washer = TestWasher(
            test_output_json_file="test_output_failure_panic.json",
            flakes_file_paths=["tasks/unit_tests/testdata/flakes_7.yaml"],
        )
        module_path = "tasks/unit_tests/testdata"
        flaky_tests = test_washer.get_flaky_marked_tests(module_path)
        self.assertEqual(
            flaky_tests,
            {'github.com/DataDog/datadog-agent/pkg/serverless/trace': {'TestLoadConfigShouldBeFast'}},
        )

    def test_flaky_on_log4(self):
        test_washer = TestWasher(
            test_output_json_file="test_output_failure_panic.json",
            flakes_file_paths=["tasks/unit_tests/testdata/flakes_8.yaml"],
        )
        module_path = "tasks/unit_tests/testdata"
        flaky_tests = test_washer.get_flaky_marked_tests(module_path)
        self.assertEqual(
            flaky_tests,
            {'github.com/DataDog/datadog-agent/pkg/serverless/trace': {'TestLoadConfigShouldBeFast'}},
        )

    def test_flaky_on_log5(self):
        test_washer = TestWasher(
            test_output_json_file="test_output_failure_panic.json",
            flakes_file_paths=["tasks/unit_tests/testdata/flakes_9.yaml"],
        )
        module_path = "tasks/unit_tests/testdata"
        flaky_tests = test_washer.get_flaky_failures(module_path)
        self.assertEqual(
            flaky_tests,
            {'github.com/DataDog/datadog-agent/pkg/serverless/trace': {'TestLoadConfigShouldBeFast'}},
        )

    def test_flaky_merge(self):
        test_washer = TestWasher(
            test_output_json_file="test_output_failure_no_marker.json",
            flakes_file_paths=["tasks/unit_tests/testdata/flakes_a.yaml", "tasks/unit_tests/testdata/flakes_b.yaml"],
        )
        module_path = "tasks/unit_tests/testdata"
        marked_flaky_tests = test_washer.get_flaky_failures(module_path)
        self.assertEqual(
            marked_flaky_tests,
            {
                'github.com/DataDog/datadog-agent/pkg/gohai/filesystem': {
                    'TestAsJSON',
                    'TestFilterDev/ReplaceDevMountLength',
                }
            },
        )

    def test_flaky_non_failing(self):
        test_washer = TestWasher(
            test_output_json_file="test_output_no_failure.json",
            flakes_file_paths=["tasks/unit_tests/testdata/flakes_1.yaml"],
        )
        module_path = "tasks/unit_tests/testdata"
        marked_flaky_tests = test_washer.get_flaky_marked_tests(module_path)
        self.assertEqual(marked_flaky_tests, {"github.com/DataDog/datadog-agent/pkg/gohai": {"TestGetPayload"}})

    def test_flaky_non_failing_marked(self):
        test_washer = TestWasher(
            test_output_json_file="test_output_no_failure_marker.json",
            flakes_file_paths=["tasks/unit_tests/testdata/flakes_2.yaml"],
        )
        module_path = "tasks/unit_tests/testdata"
        marked_flaky_tests = test_washer.get_flaky_marked_tests(module_path)
        self.assertEqual(
            marked_flaky_tests, {"github.com/DataDog/datadog-agent/pkg/gohai": {"TestGetPayloadContainerized"}}
        )
