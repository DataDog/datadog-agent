"""
Unit tests for E2EDynTestExecutor
"""

import json
import os
import tempfile
import unittest
from unittest.mock import Mock, patch, MagicMock
from datetime import datetime, timezone

from tasks.libs.dynamic_test.executor import E2EDynTestExecutor


class TestE2EDynTestExecutor(unittest.TestCase):
    """Test cases for E2EDynTestExecutor class"""
    def setUp(self):
        self.executor = E2EDynTestExecutor("unknown_s3")
        self.executor.index = json.load(open("tasks/unit_tests/libs/dynamic_test/fixtures/index.json"))
        self.ctx = Mock()


    @patch('tasks.libs.dynamic_test.executor.get_modified_packages')
    def test_get_tests_to_run(self, mock_get_modified_packages):
        """Test the get_tests_to_run method"""
        mock_get_modified_packages.return_value = {
            "./.": ["pkg/collector/python"],
            "pkg/util/log": ["./."]
        }


        tests_to_run = self.executor.get_tests_to_run(self.ctx, "test_job")
        self.assertEqual(tests_to_run, ["TestLog"])

    @patch('tasks.libs.dynamic_test.executor.get_modified_packages')
    def test_get_modified_packages(self, mock_get_modified_packages):
        """Test the get_modified_packages method"""
        mock_get_modified_packages.return_value = {
            "./.": ["pkg/collector/python"],
            "pkg/util/log": ["./."]
        }

        modified_packages = self.executor._get_modified_packages(self.ctx)
        self.assertEqual(modified_packages, ["pkg/collector/python", "pkg/util/log"])
