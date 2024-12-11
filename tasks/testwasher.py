from __future__ import annotations

import copy
import json
import os
from collections import defaultdict

import yaml
from invoke import Exit, task

from tasks.libs.ciproviders.gitlab_api import (
    resolve_gitlab_ci_configuration,
)
from tasks.libs.common.utils import gitlab_section
from tasks.libs.testing.flakes import get_tests_family_if_failing_tests, is_known_flaky_test
from tasks.test_core import ModuleTestResult

FLAKY_TEST_INDICATOR = "flakytest: this is a known flaky test"


class TestWasher:
    def __init__(
        self,
        test_output_json_file="module_test_output.json",
        flaky_test_indicator=FLAKY_TEST_INDICATOR,
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
            known_flaky_tests_parents = get_tests_family_if_failing_tests(
                all_known_flakes[package], failing_tests[package]
            )
            for failing_test in tests:
                if not is_known_flaky_test(failing_test, all_known_flakes[package], known_flaky_tests_parents):
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


@task
def generate_flake_finder_pipeline(ctx, n=3, generate_config=False):
    """
    Generate a child pipeline where jobs marked with SHOULD_RUN_IN_FLAKES_FINDER are run n times
    """
    if generate_config:
        # Read gitlab config
        config = resolve_gitlab_ci_configuration(ctx, ".gitlab-ci.yml")
    else:
        # Read gitlab config, which is computed and stored in compute_gitlab_ci_config job
        if not os.path.exists("artifacts/after.gitlab-ci.yml"):
            raise Exit(
                "The configuration is not stored as artifact. Please ensure you ran the compute_gitlab_ci_config job, or set generate_config to True"
            )
        with open("artifacts/after.gitlab-ci.yml") as f:
            config = yaml.safe_load(f)[".gitlab-ci.yml"]

    # Lets keep only variables and jobs with flake finder variable
    kept_job = {}
    for job, job_details in config.items():
        if (
            'variables' in job_details
            and 'SHOULD_RUN_IN_FLAKES_FINDER' in job_details['variables']
            and job_details['variables']['SHOULD_RUN_IN_FLAKES_FINDER'] == "true"
            and not job.startswith(".")
        ):
            # Let's exclude job that are retried for now until we find a solution to tackle them
            if 'retry' in job_details:
                continue
            kept_job[job] = job_details

    deps_job = copy.deepcopy(config["go_e2e_deps"])

    # Remove needs, rules, extends and retry from the jobs
    for job in [deps_job] + list(kept_job.values()):
        _clean_job(job)

    new_jobs = {}
    new_jobs["go_e2e_deps"] = deps_job
    new_jobs['variables'] = copy.deepcopy(config['variables'])
    new_jobs['variables']['PARENT_PIPELINE_ID'] = 'undefined'
    new_jobs['variables']['PARENT_COMMIT_SHA'] = 'undefined'
    new_jobs['stages'] = [deps_job["stage"]] + [f'flake-finder-{i}' for i in range(n)]

    # Create n jobs with the same configuration
    for job in kept_job:
        for i in range(n):
            new_job = copy.deepcopy(kept_job[job])
            new_job["stage"] = f"flake-finder-{i}"
            new_job["dependencies"] = ["go_e2e_deps"]
            if 'variables' in new_job:
                if (
                    'E2E_PIPELINE_ID' in new_job['variables']
                    and new_job['variables']['E2E_PIPELINE_ID'] == "$CI_PIPELINE_ID"
                ):
                    new_job['variables']['E2E_PIPELINE_ID'] = "$PARENT_PIPELINE_ID"
                if (
                    'E2E_COMMIT_SHA' in new_job['variables']
                    and new_job['variables']['E2E_COMMIT_SHA'] == "$CI_COMMIT_SHA"
                ):
                    new_job['variables']['E2E_COMMIT_SHA'] = "$PARENT_COMMIT_SHA"
                if 'E2E_PRE_INITIALIZED' in new_job['variables']:
                    del new_job['variables']['E2E_PRE_INITIALIZED']
            new_job["rules"] = [{"when": "always"}]
            new_job["needs"] = ["go_e2e_deps"]
            new_jobs[f"{job}-{i}"] = new_job

    with open("flake-finder-gitlab-ci.yml", "w") as f:
        f.write(yaml.safe_dump(new_jobs))

    with gitlab_section("Flake finder generated pipeline", collapsed=True):
        print(yaml.safe_dump(new_jobs))


def _clean_job(job):
    """
    Remove the needs, rules, extends and retry from the job
    """
    for step in ('needs', 'rules', 'extends', 'retry'):
        if step in job:
            del job[step]
    return job
