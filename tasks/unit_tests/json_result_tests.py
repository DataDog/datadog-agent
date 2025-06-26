import json
import random
from datetime import datetime
from glob import glob
from pathlib import Path
from unittest import TestCase

from tasks.libs.testing.result_json import ActionType, ResultJsonLine, run_is_failing


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
