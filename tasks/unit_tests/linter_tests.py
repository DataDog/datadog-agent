import os
import unittest
from unittest.mock import MagicMock

from codeowners import CodeOwners

import tasks.linter as linter
from tasks.libs.linter.gitlab_exceptions import FailureLevel, GitlabLintFailure


class TestIsGetParameterCall(unittest.TestCase):
    test_file = "test_linter.tmp"

    def tearDown(self):
        if os.path.exists(self.test_file):
            os.unlink(self.test_file)

    def test_no_get_param(self):
        with open(self.test_file, "w") as f:
            f.write("Hello World")
        matched = linter.list_get_parameter_calls(self.test_file)
        self.assertListEqual([], matched)

    def test_without_wrapper_no_env(self):
        with open(self.test_file, "w") as f:
            f.write(
                "API_KEY=$(aws ssm get-parameter --region us-east-1 --name test.datadog-agent.datadog_api_key_org2 --with-decryption  --query Parameter.Value --out text)"
            )
        matched = linter.list_get_parameter_calls(self.test_file)[0]
        self.assertFalse(matched.with_wrapper)
        self.assertFalse(matched.with_env_var)

    def test_without_wrapper_with_env(self):
        with open(self.test_file, "w") as f:
            f.write(
                "  - DD_API_KEY=$(aws ssm get-parameter --region us-east-1 --name $API_KEY_ORG2 --with-decryption  --query Parameter.Value --out text || exit $?; export DD_API_KEY"
            )
        matched = linter.list_get_parameter_calls(self.test_file)[0]
        self.assertFalse(matched.with_wrapper)
        self.assertTrue(matched.with_env_var)

    def test_with_wrapper_no_env(self):
        with open(self.test_file, "w") as f:
            f.write(
                "DD_API_KEY=$($CI_PROJECT_DIR/tools/ci/fetch_secret.sh test.datadog-agent.datadog_api_key_org2) || exit $?; export DD_API_KEY"
            )
        matched = linter.list_get_parameter_calls(self.test_file)[0]
        self.assertTrue(matched.with_wrapper)
        self.assertFalse(matched.with_env_var)

    def test_with_wrapper_with_env(self):
        with open(self.test_file, "w") as f:
            f.write(
                "DD_APP_KEY=$($CI_PROJECT_DIR/tools/ci/fetch_secret.sh $AGENT_APP_KEY_ORG2 token) || exit $?; export DD_APP_KEY"
            )
        matched = linter.list_get_parameter_calls(self.test_file)
        self.assertListEqual([], matched)

    def test_multi_match_windows(self):
        with open(self.test_file, "w") as f:
            f.write(
                'DD_API_KEY=$(& "$CI_PROJECT_DIR\tools \\ci\fetch_secret.ps1" -parameterName test.datadog-agent.datadog_api_key_org2 -tempFile $tmpfile)\n'
                'DD_API_KEY=$(& "$CI_PROJECT_DIR\tools \\ci\fetch secret.ps1" -parameterName "$Env:MISSING_UNDERSCORE" -tempFile $tmpfile)\n'
                '`DD_APP_KEY=$(& "$CI_PROJECT_DIR\tools\\ci\fetch_secret.ps1" -parameterName "bad.name" -tempFile "$tmpfile")\n'
                'DD_APP=$(& "$CI_PROJECT_DIR\tools\\ci\fetch_secret.ps1" -parameterName "$Env:TEST" -tempFile $tmpfile)\n'
            )
        matched = linter.list_get_parameter_calls(self.test_file)
        self.assertEqual(2, len(matched))
        self.assertTrue(matched[0].with_wrapper)
        self.assertFalse(matched[0].with_env_var)
        self.assertTrue(matched[1].with_wrapper)
        self.assertFalse(matched[1].with_env_var)


class TestGitlabChangePaths(unittest.TestCase):
    def test_all_ok(self):
        linter.check_change_paths_valid_gitlab_ci_jobs(
            [("somejob", {"rules": {"changes": {"paths": ["tasks/**/*.py"]}}})]
        )

    def test_bad_paths(self):
        with self.assertRaises(GitlabLintFailure):
            linter.check_change_paths_valid_gitlab_ci_jobs(
                [("somejob", {"rules": {"changes": {"paths": ["tosks/**/*.py", "tasks/**/*.py", "tusks/**/*.py"]}}})]
            )


