import unittest

from tasks.libs.common.utils import clean_nested_paths
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
