import shutil
import subprocess
import unittest
from pathlib import Path

import yaml

from tasks.libs.ciproviders.circleci import update_circleci_config


class TestUpdateCircleCI(unittest.TestCase):
    circleci_file = ".circleci/config.yml"
    circleci_test = ".circleci/config-test.yml"
    erroneous_file = "tasks/unit_tests/testdata/erroneous_circleci_config.yml"

    def setUp(self) -> None:
        shutil.copy(self.circleci_file, self.circleci_test)
        return super().setUp()

    def tearDown(self) -> None:
        subprocess.run(f"git checkout -- {self.erroneous_file}".split())
        Path(self.circleci_test).unlink()
        return super().tearDown()

    def test_nominal(self):
        update_circleci_config(self.circleci_test, "1m4g3", test=True)
        with open(self.circleci_test) as gl:
            circle_ci = yaml.safe_load(gl)
        full_image = circle_ci['templates']['job_template']['docker'][0]['image']
        image, version = full_image.split(":")
        self.assertTrue(image.endswith("_test_only"))
        self.assertEqual("1m4g3", version)

    def test_update_no_test(self):
        update_circleci_config(self.circleci_test, "1m4g3", test=False)
        with open(self.circleci_test) as gl:
            circle_ci = yaml.safe_load(gl)
        full_image = circle_ci['templates']['job_template']['docker'][0]['image']
        image, version = full_image.split(":")
        self.assertFalse(image.endswith("_test_only"))
        self.assertEqual("1m4g3", version)

    def test_raise(self):
        with self.assertRaises(RuntimeError):
            update_circleci_config(self.erroneous_file, "1m4g3", test=False)
