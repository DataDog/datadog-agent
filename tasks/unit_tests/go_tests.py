import unittest
from unittest.mock import MagicMock, patch

from tasks.gotest import find_impacted_packages, should_run_all_tests


class TestUtils(unittest.TestCase):
    def test_impacted_packages_1(self):
        dependencies = {
            "pkg1": [
                "pkg2",
            ],
            "pkg2": ["pkg3"],
            "pkg3": [],
        }
        changed_files = {"pkg1"}
        expected_impacted_packages = {"pkg1", "pkg2", "pkg3"}
        self.assertEqual(find_impacted_packages(dependencies, changed_files), expected_impacted_packages)

    def test_impacted_packages_2(self):
        dependencies = {
            "pkg1": ["pkg2", "pkg3"],
            "pkg2": ["pkg4"],
            "pkg3": ["pkg4"],
            "pkg4": [],
        }
        changed_files = {"pkg1"}
        expected_impacted_packages = {"pkg1", "pkg2", "pkg3", "pkg4"}
        self.assertEqual(find_impacted_packages(dependencies, changed_files), expected_impacted_packages)

    def test_impacted_packages_3(self):
        dependencies = {
            "pkg1": ["pkg2"],
            "pkg2": ["pkg1"],
        }
        changed_files = {"pkg1"}
        expected_impacted_packages = {"pkg1", "pkg2"}
        self.assertEqual(find_impacted_packages(dependencies, changed_files), expected_impacted_packages)

    def test_impacted_packages_4(self):
        dependencies = {
            "pkg1": ["pkg2"],
            "pkg2": ["pkg3"],
            "pkg3": [],
        }
        changed_files = {"pkg3"}
        expected_impacted_packages = {"pkg3"}
        self.assertEqual(find_impacted_packages(dependencies, changed_files), expected_impacted_packages)

    @patch("tasks.gotest._get_release_json_value", new=MagicMock())
    @patch("tasks.gotest.get_modified_files", new=MagicMock(return_value=["pkg/foo.go", "pkg/bar.go"]))
    def test_should_run_all_tests_1(self):
        trigger_files = ["pkg/foo.go"]

        self.assertTrue(should_run_all_tests(None, trigger_files))

    @patch("tasks.gotest._get_release_json_value", new=MagicMock())
    @patch("tasks.gotest.get_modified_files", new=MagicMock(return_value=["pkg/toto/bar.go"]))
    def test_should_run_all_tests_2(self):
        trigger_files = ["pkg/*"]

        self.assertTrue(should_run_all_tests(None, trigger_files))

    @patch("tasks.gotest._get_release_json_value", new=MagicMock())
    @patch("tasks.gotest.get_modified_files", new=MagicMock(return_value=["pkg/foo.go", "pkg/bar.go"]))
    def test_should_run_all_tests_3(self):
        trigger_files = ["pkg/toto/bar.go"]

        self.assertFalse(should_run_all_tests(None, trigger_files))

    @patch("tasks.gotest._get_release_json_value", new=MagicMock())
    @patch("tasks.gotest.get_modified_files", new=MagicMock(return_value=["pkg/foo.go", "pkg/bar.go"]))
    def test_should_run_all_tests_4(self):
        trigger_files = ["pkgs/*"]

        self.assertFalse(should_run_all_tests(None, trigger_files))
