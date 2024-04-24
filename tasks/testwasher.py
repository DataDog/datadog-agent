from collections import defaultdict
import json
import yaml
from typing import List
from tasks.test_core import ModuleTestResult


class TestWasher:
    test_output_json_file = "module_test_output.json"
    flaky_test_indicator = "flakytest: this is a known flaky test"
    flakes_file_path = "flakes.yaml"

    known_flaky_tests = defaultdict(set)

    def __init__(self):
        self.parse_flaky_file()

    def get_non_flaky_failing_tests(self, module_path):
        """
        Parse the test output json file and compute the failing tests and the one known flaky
        """
        failing_tests = defaultdict(set)
        flaky_marked_tests = defaultdict(set)
        with open(f"{module_path}/{self.test_output_json_file}") as f:
            for line in f:
                test_result = json.loads(line)
                # TODO: tests can be retried we should handle that
                if test_result["Action"] == "fail" and "Test" in test_result:
                    failing_tests[test_result["Package"]].add(test_result["Test"])
                if (
                    "Output" in test_result
                    and "Test" in test_result
                    and self.flaky_test_indicator in test_result["Output"]
                ):
                    flaky_marked_tests[test_result["Package"]].add(test_result["Test"])

        non_flaky_failing_tests = defaultdict(set)
        for package, tests in failing_tests.items():
            non_flaky_failing_tests_in_package = set()
            for failing_test in tests:
                if all(
                    not failing_test.startswith(flaky_marked_test) for flaky_marked_test in flaky_marked_tests[package]
                ) and all(
                    not failing_test.startswith(flaky_marked_test)
                    for flaky_marked_test in self.known_flaky_tests[package]
                ):
                    non_flaky_failing_tests_in_package.add(failing_test)
            if non_flaky_failing_tests_in_package:
                non_flaky_failing_tests[package] = non_flaky_failing_tests_in_package
        return non_flaky_failing_tests

    def parse_flaky_file(self):
        """
        Parse the flakes.yaml file and add the tests listed there to the kown flaky tests list
        """
        with open(self.flakes_file_path) as f:
            flakes = yaml.safe_load(f)
            if not flakes:
                return
            for package, tests in flakes.items():
                self.known_flaky_tests[f"github.com/DataDog/datadog-agent/{package}"] = set(tests)

    def process_module_results(self, module_results: List[ModuleTestResult]):
        """
        Process the module test results and decide whether we should succeed or not.
        If only known flaky tests are failing, we should succeed.
        If failing, displays the failing tests that are not known to be flaky
        """

        should_succeed = True
        failed_tests_string = ""
        for module_result in module_results:
            non_flaky_failing_tests = self.get_non_flaky_failing_tests(module_result.path)
            if non_flaky_failing_tests:
                should_succeed = False
                for package, tests in non_flaky_failing_tests.items():
                    for test in tests:
                        failed_tests_string += f"- {package} {test}\n"
        print("The test command failed, the following tests failed and are not supposed to be flaky:")
        print(failed_tests_string)

        return should_succeed