class TestGitlabCIJobsNeedsRules(unittest.TestCase):
    def empty_ci_linters_config(self) -> MagicMock:
        mock = MagicMock()
        mock.job_owners_jobs = set()
        mock.path = './config.yml'

        return mock

    def test_no_changes(self):
        linter.check_needs_rules_gitlab_ci_jobs([], self.empty_ci_linters_config())

    def test_ok(self):
        linter.check_needs_rules_gitlab_ci_jobs(
            [('hello', {'stage': 'lint', 'script': 'echo hello', 'needs': [], 'rules': []})],
            self.empty_ci_linters_config(),
        )

    def test_error(self):
        with self.assertRaises(GitlabLintFailure) as cm:
            linter.check_needs_rules_gitlab_ci_jobs(
                [('hello', {'stage': 'lint', 'script': 'echo hello'})], self.empty_ci_linters_config()
            )
        self.assertEqual(cm.exception.level, FailureLevel.ERROR)


class TestGitlabCIJobsOwners(unittest.TestCase):
    def empty_ci_linters_config(self) -> MagicMock:
        mock = MagicMock()
        mock.job_owners_jobs = set()
        mock.path = './config.yml'

        return mock

    def test_one_job(self):
        jobowners = """
        /somejob        @DataDog/the-best-team
        """

        linter.check_owners_gitlab_ci_jobs([('somejob', {})], self.empty_ci_linters_config(), CodeOwners(jobowners))

    def test_one_job_glob(self):
        jobowners = """
        /my*        @DataDog/the-best-team
        """

        linter.check_owners_gitlab_ci_jobs([('myjob', {})], self.empty_ci_linters_config(), CodeOwners(jobowners))

    def test_one_job_fail(self):
        jobowners = """
        /somejob        @DataDog/the-best-team
        """

        with self.assertRaises(GitlabLintFailure) as cm:
            linter.check_owners_gitlab_ci_jobs(
                [('someotherjob', {})], self.empty_ci_linters_config(), CodeOwners(jobowners)
            )
        self.assertEqual(cm.exception.level, FailureLevel.ERROR)

    def test_multiple_jobs(self):
        jobowners = """
        /somejob        @DataDog/the-best-team
        /my*            @DataDog/another-best-team
        """

        linter.check_owners_gitlab_ci_jobs(
            [('somejob', {}), ('myjob', {})], self.empty_ci_linters_config(), CodeOwners(jobowners)
        )

    def test_multiple_jobs_fail(self):
        jobowners = """
        /somejob        @DataDog/the-best-team
        """

        with self.assertRaises(GitlabLintFailure) as cm:
            linter.check_owners_gitlab_ci_jobs(
                [('somejob', {}), ('someotherjob', {})],
                self.empty_ci_linters_config(),
                CodeOwners(jobowners),
            )
        self.assertEqual(cm.exception.level, FailureLevel.ERROR)

    def test_multiple_jobs_ignore(self):
        jobowners = """
        /somejob        @DataDog/the-best-team
        """

        config = self.empty_ci_linters_config()
        config.job_owners_jobs.add('someotherjob')

        with self.assertRaises(GitlabLintFailure) as cm:
            linter.check_owners_gitlab_ci_jobs([('somejob', {}), ('someotherjob', {})], config, CodeOwners(jobowners))
        self.assertEqual(cm.exception.level, FailureLevel.IGNORED)


class TestGitlabCIJobsCodeowners(unittest.TestCase):
    def test_no_file(self):
        codeowners = """
        /somefile       @DataDog/the-best-team
        /.*             @DataDog/another-best-team
        """

        linter._gitlab_ci_jobs_codeowners_lint([], CodeOwners(codeowners))

    def test_one_file(self):
        codeowners = """
        /somefile       @DataDog/the-best-team
        /.*             @DataDog/another-best-team
        """

        linter._gitlab_ci_jobs_codeowners_lint(['somefile'], CodeOwners(codeowners))

    def test_multiple_files(self):
        codeowners = """
        /somefile       @DataDog/the-best-team
        /.*             @DataDog/another-best-team
        """

        linter._gitlab_ci_jobs_codeowners_lint(['somefile', '.gitlab-ci.yml'], CodeOwners(codeowners))

    def test_error(self):
        codeowners = """
        /somefile       @DataDog/the-best-team
        /.*             @DataDog/another-best-team
        """

        with self.assertRaises(GitlabLintFailure) as cm:
            linter._gitlab_ci_jobs_codeowners_lint(['becareful', '.gitlab-ci.yml'], CodeOwners(codeowners))
        self.assertEqual(cm.exception.level, FailureLevel.ERROR)
