"""Classes related to the CI configuration files."""

from pathlib import Path

import yaml

from tasks.libs.common.color import Color, color_message

# This file is used to set exceptions for jobs that do not require needs or rules
CONFIG_CI_LINTERS = Path(".gitlab/.ci-linters.yml")


class CILintersConfig:
    def __init__(
        self, path: str | Path = CONFIG_CI_LINTERS, lint=False, all_jobs: set[str] = None, all_stages: set[str] = None
    ) -> None:
        """Parses the ci linters configuration file and lints it.

        Args:
            lint: If True, will lint the file to verify that each job / stage is present in the configuration
            all_jobs: All the jobs in the configuration used to verify that the specified jobs are present within the full configuration
            all_stages: All the stages in the configuration used to verify that the specified stages are present within the full configuration
        """

        self.path = path
        with open(path) as f:
            config = yaml.safe_load(f)

        self.needs_rules_stages: set[str] = set(config['needs-rules']['allowed-stages'])
        self.needs_rules_jobs: set[str] = set(config['needs-rules']['allowed-jobs'])
        self.job_owners_jobs: set[str] = set(config['job-owners']['allowed-jobs'])

        if lint:
            self.lint_all(all_jobs, all_stages)

    def lint_assert_subset(self, errors: list[str], items: str, all_items: set[str], kind: str):
        """
        Asserts that multiple items belong to a super set.

        Args:
            kind: "job" or "stage"

        Side effects:
            errors will be appended with a message if the items are not a subset of all_items.

        Raises:
            AssertionError: Invalid kind argument
        """

        assert kind in ("job", "stage"), f"Invalid kind: {kind}"

        error_items = [item for item in items if item not in all_items]

        if error_items:
            error_items = '\n'.join(f'- {item}' for item in sorted(error_items))
            errors.append(f"The {self.path} file contains {kind}s not present in the configuration:\n{error_items}")

        return len(error_items)

    def lint_all(self, all_jobs, all_stages):
        errors = []

        self.lint_assert_subset(errors, self.needs_rules_jobs, all_jobs, "job")
        self.lint_assert_subset(errors, self.needs_rules_stages, all_stages, "stage")
        self.lint_assert_subset(errors, self.job_owners_jobs, all_jobs, "job")

        if errors:
            errors = '\n'.join(f"{color_message('Error', Color.RED)}: {error}" for error in errors)

            raise ValueError(errors)
