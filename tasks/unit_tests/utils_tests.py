import os
import unittest

from tasks.libs.common.utils import clean_nested_paths, environ
from tasks.libs.package.utils import get_package_name


class TestUtils(unittest.TestCase):
    def test_clean_nested_paths_1(self):
        paths = [
            "./pkg/utils/toto",
            "./pkg/utils/",
            "./pkg",
            "./toto/pkg",
            "./pkg/utils/tata",
            "./comp",
            "./component",
            "./comp/toto",
        ]
        expected_paths = ["./comp", "./component", "./pkg", "./toto/pkg"]
        self.assertEqual(clean_nested_paths(paths), expected_paths)

    def test_clean_nested_paths_2(self):
        paths = [
            ".",
            "./pkg/utils/toto",
            "./pkg/utils/",
            "./pkg",
            "./toto/pkg",
            "./pkg/utils/tata",
            "./comp",
            "./component",
            "./comp/toto",
        ]
        expected_paths = ["."]
        self.assertEqual(clean_nested_paths(paths), expected_paths)


class TestEnviron(unittest.TestCase):
    def test_restores_var_after_exception(self):
        """A raise inside the `with` block must not leave the deleted/overridden var stuck."""
        os.environ["SOME_TEST_VAR"] = "original"
        try:
            with self.assertRaises(RuntimeError):
                with environ({"SOME_TEST_VAR": "DELETE"}):
                    self.assertNotIn("SOME_TEST_VAR", os.environ)
                    raise RuntimeError("boom")
            self.assertEqual(os.environ.get("SOME_TEST_VAR"), "original")
        finally:
            os.environ.pop("SOME_TEST_VAR", None)

    def test_sequential_delete_after_exception_does_not_raise(self):
        """A second `environ()` DELETE call must not KeyError once the first one restored properly."""
        os.environ["SOME_TEST_VAR"] = "original"
        try:
            with self.assertRaises(RuntimeError):
                with environ({"SOME_TEST_VAR": "DELETE"}):
                    raise RuntimeError("boom")

            with environ({"SOME_TEST_VAR": "DELETE"}):
                self.assertNotIn("SOME_TEST_VAR", os.environ)
            self.assertEqual(os.environ.get("SOME_TEST_VAR"), "original")
        finally:
            os.environ.pop("SOME_TEST_VAR", None)


class TestGetPackageName(unittest.TestCase):
    def test_get_package_name_no_flavor(self):
        """Test get_package_name with no flavor (empty string)"""
        binary = "agent"
        flavor = ""

        result = get_package_name(binary, flavor)

        self.assertEqual(result, "datadog-agent")

    def test_get_package_name_with_flavor(self):
        """Test get_package_name with a flavor specified"""
        binary = "agent"
        flavor = "iot"

        result = get_package_name(binary, flavor)

        self.assertEqual(result, "datadog-iot-agent")

    def test_get_package_name_different_binary(self):
        """Test get_package_name with a different binary name"""
        binary = "dogstatsd"
        flavor = ""

        result = get_package_name(binary, flavor)

        self.assertEqual(result, "datadog-dogstatsd")
