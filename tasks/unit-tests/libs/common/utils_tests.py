import os
import unittest
from unittest import mock

from tasks.libs.common.utils import (
    running_in_ci,
    running_in_circleci,
    running_in_github_actions,
    running_in_gitlab_ci,
    running_in_pre_commit,
    running_in_pyapp,
)


class TestRunningIn(unittest.TestCase):
    def test_running_in(self):
        parameters = [
            ("PRE_COMMIT", "1", True, running_in_pre_commit),
            ("PRE_COMMIT", "", False, running_in_pre_commit),
            ("PYAPP", "1", True, running_in_pyapp),
            ("PYAPP", "", False, running_in_pyapp),
            ("CIRCLECI", "true", True, running_in_circleci),
            ("CIRCLECI", "", False, running_in_circleci),
            ("GITLAB_CI", "true", True, running_in_gitlab_ci),
            ("GITLAB_CI", "", False, running_in_gitlab_ci),
            ("GITHUB_ACTIONS", "true", True, running_in_github_actions),
            ("GITHUB_ACTIONS", "", False, running_in_github_actions),
            ("GITHUB_ACTIONS", "true", True, running_in_ci),
            ("GITLAB_CI", "true", True, running_in_ci),
            ("CIRCLECI", "true", True, running_in_ci),
            ("CIRCLECI", "false", False, running_in_ci),
            ("GITHUB_ACTIONS", "false", False, running_in_ci),
            ("GITLAB_CI", "false", False, running_in_ci),
        ]

        for env_var, value, expected, func in parameters:
            with self.subTest(env_var=env_var, value=value, expected_value=expected):
                with mock.patch.dict(os.environ, {env_var: value}):
                    self.assertEqual(expected, func())
