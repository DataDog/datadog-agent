import os
import unittest
from unittest.mock import MagicMock, patch

from invoke import Exit, MockContext

import tasks.linter as linter


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
                "DD_APP_KEY=$($CI_PROJECT_DIR/tools/ci/fetch_secret.sh $APP_KEY_ORG2) || exit $?; export DD_APP_KEY"
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
    @patch("builtins.print")
    @patch(
        "tasks.linter.generate_gitlab_full_configuration",
        new=MagicMock(return_value={"rules": {"changes": {"paths": ["tasks/**/*.py"]}}}),
    )
    def test_all_ok(self, print_mock):
        linter.gitlab_change_paths(MockContext())
        print_mock.assert_called_with("All rule:changes:paths from gitlab-ci are \x1b[92mvalid\x1b[0m.")

    @patch(
        "tasks.linter.generate_gitlab_full_configuration",
        new=MagicMock(
            return_value={"rules": {"changes": {"paths": ["tosks/**/*.py", "tasks/**/*.py", "tusks/**/*.py"]}}}
        ),
    )
    def test_bad_paths(self):
        with self.assertRaises(Exit):
            linter.gitlab_change_paths(MockContext())
