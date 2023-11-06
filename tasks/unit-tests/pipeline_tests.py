import subprocess
import unittest
from unittest.mock import MagicMock, patch

import yaml
from invoke import MockContext, Result
from invoke.exceptions import Exit

from .. import pipeline


class TestVerifyWorkspace(unittest.TestCase):
    @patch('tasks.pipeline.GithubAPI', autospec=True)
    @patch('tasks.pipeline.check_clean_branch_state', new=MagicMock())
    def test_with_branch(self, mock_gh):
        branch_test_name = "tryphon_tournesol"
        context_mock = MockContext(run=Result("haddock"))
        branch = pipeline.verify_workspace(context_mock, branch_test_name)
        self.assertEqual(branch_test_name, branch)
        mock_gh.assert_not_called()

    @patch('tasks.pipeline.GithubAPI', autospec=True)
    @patch('tasks.pipeline.check_clean_branch_state', new=MagicMock())
    def test_without_branch(self, mock_gh):
        context_mock = MockContext(run=Result("haddock"))
        branch = pipeline.verify_workspace(context_mock, None)
        self.assertEqual("haddock/test_buildimages", branch)
        mock_gh.assert_called()

    @patch('tasks.pipeline.GithubAPI', autospec=True)
    def test_bad_workspace(self, _):
        with open(".gitignore", "a") as f:
            f.write("# test comment")
        with self.assertRaises(Exit):
            context_mock = MockContext(run=Result("haddock"))
            _ = pipeline.verify_workspace(context_mock)
        subprocess.run("git checkout -- .gitignore".split())


class TestUpdateGitlabCI(unittest.TestCase):
    gitlabci_file = "tasks/unit-tests/testdata/fake_gitlab-ci.yml"
    erroneous_file = "tasks/unit-tests/testdata/erroneous_gitlab-ci.yml"

    def tearDown(self) -> None:
        subprocess.run(f"git checkout -- {self.gitlabci_file} {self.erroneous_file}".split())
        return super().tearDown()

    def test_nominal(self):
        pipeline.update_gitlab_config(self.gitlabci_file, "1mageV3rsi0n", test_version=True)
        with open(self.gitlabci_file, "r") as gl:
            gitlab_ci = yaml.safe_load(gl)
        for variable, value in gitlab_ci["variables"].items():
            # TEST_INFRA_DEFINITION_BUILDIMAGE label format differs from other buildimages
            if variable.endswith("_SUFFIX") and not variable.startswith("TEST_INFRA_DEFINITION"):
                self.assertEqual("_test_only", value)

    def test_update_no_test(self):
        pipeline.update_gitlab_config(self.gitlabci_file, "1mageV3rsi0n", test_version=False)
        with open(self.gitlabci_file, "r") as gl:
            gitlab_ci = yaml.safe_load(gl)
        for variable, value in gitlab_ci["variables"].items():
            if variable.endswith("_SUFFIX"):
                self.assertEqual("", value)

    def test_raise(self):
        with self.assertRaises(RuntimeError):
            pipeline.update_gitlab_config(self.erroneous_file, "1mageV3rsi0n", test_version=False)


class TestUpdateCircleCI(unittest.TestCase):
    circleci_file = "tasks/unit-tests/testdata/fake_circleci_config.yml"
    erroneous_file = "tasks/unit-tests/testdata/erroneous_circleci_config.yml"

    def tearDown(self) -> None:
        subprocess.run(f"git checkout -- {self.circleci_file} {self.erroneous_file}".split())
        return super().tearDown()

    def test_nominal(self):
        pipeline.update_circleci_config(self.circleci_file, "1m4g3", test_version=True)
        with open(self.circleci_file, "r") as gl:
            circle_ci = yaml.safe_load(gl)
        full_image = circle_ci['templates']['job_template']['docker'][0]['image']
        image, version = full_image.split(":")
        self.assertTrue(image.endswith("_test_only"))
        self.assertEqual("1m4g3", version)

    def test_update_no_test(self):
        pipeline.update_circleci_config(self.circleci_file, "1m4g3", test_version=False)
        with open(self.circleci_file, "r") as gl:
            circle_ci = yaml.safe_load(gl)
        full_image = circle_ci['templates']['job_template']['docker'][0]['image']
        image, version = full_image.split(":")
        self.assertFalse(image.endswith("_test_only"))
        self.assertEqual("1m4g3", version)

    def test_raise(self):
        with self.assertRaises(RuntimeError):
            pipeline.update_circleci_config(self.erroneous_file, "1m4g3", test_version=False)


if __name__ == "__main__":
    unittest.main()
