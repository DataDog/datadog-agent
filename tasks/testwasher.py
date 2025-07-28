from __future__ import annotations

import copy
import os
import re
from collections import defaultdict

import yaml
from invoke import Exit, task

from tasks.libs.ciproviders.gitlab_api import (
    resolve_gitlab_ci_configuration,
)
from tasks.libs.common.utils import gitlab_section
from tasks.libs.pipeline.generation import remove_fields, update_child_job_variables, update_needs_parent
from tasks.libs.testing.flakes import consolidate_flaky_failures
from tasks.libs.testing.result_json import ResultJson
from tasks.test_core import DEFAULT_TEST_OUTPUT_JSON, TestResult

FLAKY_TEST_INDICATOR = "flakytest: this is a known flaky test"


class TestWasher:
    def __init__(
        self,
        test_output_json_file=DEFAULT_TEST_OUTPUT_JSON,
        flaky_test_indicator=FLAKY_TEST_INDICATOR,
        flakes_file_paths: list[str] | None = None,
    ):
        """Used to deduce which tests are flaky using the resulting test output and the flaky configurations.

        Args:
            - flakes_file_paths: Paths to flake configuration files that will be merged. ["flakes.yaml"] by default
        """
        # If FLAKY_PATTERNS_CONFIG is set in the environment, it will be used to determine which tests are flaky. Should have the same format as flakes.yaml

        self.test_output_json_file = test_output_json_file
        self.test_output_json = ResultJson.from_file(test_output_json_file)  # type: ignore[attribute-defined-outside-init]
        self.flaky_test_indicator = flaky_test_indicator
        self.flakes_file_paths = flakes_file_paths or ["flakes.yaml"]
        if os.environ.get("FLAKY_PATTERNS_CONFIG"):
            self.flakes_file_paths.append(os.environ.get("FLAKY_PATTERNS_CONFIG"))

        self.known_flaky_tests = defaultdict(set)
        # flaky_log_patterns[package][test] = [pattern1, pattern2...]
        self.flaky_log_patterns = defaultdict(lambda: defaultdict(list))
        # Top level `on-log` used to have a pattern for every test
        self.flaky_log_main_patterns = []

        self.parse_flaky_files()

    def get_flaky_marked_tests(self) -> dict:
        """
        Read the test output json file and return the tests that are marked as flaky, tests that succeeded but are marked as flaky will be returned
        """
        marked_flaky_test = defaultdict(set)  # dict[package] =  {TestName, TestName2}

        for package, tests in self.test_output_json.package_tests_dict.items():
            for test_name, actions in tests.items():
                if test_name == "_":
                    # This is a package-level action list, we don't care about it for flaky tests
                    continue
                # If the test is marked as flaky, we add it to the marked flaky tests
                if any(self.is_marked_flaky(package, test_name, action.output) for action in actions if action.output):
                    marked_flaky_test[package].add(test_name)

        return marked_flaky_test

    def is_marked_flaky(self, package: str, test: str, log: str) -> bool:
        """Returns whether the test is marked as flaky based on the log output."""
        return (
            self.flaky_test_indicator in log
            or (
                package in self.flaky_log_patterns
                and test in self.flaky_log_patterns[package]
                and len(self.flaky_log_patterns[package][test]) > 0
            )
            or (package in self.known_flaky_tests and test in self.known_flaky_tests[package])
        )

    def get_failing_tests(self) -> dict[str, set[str]]:
        """
        Read the test output json file and return the tests that are failing
        """
        return self.test_output_json.failing_tests  # type: ignore[assignment]

    def get_flaky_failures(self) -> dict[str, set[str]]:
        """
        Return failures that are due to flakiness. A test is considered flaky if it failed because of a flake.
        In the following cases the test failure is considered a flaky failure:
        - The test failed and is marked as flaky
        - Test failed and is not marked as flaky but all its failing children are marked as flaky or are flaky failures.
        """
        # TODO[ACIX-913]: Fix the semantics of this function - it actually returns all flaky tests, not just *failing* tests.
        # See for example the test_pretty_print_inner_depth2 test in tasks/unit_tests/e2e_testing_tests.py.
        # When running the test, this function returns a non-empty result, even though all the tests in the dummy json output are passing.
        # This is not super easy to fix, because it is used in many places that now rely on this behavior.
        failing_tests = self.test_output_json.failing_tests

        flaky_failures = defaultdict(set)
        for package, tests in self.test_output_json.package_tests_dict.items():
            for test, lines in tests.items():
                if any(self.is_flaky_failure(package, test, line.output) for line in lines if line.output):
                    flaky_failures[package].add(test)

        for pkg, test in failing_tests.items():
            if pkg not in flaky_failures:
                continue
            flaky_failures[pkg] = consolidate_flaky_failures(flaky_failures[pkg], test)

        return flaky_failures

    def is_flaky_failure(self, package: str, test: str, log: str) -> bool:
        """
        Returns whether the test is a flaky failure:
        - on-log: pattern is matching the log
        - test is marked as flaky no matter the log
        """
        if package in self.known_flaky_tests and test in self.known_flaky_tests[package]:
            return True
        if self.flaky_test_indicator in log:
            return True

        # flake.MarkOnLog can be used in a parent test to mark a subtest.
        # As a consequence we need to append parent test information.
        all_flaky_log_patterns = []
        test_parts = test.split('/')
        for i_part in range(len(test_parts)):
            parent_test = '/'.join(test_parts[: i_part + 1])
            if parent_test in self.flaky_log_patterns[package]:
                all_flaky_log_patterns += self.flaky_log_patterns[package][parent_test]

        for pattern in self.flaky_log_main_patterns + all_flaky_log_patterns:
            if re.search(pattern, log, re.IGNORECASE):
                return True

        return False

    def get_non_flaky_failing_tests(self):
        """
        Parse the test output json file and compute the failing tests and the one known flaky
        """
        failing_tests = self.get_failing_tests()
        flaky_failures = self.get_flaky_failures()

        non_flaky_failures = defaultdict(set)
        for package, tests in failing_tests.items():
            non_flaky_failures[package] = tests - flaky_failures[package]

        # Clean empty packages
        non_flaky_failures = {k: v for k, v in non_flaky_failures.items() if v}

        return non_flaky_failures

    def parse_flaky_files(self):
        """
        Parse the flakes.yaml like files and add the tests listed there to the known flaky tests list / the flaky log patterns to the flaky log patterns list
        """
        reserved_keywords = ("on-log",)

        for path in self.flakes_file_paths:
            with open(path) as f:
                flakes = yaml.safe_load(f)

            if not flakes:
                continue

            # Add the tests to the known flaky tests list
            for package, tests in flakes.items():
                if package in reserved_keywords:
                    continue

                for test in tests:
                    if 'on-log' in test:
                        patterns = test['on-log']
                        if isinstance(patterns, str):
                            patterns = [patterns]
                        self.flaky_log_patterns[f"github.com/DataDog/datadog-agent/{package}"][test['test']] += patterns
                    else:
                        # If there is no `on-log`, we consider it as a known flaky test right away
                        self.known_flaky_tests[f"github.com/DataDog/datadog-agent/{package}"].add(test['test'])

            # on-log patterns at the top level
            main_patterns = flakes.get('on-log', [])
            if isinstance(main_patterns, str):
                main_patterns = [main_patterns]
            self.flaky_log_main_patterns += main_patterns

    def process_result(self, result: TestResult):
        """
        Process the test results and decide whether we should succeed or not.
        If only known flaky tests are failing, we should succeed.
        If failing, displays the failing tests that are not known to be flaky
        """
        should_succeed = True
        failed_command = False
        failed_tests = []
        non_flaky_failing_tests = self.get_non_flaky_failing_tests()
        if (
            not self.get_failing_tests() and result.failed
        ):  # In this case the Go test command failed but no test failed, it means that the test command itself failed (build errors,...)
            should_succeed = False
            failed_command = True
        if non_flaky_failing_tests:
            should_succeed = False
            for package, tests in non_flaky_failing_tests.items():
                failed_tests.extend(f"- {package} {test}" for test in tests)
        if failed_tests:
            print("The test command failed, the following tests failed and are not supposed to be flaky:")
            print("\n".join(sorted(failed_tests)))
        if failed_command:
            print("The test command failed, before test execution.")
            print("Please check the job logs for more information.")

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

    # Remove rules, extends and retry from the jobs, update needs to point to parent pipeline
    for job in list(kept_job.values()):
        remove_fields(job)
        if "needs" in job:
            job["needs"] = update_needs_parent(
                job["needs"],
                deps_to_keep=["go_e2e_deps", "tests_windows_sysprobe_x64", "tests_windows_secagent_x64"],
                package_deps=[
                    "agent_deb-x64-a7-fips",
                    "agent_deb-x64-a7",
                    "windows_msi_and_bosh_zip_x64-a7-fips",
                    "windows_msi_and_bosh_zip_x64-a7",
                    "agent_rpm-x64-a7",
                    "agent_suse-x64-a7",
                ],
            )

    new_jobs = {}
    new_jobs['variables'] = copy.deepcopy(config['variables'])
    new_jobs['default'] = copy.deepcopy(config['default'])
    new_jobs['variables']['PARENT_PIPELINE_ID'] = 'undefined'
    new_jobs['variables']['PARENT_COMMIT_SHA'] = 'undefined'
    new_jobs['variables']['PARENT_COMMIT_SHORT_SHA'] = 'undefined'
    new_jobs['stages'] = [f'flake-finder-{i}' for i in range(n)]

    updated_jobs = update_child_job_variables(kept_job)
    # Create n jobs with the same configuration
    for job in kept_job:
        for i in range(n):
            new_jobs[f"{job}-{i}"] = copy.deepcopy(updated_jobs[job])
            new_jobs[f"{job}-{i}"]["stage"] = f"flake-finder-{i}"

            if 'E2E_PRE_INITIALIZED' in new_jobs[f"{job}-{i}"]['variables']:
                del new_jobs[f"{job}-{i}"]['variables']['E2E_PRE_INITIALIZED']

            new_jobs[f"{job}-{i}"]["rules"] = [{"when": "always"}]
            if i > 0:
                new_jobs[f"{job}-{i}"]["needs"].append({"job": f"{job}-{i - 1}", "artifacts": False})

    with open("flake-finder-gitlab-ci.yml", "w") as f:
        f.write(yaml.safe_dump(new_jobs))

    with gitlab_section("Flake finder generated pipeline", collapsed=True):
        print(yaml.safe_dump(new_jobs))
