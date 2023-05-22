import subprocess
import unittest
from unittest.mock import mock_open, patch

import yaml

from .. import pipeline


class TestUpdateGitlabCI(unittest.TestCase):
    def tearDown(self) -> None:
        subprocess.run("git checkout -- .gitlab-ci.yml".split())
        return super().tearDown()

    def test_nominal(self):
        pipeline.update_gitlab_ci("1mageV3rsi0n", test_version=True)
        with open(".gitlab-ci.yml", "r") as gl:
            gitlab_ci = yaml.safe_load(gl)
        for variable, value in gitlab_ci["variables"].items():
            if variable.endswith("_SUFFIX"):
                self.assertEqual("_test_only", value)

    def test_update_no_test(self):
        pipeline.update_gitlab_ci("1mageV3rsi0n", test_version=False)
        with open(".gitlab-ci.yml", "r") as gl:
            gitlab_ci = yaml.safe_load(gl)
        for variable, value in gitlab_ci["variables"].items():
            if variable.endswith("_SUFFIX"):
                self.assertEqual("", value)

    @patch(
        'builtins.open',
        new=mock_open(read_data="---\nvariables:\n DD_AGENT_IMAGE_SUFFIX: ''\n DD_AGENT_IMAGE: 42\n"),
    )
    def test_raise(self):
        with self.assertRaises(RuntimeError):
            pipeline.update_gitlab_ci("1mageV3rsi0n", test_version=False)


class TestUpdateCircleCI(unittest.TestCase):
    def tearDown(self) -> None:
        subprocess.run("git checkout -- .circleci/config.yml".split())
        return super().tearDown()

    def test_nominal(self):
        pipeline.update_circle_ci("1m4g3", test_version=True)
        with open(".circleci/config.yml", "r") as gl:
            circle_ci = yaml.safe_load(gl)
        image = circle_ci['templates']['job_template']['docker'][0]['image']
        version = image.split(":")[-1]
        self.assertEqual("1m4g3_test_only", version)

    def test_update_no_test(self):
        pipeline.update_circle_ci("1m4g3", test_version=False)
        with open(".circleci/config.yml", "r") as gl:
            circle_ci = yaml.safe_load(gl)
        image = circle_ci['templates']['job_template']['docker'][0]['image']
        version = image.split(":")[-1]
        self.assertEqual("1m4g3", version)

    @patch(
        'builtins.open',
        new=mock_open(
            read_data="---\ntemplates:\n job_template: &job_template\n docker:\n - image: datadog/datadog-agent-runner-name-changed:go1199\n"
        ),
    )
    def test_raise(self):
        with self.assertRaises(RuntimeError):
            pipeline.update_circle_ci("1m4g3", test_version=False)


if __name__ == "__main__":
    unittest.main()
