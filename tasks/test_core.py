import abc
import json
import os
from collections import defaultdict
from typing import Dict, List

from tasks.flavor import AgentFlavor
from tasks.libs.common.color import color_message
from tasks.modules import DEFAULT_MODULES, GoModule


class ModuleResult(abc.ABC):
    def __init__(self, path):
        # The full path of the module
        self.path = path
        # Whether the command failed for that module
        self.failed = False
        # String for representing the result type in printed output
        self.result_type = "generic"

    def failure_string(self, flavor):
        return color_message(f"{self.result_type} for module {self.path} failed ({flavor.name} flavor)\n", "red")

    @abc.abstractmethod
    def get_failure(self, flavor):  # noqa: U100
        """
        Return a tuple with two elements:
        * bool value - True if the result is failed, False otherwise
        * str value - human-readable failure representation (if failed), empty string otherwise
        """
        pass


class ModuleLintResult(ModuleResult):
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


class ModuleTestResult(ModuleResult):
    def __init__(self, path):
        super().__init__(path)
        self.result_type = "Tests"
        # Path to the result.json file output by gotestsum (should always be present)
        self.result_json_path = None
        # Path to the junit file output by gotestsum (only present if specified in inv test)
        self.junit_file_path = None

    def get_failure(self, flavor):
        failure_string = ""

        if self.failed:
            failure_string = self.failure_string(flavor)
            failed_packages = set()
            failed_tests = defaultdict(set)

            # TODO(AP-1959): this logic is now repreated, with some variations, in three places:
            # here, in system-probe.py, and in libs/pipeline_notifications.py
            # We should have some common result.json parsing lib.
            if self.result_json_path is not None and os.path.exists(self.result_json_path):
                with open(self.result_json_path, encoding="utf-8") as tf:
                    for line in tf:
                        json_test = json.loads(line.strip())
                        # This logic assumes that the lines in result.json are "in order", i.e. that retries
                        # are logged after the initial test run.

                        # The line is a "Package" line, but not a "Test" line.
                        # We take these into account, because in some cases (panics, race conditions),
                        # individual test failures are not reported, only a package-level failure is.
                        if 'Package' in json_test and 'Test' not in json_test:
                            package = json_test['Package']
                            action = json_test["Action"]

                            if action == "fail":
                                failed_packages.add(package)
                            elif action == "pass" and package in failed_tests:
                                # The package was retried and fully succeeded, removing from the list of packages to report
                                failed_packages.remove(package)

                        # The line is a "Test" line.
                        elif 'Package' in json_test and 'Test' in json_test:
                            name = json_test['Test']
                            package = json_test['Package']
                            action = json_test["Action"]
                            if action == "fail":
                                failed_tests[package].add(name)
                            elif action == "pass" and name in failed_tests.get(package, set()):
                                # The test was retried and succeeded, removing from the list of tests to report
                                failed_tests[package].remove(name)

            if failed_packages:
                failure_string += "Test failures:\n"
                for package in sorted(failed_packages):
                    tests = failed_tests.get(package, set())
                    if not tests:
                        failure_string += f"- {package} package failed due to panic / race condition\n"
                    else:
                        for name in sorted(tests):
                            failure_string += f"- {package} {name}\n"
            else:
                failure_string += "The test command failed, but no test failures detected in the result json."

        return self.failed, failure_string


def test_core(
    modules: List[GoModule],
    flavor: AgentFlavor,
    module_class: GoModule,
    operation_name: str,
    command,
    skip_module_class: bool = False,
    headless_mode: bool = False,
):
    """
    Run the command function on each module of the modules list.
    """
    modules_results = []
    if not headless_mode:
        print(f"--- Flavor {flavor.name}: {operation_name}")
    for module in modules:
        module_result = None
        if not skip_module_class:
            module_result = module_class(path=module.full_path())
        if not headless_mode:
            skipped_header = "[Skipped]" if not module.condition() else ""
            print(f"----- {skipped_header} Module '{module.full_path()}'")
        if not module.condition():
            continue

        command(modules_results, module, module_result)
    return modules_results


def process_input_args(input_module, input_targets, input_flavor, headless_mode=False):
    """
    Takes the input module, targets and flavor arguments from inv test and inv codecov,
    sets default values for them & casts them to the expected types.
    """
    if isinstance(input_module, str):
        # when this function is called from the command line, targets are passed
        # as comma separated tokens in a string
        if isinstance(input_targets, str):
            modules = [GoModule(input_module, targets=input_targets.split(','))]
        else:
            modules = [m for m in DEFAULT_MODULES.values() if m.path == input_module]
    elif isinstance(input_targets, str):
        modules = [GoModule(".", targets=input_targets.split(','))]
    else:
        if not headless_mode:
            print("Using default modules and targets")
        modules = DEFAULT_MODULES.values()

    flavor = AgentFlavor.base
    if input_flavor:
        flavor = AgentFlavor[input_flavor]

    return modules, flavor


def process_module_results(flavor: AgentFlavor, module_results: Dict[str, Dict[str, List[ModuleResult]]]):
    """
    Prints failures in module results, and returns False if at least one module failed.
    """

    success = True
    for module_result in module_results:
        if module_result is not None:
            module_failed, failure_string = module_result.get_failure(flavor)
            success = success and (not module_failed)
            if module_failed:
                print(failure_string)

    return success
