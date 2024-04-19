import subprocess
import unittest

import yaml

from tasks.libs.ciproviders.gitlab_api import update_gitlab_config
from tasks.libs.ciproviders.circleci import update_circleci_config


class TestUpdateGitlabCI(unittest.TestCase):
    gitlabci_file = "tasks/unit-tests/testdata/fake_gitlab-ci.yml"
    erroneous_file = "tasks/unit-tests/testdata/erroneous_gitlab-ci.yml"

    def tearDown(self) -> None:
        subprocess.run(f"git checkout -- {self.gitlabci_file} {self.erroneous_file}".split())
        return super().tearDown()

    def test_nominal(self):
        update_gitlab_config(self.gitlabci_file, "1mageV3rsi0n", test_version=True)
        with open(self.gitlabci_file) as gl:
            gitlab_ci = yaml.safe_load(gl)
        for variable, value in gitlab_ci["variables"].items():
            if variable.endswith("_SUFFIX"):
                self.assertEqual("_test_only", value)

    def test_update_no_test(self):
        update_gitlab_config(self.gitlabci_file, "1mageV3rsi0n", test_version=False)
        with open(self.gitlabci_file) as gl:
            gitlab_ci = yaml.safe_load(gl)
        for variable, value in gitlab_ci["variables"].items():
            if variable.endswith("_SUFFIX"):
                self.assertEqual("", value)

    def test_raise(self):
        with self.assertRaises(RuntimeError):
            update_gitlab_config(self.erroneous_file, "1mageV3rsi0n", test_version=False)


class TestUpdateCircleCI(unittest.TestCase):
    circleci_file = "tasks/unit-tests/testdata/fake_circleci_config.yml"
    erroneous_file = "tasks/unit-tests/testdata/erroneous_circleci_config.yml"

    def tearDown(self) -> None:
        subprocess.run(f"git checkout -- {self.circleci_file} {self.erroneous_file}".split())
        return super().tearDown()

    def test_nominal(self):
        update_circleci_config(self.circleci_file, "1m4g3", test_version=True)
        with open(self.circleci_file) as gl:
            circle_ci = yaml.safe_load(gl)
        full_image = circle_ci['templates']['job_template']['docker'][0]['image']
        image, version = full_image.split(":")
        self.assertTrue(image.endswith("_test_only"))
        self.assertEqual("1m4g3", version)

    def test_update_no_test(self):
        update_circleci_config(self.circleci_file, "1m4g3", test_version=False)
        with open(self.circleci_file) as gl:
            circle_ci = yaml.safe_load(gl)
        full_image = circle_ci['templates']['job_template']['docker'][0]['image']
        image, version = full_image.split(":")
        self.assertFalse(image.endswith("_test_only"))
        self.assertEqual("1m4g3", version)

    def test_raise(self):
        with self.assertRaises(RuntimeError):
            update_circleci_config(self.erroneous_file, "1m4g3", test_version=False)
