import json
import unittest
from unittest import mock

import requests

from ..update_python3 import filter_versions, list_available_versions, max_version

GHA_SUPPORTED_PLATFORMS = [("win32", "x64"), ("darwin", "x64"), ("darwin", "arm64")]

with open("tasks/unit-tests/testdata/mocked-versions-manifest.json", 'r') as f:
    MOCKED_VERSIONS_MANIFEST = json.load(f)


def mocked_requests_get(*args):
    """
    This method will be used by the mock to replace requests.get
    """

    class MockResponse(requests.models.Response):
        def __init__(self, json_data, status_code):
            self.json_data = json_data
            self.status_code = status_code

        def json(self):
            return self.json_data

    if args[0] == 'https://raw.githubusercontent.com/actions/python-versions/main/versions-manifest.json':
        return MockResponse(MOCKED_VERSIONS_MANIFEST, 200)
    return MockResponse(None, 404)


class TestLicensesMethod(unittest.TestCase):
    def test_filter_versions(self):
        versions_list = ['3.13.0-alpha.1', '3.12.0', '3.12.0-rc.3', '3.11.5', '3.10.11']
        self.assertEqual(filter_versions(versions_list), versions_list)
        self.assertEqual(filter_versions(versions_list, majmin='3.12'), ['3.12.0-rc.3', '3.12.0'])
        with self.assertRaises(ValueError) as cm:
            filter_versions(versions_list, majmin='3.13.0-alpha.1')
        self.assertEqual("The majmin argument's format should be x.y, '3.13.0-alpha.1' is x.y.z", str(cm.exception))
        with self.assertRaises(ValueError) as cm:
            filter_versions(versions_list, majmin='aaa')
        self.assertEqual("The majmin 'aaa' format should be x.y", str(cm.exception))

    def test_max_version(self):
        self.assertEqual(max_version(['3.12.0', '3.12.0-rc.3', '3.11.5', '3.10.11']), '3.12.0')
        self.assertEqual(max_version(['3.12.0', '3.2.0']), '3.12.0')

    @mock.patch('requests.get', side_effect=mocked_requests_get)
    def test_list_available_versions(self, _mock_get):
        self.assertEqual(list_available_versions(), ['3.13.0-alpha.1', '3.12.0'])
        self.assertEqual(
            list_available_versions(platforms=[('win32', "x64")]), ['3.13.0-alpha.1', '3.12.0', '3.12.0-rc.3']
        )
        self.assertEqual(list_available_versions(platforms=[('win32', "x64")], keep_stable_only=True), ['3.12.0'])


if __name__ == '__main__':
    unittest.main()
