import json
import random
from datetime import datetime
from glob import glob
from pathlib import Path
from unittest import TestCase

from tasks.libs.testing.result_json import ActionType, ResultJson, ResultJsonLine, merge_result_jsons, run_is_failing


def get_dummy_result_lines(test_name: str, test_package: str, cnt: int) -> list[ResultJsonLine]:
    possible_actions = [ActionType.SKIP, ActionType.PAUSE, ActionType.CONT]
    output = []
    for _ in range(cnt):
        action = random.choice(possible_actions)
        output.append(
            ResultJsonLine(
                time=datetime.now(),
                action=action,
                package=test_package,
                test=test_name,
                output=f"Dummy output for {test_name} with action {action} !",
            )
        )
    return output


class TestJSONResultLine(TestCase):
    test_name = "TestExample"
    test_package = "github.com/DataDog/datadog-agent/test/example"

    def test_parsing(self):
        # Verify that all test json files in the testdata directory can be parsed
        test_files = glob(str(Path(__file__).parent / "testdata" / "test_output_*.json"))
        for file in test_files:
            with open(file) as f:
                for line in f:
                    # Just make sure this does not raise an exception
                    ResultJsonLine.from_dict(json.loads(line))

    def test_run_is_failing_negative(self):
        # Verify that run_is_failing works correctly for a non-failing run
        lines = get_dummy_result_lines(self.test_name, self.test_package, 3)
        lines.append(
            ResultJsonLine(
                time=datetime.now(),
                action=ActionType.PASS,
                package=self.test_package,
                test=self.test_name,
                output="Run completed successfully.",
            )
        )
        result = run_is_failing(lines)
        self.assertFalse(result, "Expected run_is_failing to return False for a non-failing run.")

    def test_run_is_failing_positive(self):
        # Verify that run_is_failing works correctly for a failing run
        lines = get_dummy_result_lines(self.test_name, self.test_package, 3)
        lines.append(
            ResultJsonLine(
                time=datetime.now(),
                action=ActionType.FAIL,
                package=self.test_package,
                test=self.test_name,
                output="Run failed !",
            )
        )
        result = run_is_failing(lines)
        self.assertTrue(result, "Expected run_is_failing to return True for a failing run.")

    def test_run_is_failing_with_retry(self):
        # Verify that run_is_failing works correctly with retries
        lines = get_dummy_result_lines(self.test_name, self.test_package, 2)
        lines.append(
            ResultJsonLine(
                time=datetime.now(),
                action=ActionType.FAIL,
                package=self.test_package,
                test=self.test_name,
                output="Run failed, retrying...",
            )
        )
        lines.extend(get_dummy_result_lines(self.test_name, self.test_package, 2))
        lines.append(
            ResultJsonLine(
                time=datetime.now(),
                action=ActionType.PASS,
                package=self.test_package,
                test=self.test_name,
                output="Retry successful.",
            )
        )
        lines.extend(get_dummy_result_lines(self.test_name, self.test_package, 2))
        result = run_is_failing(lines)
        self.assertFalse(result, "Expected run_is_failing to return False for a run with a successful retry.")

    def test_run_is_failing_with_panic(self):
        # Verify that run_is_failing works correctly with a line not marked as fail but with a panic message
        lines = get_dummy_result_lines(self.test_name, self.test_package, 3)
        lines.append(
            ResultJsonLine(
                time=datetime.now(),
                action=ActionType.PASS,
                package=self.test_package,
                test=self.test_name,
                output="panic: something went wrong!",
            )
        )
        result = run_is_failing(lines)
        self.assertTrue(result, "Expected run_is_failing to return True for a run with a panic.")


