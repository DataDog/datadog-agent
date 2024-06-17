from __future__ import annotations

import copy
import json
from collections import defaultdict

import yaml
from invoke import task

from tasks.libs.ciproviders.gitlab_api import (
    generate_gitlab_full_configuration,
)
from tasks.test_core import ModuleTestResult


class TestWasher:
    def __init__(
        self,
        test_output_json_file="module_test_output.json",
        flaky_test_indicator="flakytest: this is a known flaky test",
        flakes_file_path="flakes.yaml",
    ):
        self.test_output_json_file = test_output_json_file
        self.flaky_test_indicator = flaky_test_indicator
        self.flakes_file_path = flakes_file_path
        self.known_flaky_tests = defaultdict(set)

        self.parse_flaky_file()

    def get_non_flaky_failing_tests(self, failing_tests: dict, flaky_marked_tests: dict):
        """
        Parse the test output json file and compute the failing tests and the one known flaky
        """

        all_known_flakes = self.merge_known_flakes(flaky_marked_tests)
        non_flaky_failing_tests = defaultdict(set)

        for package, tests in failing_tests.items():
            non_flaky_failing_tests_in_package = set()
            known_flaky_tests_parents = self.get_tests_family(all_known_flakes[package])
            for failing_test in tests:
                if not self.is_known_flaky_test(failing_test, all_known_flakes[package], known_flaky_tests_parents):
                    non_flaky_failing_tests_in_package.add(failing_test)
            if non_flaky_failing_tests_in_package:
                non_flaky_failing_tests[package] = non_flaky_failing_tests_in_package
        return non_flaky_failing_tests

    def merge_known_flakes(self, marked_flakes):
        """
        Merge flakes marked in the go code and the ones from the flakes.yaml file
        """
        known_flakes = self.known_flaky_tests.copy()
        for package, tests in marked_flakes.items():
            if package in known_flakes:
                known_flakes[package] = known_flakes[package].union(tests)
            else:
                known_flakes[package] = tests
        return known_flakes

    def parse_flaky_file(self):
        """
        Parse the flakes.yaml file and add the tests listed there to the kown flaky tests list
        """
        with open(self.flakes_file_path) as f:
            flakes = yaml.safe_load(f)
            if not flakes:
                return
            for package, tests in flakes.items():
                self.known_flaky_tests[f"github.com/DataDog/datadog-agent/{package}"].update(set(tests))

    def parse_test_results(self, module_path: str) -> tuple[dict, dict]:
        failing_tests = defaultdict(set)
        flaky_marked_tests = defaultdict(set)

        with open(f"{module_path}/{self.test_output_json_file}", encoding='utf-8') as f:
            for line in f:
                test_result = json.loads(line)
                if "Test" not in test_result:
                    continue
                if test_result["Action"] == "fail":
                    failing_tests[test_result["Package"]].add(test_result["Test"])
                if test_result["Action"] == "success":
                    if test_result["Test"] in failing_tests[test_result["Package"]]:
                        failing_tests[test_result["Package"]].remove(test_result["Test"])
                if "Output" in test_result and self.flaky_test_indicator in test_result["Output"]:
                    flaky_marked_tests[test_result["Package"]].add(test_result["Test"])
        return failing_tests, flaky_marked_tests

    def process_module_results(self, module_results: list[ModuleTestResult]):
        """
        Process the module test results and decide whether we should succeed or not.
        If only known flaky tests are failing, we should succeed.
        If failing, displays the failing tests that are not known to be flaky
        """

        should_succeed = True
        failed_tests = []
        failed_command_modules = []
        for module_result in module_results:
            failing_tests, flaky_marked_tests = self.parse_test_results(module_result.path)
            non_flaky_failing_tests = self.get_non_flaky_failing_tests(
                failing_tests=failing_tests, flaky_marked_tests=flaky_marked_tests
            )
            if (
                not failing_tests and module_result.failed
            ):  # In this case the Go test command failed on one of the modules but no test failed, it means that the test command itself failed (build errors,...)
                should_succeed = False
                failed_command_modules.append(module_result.path)
            if non_flaky_failing_tests:
                should_succeed = False
                for package, tests in non_flaky_failing_tests.items():
                    failed_tests.extend(f"- {package} {test}" for test in tests)
        if failed_tests:
            print("The test command failed, the following tests failed and are not supposed to be flaky:")
            print("\n".join(sorted(failed_tests)))
        if failed_command_modules:
            print("The test command failed, before test execution on the following modules:")
            print("\n".join(sorted(failed_command_modules)))
            print("Please check the job logs for more information")

        return should_succeed

    def is_known_flaky_test(self, failing_test, known_flaky_tests, known_flaky_tests_parents):
        """
        Check if a test is known to be flaky
        The method should be called with the following arguments:
        - failing_test: the test that is failing
        - known_flaky_tests: the set of tests that are known to be flaky
        - known_flaky_tests_parents: the set of tests that are ancestors of a known flaky test, thus would fail when the flaky leaf test fails
        If a test is a parent of a test that is known to be flaky, the test should be considered flaky
        For example:
        - if TestEKSSuite/TestCPU is known to be flaky, TestEKSSuite/TestCPU/TestCPUUtilization should be considered flaky
        - if TestEKSSuite/TestCPU is known to be flaky, TestEKSSuite should be considered flaky
        - if TestEKSSuite/TestCPU is known to be flaky, TestEKSSuite/TestMemory should not be considered flaky
        """

        failing_test_parents = self.get_tests_family([failing_test])

        if any(parent in known_flaky_tests for parent in failing_test_parents):
            return True

        return failing_test in known_flaky_tests_parents

    def get_tests_family(self, test_name_list):
        """
        Get the parent tests of a list of tests
        For example with the test ["TestEKSSuite/TestCPU/TestCPUUtilization", "TestKindSuite/TestCPU"]
        this method should return the set{"TestEKSSuite/TestCPU/TestCPUUtilization", "TestEKSSuite/TestCPU", "TestEKSSuite", "TestKindSuite/TestCPU", "TestKindSuite"}
        """
        test_family = set(test_name_list)
        for test_name in test_name_list:
            while test_name.count('/') > 0:
                test_name = test_name.rsplit('/', 1)[0]
                test_family.add(test_name)
        return test_family


