from __future__ import annotations

import abc
import os

from tasks.flavor import AgentFlavor
from tasks.libs.civisibility import get_test_link_to_test_on_main
from tasks.libs.common.color import color_message
from tasks.libs.common.gomodules import get_default_modules
from tasks.libs.common.utils import running_in_ci
from tasks.libs.testing.result_json import ResultJson
from tasks.modules import GoModule

DEFAULT_TEST_OUTPUT_JSON = "test_output.json"
DEFAULT_E2E_TEST_OUTPUT_JSON = "e2e_test_output.json"


class ExecResult(abc.ABC):
    def __init__(self, path):
        # The full path of the module
        self.path = path
        # Whether the command failed
        self.failed = False
        # String for representing the result type in printed output
        self.result_type = "generic"

    def failure_string(self, flavor):
        return color_message(f"{self.result_type} failed ({flavor.name} flavor)\n", "red")

    @abc.abstractmethod
    def get_failure(self, flavor):  # noqa: U100
        """
        Return a tuple with two elements:
        * bool value - True if the result is failed, False otherwise
        * str value - human-readable failure representation (if failed), empty string otherwise
        """
        pass


class LintResult(ExecResult):
    def __init__(self, path):
        super().__init__(path)
        self.result_type = "Linters"
        # Results of failed lint calls
        self.lint_outputs = []

    def get_failure(self, flavor):
        failure_string = ""

        if self.failed:
            failure_string = self.failure_string(flavor)
            failure_string += "Linter failures:\n"
            for lint_output in self.lint_outputs:
                if lint_output.exited != 0:
                    failure_string = f"{failure_string}{lint_output.stdout}\n" if lint_output.stdout else failure_string
                    failure_string = f"{failure_string}{lint_output.stderr}\n" if lint_output.stderr else failure_string

        return self.failed, failure_string


class TestResult(ExecResult):
    def __init__(self, path):
        super().__init__(path)
        self.result_type = "Tests"
        # Path to the result.json file output by gotestsum (should always be present)
        self.result_json_path = None
        # List of paths to the junit files output by gotestsum (only present if specified in dda inv test)
        # We have multiple because each retry will produce a new junit file
        self.junit_file_paths = []

    def get_failing_tests(self) -> tuple[set[str], dict[str, set[str]]]:
        obj: ResultJson = ResultJson.from_file(self.result_json_path)
        return obj.failing_packages, obj.failing_tests

    def get_failure(self, flavor):
        failure_string = ""

        if not self.failed:
            return self.failed, failure_string

        if self.result_json_path is None or not os.path.exists(self.result_json_path):
            failure_string += "No result json saved, cannot determine whether tests failed or not."
            return self.failed, failure_string

        failure_string = self.failure_string(flavor)

        failed_packages, failed_tests = self.get_failing_tests()

        if not failed_packages:
            failure_string += "The test command failed, but no test failures detected in the result json."
            return self.failed, failure_string

        failure_string += "Test failures:\n"
        for package in sorted(failed_packages):
            tests = failed_tests.get(package, set())
            if not tests:
                failure_string += f"- {package} package failed due to panic / race condition\n"
            else:
                for name in sorted(tests):
                    failure_string += f"- {package} {name}\n"

                    if running_in_ci():
                        failure_string += f"  See this test name on main in Test Visibility at {get_test_link_to_test_on_main(package, name)}\n"

        return self.failed, failure_string


def process_input_args(
    ctx,
    input_module,
    input_targets,
    input_flavor,
    headless_mode=False,
    only_modified_packages=False,
    build_tags=None,
    lint=False,
):
    """
    Takes the input module, targets and flavor arguments from dda inv test and dda inv coverage.upload-to-codecov,
    sets default values for them & casts them to the expected types.
    """
    if only_modified_packages:
        from tasks import get_modified_packages

        if not build_tags:
            build_tags = []

        modules = get_modified_packages(ctx, build_tags, lint=lint)
    elif isinstance(input_module, str):
        # when this function is called from the command line, targets are passed
        # as comma separated tokens in a string
        if isinstance(input_targets, str):
            modules = [GoModule(input_module, test_targets=input_targets.split(','))]
        else:
            modules = [m for m in get_default_modules().values() if m.path == input_module]
    elif isinstance(input_targets, str):
        modules = [GoModule(".", test_targets=input_targets.split(','))]
    else:
        if not headless_mode:
            print("Using default modules and targets")
        modules = get_default_modules().values()

    flavor = AgentFlavor.base
    if input_flavor:
        flavor = AgentFlavor[input_flavor]

    return modules, flavor


def process_result(flavor: AgentFlavor, result: ExecResult):
    """
    Prints failures in results, and returns False if the result is a failure.
    """

    if result is None:
        return True

    failed, failure_string = result.get_failure(flavor)
    if failed:
        print(failure_string)

    return not failed