class TestJSONResult(TestCase):
    def test_parsing(self):
        # Verify that all test json files in the testdata directory can be parsed
        test_files = glob(str(Path(__file__).parent / "testdata" / "test_output_*.json"))
        for file in test_files:
            # Just make sure this does not raise an exception
            res = ResultJson.from_file(file)
            self.assertGreater(len(res.lines), 0, f"Expected at least one line in {file}")

    def test_package_tests_dict(self):
        # Verify that package_tests_dict is correctly populated
        # This test file was chosen as it
        res = ResultJson.from_file(str(Path(__file__).parent / "testdata" / "test_output_varied.json"))
        package_tests_dict_actions = {
            package: {test: [line.action.value for line in lines] for test, lines in tests.items()}
            for package, tests in res.package_tests_dict.items()
        }

        expected_package_tests_dict_actions = {
            "github.com/DataDog/datadog-agent/testpackage1": {
                "test_1": ["start", "pause", "cont", "run", "fail"],
                "test_2": ["start", "pause", "cont", "pass"],
                "test_3": ["start", "pause", "cont", "fail"],
                "test_4": ["skip"],
                "_": ["start", "run", "output", "pause", "cont", "fail"],
            },
            "github.com/DataDog/datadog-agent/testpackage2": {
                "test_1": ["start", "pause", "cont", "fail"],
                "test_2": ["start", "output", "run", "output", "pass"],
                "_": ["start", "run", "run", "pause", "cont", "output", "fail"],
            },
            "github.com/DataDog/datadog-agent/testpackage3/inner_package": {
                "test_1": ["start", "pause", "cont", "output", "pause", "cont", "pass"]
            },
        }

        self.assertEqual(
            package_tests_dict_actions,
            expected_package_tests_dict_actions,
            "The package_tests_dict does not match the expected structure.",
        )

    def test_failing_tests(self):
        # Verify that failing tests are correctly identified for a few json test files
        files_and_answers = {
            "test_output_varied.json": {
                ("github.com/DataDog/datadog-agent/testpackage1", "test_1"),
                ("github.com/DataDog/datadog-agent/testpackage1", "test_3"),
                ("github.com/DataDog/datadog-agent/testpackage2", "test_1"),
            },
            "test_output_flaky_retried.json": {
                ("github.com/DataDog/datadog-agent/testpackage1", "test_1"),
            },
            "test_output_failure_panic.json": {
                ("github.com/DataDog/datadog-agent/pkg/serverless/trace", "TestLoadConfigShouldBeFast")
            },
            "test_output_failure_only_parent.json": {
                ("github.com/DataDog/datadog-agent/test/new-e2e/tests/containers", "TestEKSSuite"),
            },
            "test_output_no_failure.json": set(),
            "test_output_failure_no_marker.json": {
                ("github.com/DataDog/datadog-agent/pkg/gohai", "TestGetPayload"),
            },
        }
        for file, answers in files_and_answers.items():
            res = ResultJson.from_file(str(Path(__file__).parent / "testdata" / file))
            failing_tests = {(package, test) for package, tests in res.failing_tests.items() for test in tests}
            self.assertEqual(failing_tests, answers, f"The failing tests do not match the expected set for {file}.")

    def test_failing_packages(self):
        # Verify that failing packages are correctly identified for a few json test files
        files_and_answers = {
            "test_output_varied.json": {
                "github.com/DataDog/datadog-agent/testpackage1",
                "github.com/DataDog/datadog-agent/testpackage2",
            },
            "test_output_flaky_retried.json": {"github.com/DataDog/datadog-agent/testpackage1"},
            "test_output_failure_panic.json": {"github.com/DataDog/datadog-agent/pkg/serverless/trace"},
            "test_output_failure_only_parent.json": {
                "github.com/DataDog/datadog-agent/test/new-e2e/tests/containers",
            },
            "test_output_no_failure.json": set(),
            "test_output_failure_no_marker.json": {
                "github.com/DataDog/datadog-agent/pkg/gohai",
            },
        }
        for file, answers in files_and_answers.items():
            res = ResultJson.from_file(str(Path(__file__).parent / "testdata" / file))
            failing_packages = res.failing_packages
            self.assertEqual(
                failing_packages, answers, f"The failing packages do not match the expected set for {file}."
            )

    def test_merge_result_jsons(self):
        # Verify that merging multiple ResultJson objects works correctly
        res1 = ResultJson.from_file(str(Path(__file__).parent / "testdata" / "test_output_varied.json"))
        res2 = ResultJson.from_file(str(Path(__file__).parent / "testdata" / "test_output_flaky_retried.json"))
        res3 = ResultJson.from_file(str(Path(__file__).parent / "testdata" / "test_output_failure_panic.json"))
        merged = merge_result_jsons([res1, res2, res3])

        # Check the failing packages
        expected_failing_packages = {
            "github.com/DataDog/datadog-agent/testpackage1",
            "github.com/DataDog/datadog-agent/testpackage2",
            "github.com/DataDog/datadog-agent/pkg/serverless/trace",
        }
        self.assertEqual(merged.failing_packages, expected_failing_packages)

        # Check the failing tests
        # Note that since testpackage1/test3 has been retried in res2 it should not be considered failing
        expected_failing_tests = {
            "github.com/DataDog/datadog-agent/testpackage1": {"test_1"},
            "github.com/DataDog/datadog-agent/testpackage2": {"test_1"},
            "github.com/DataDog/datadog-agent/pkg/serverless/trace": {"TestLoadConfigShouldBeFast"},
        }
        self.assertEqual(merged.failing_tests, expected_failing_tests)