@task()
def generate_flake_finder_pipeline(_, n=3):
    """
    Verify that the jobs defined within job_files contain a change path rule.
    """

    # Read gitlab config
    config = generate_gitlab_full_configuration(".gitlab-ci.yml", {}, return_dump=False, apply_postprocessing=True)

    for job in config:
        print(job)
    # Lets keep only variables and jobs with flake finder variable
    kept_job = {}
    for job, job_details in config.items():
        if (
            job == 'variables'
            or 'variables' in job_details
            and 'FLAKES_FINDER' in job_details['variables']
            and job_details['variables']['FLAKES_FINDER'] == "true"
        ):
            # Let's exlude job that are retried for now untill we find a solution to tackle them
            if 'retry' in job_details:
                continue
            kept_job[job] = job_details

    # Remove needs, rules and retry from the jobs
    for job in kept_job:
        if 'needs' in kept_job[job]:
            del kept_job[job]["needs"]
        if 'rules' in kept_job[job]:
            del kept_job[job]["rules"]
        if 'retry' in kept_job[job]:
            del kept_job[job]["retry"]

    new_jobs = {}
    new_jobs['variables'] = copy.deepcopy(config['variables'])
    new_jobs['variables']['PARENT_PIPELINE_ID'] = 'undefined'
    new_jobs['variables']['PARENT_COMMIT_SHA'] = 'undefined'
    new_jobs['stages'] = [f'flake-finder-{i}' for i in range(n)]

    # Create n jobs with the same configuration
    for job in kept_job:
        for i in range(n):
            new_job = copy.deepcopy(kept_job[job])
            new_job["stage"] = f"flake-finder-{i}"
            if 'variables' in new_job:
                if 'E2E_PIPELINE_ID' in new_job['variables']:
                    new_job['variables']['E2E_PIPELINE_ID'] = "$PARENT_PIPELINE_ID"
                if 'E2E_COMMIT_SHA' in new_job['variables']:
                    new_job['variables']['E2E_COMMIT_SHA'] = "$PARENT_COMMIT_SHA"
            new_jobs[f"{job}-{i}"] = new_job

    with open("flake-finder-gitlab-ci.yml", "w") as f:
        f.write(yaml.safe_dump(new_jobs))

    print("New Gitlab-ci.yml:", yaml.safe_dump(new_jobs))
