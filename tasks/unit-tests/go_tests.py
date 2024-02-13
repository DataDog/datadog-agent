import unittest

from tasks.go_test import find_impacted_packages


class TestUtils(unittest.TestCase):
    def test_find_impacted_packages(self):
        dependencies = {
            "pkg1": ["pkg2", "pkg3"],
            "pkg2": ["pkg3"],
            "pkg3": [],
        }
        changed_files = {"pkg1"}
        expected_impacted_packages = {"pkg1", "pkg2", "pkg3"}
        self.assertEqual(find_impacted_packages(dependencies, changed_files), expected_impacted_packages)

    def test_impacted_packages(self):
        dependencies = {
            "pkg1": ["pkg3"],
            "pkg2": ["pkg3"],
            "pkg3": [],
        }
        changed_files = {"pkg1"}
        expected_impacted_packages = {"pkg1", "pkg3"}
        self.assertEqual(find_impacted_packages(dependencies, changed_files), expected_impacted_packages)
