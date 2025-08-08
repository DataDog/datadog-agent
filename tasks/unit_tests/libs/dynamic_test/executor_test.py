"""
Unit tests for dynamic test executor
"""

import unittest
from unittest.mock import MagicMock, patch

from tasks.libs.dynamic_test.executor import E2EDynTestExecutor


class TestE2EDynTestExecutor(unittest.TestCase):
    @patch("tasks.libs.dynamic_test.executor.get_modified_packages")
    def test_get_tests_to_run_with_matching_packages(self, mock_get_modified_packages):
        # Arrange: mock modified packages per module
        mock_get_modified_packages.return_value = {
            "./.": ["pkg/collector/python/aggregator.go"],
            "pkg/util/log": ["./."],
        }

        ctx = MagicMock()
        executor = E2EDynTestExecutor(ctx, "s3://unknown-bucket/path")

        # The executor expects an index mapping job_name -> package -> [tests]
        executor.index = {
            "test_job": {
                "pkg/collector/python/aggregator.go": ["test_test"],
            }
        }

        # Act
        tests_to_run = executor.get_tests_to_run("test_job")

        # Assert
        self.assertEqual(tests_to_run, ["test_test"])
        mock_get_modified_packages.assert_called_once_with(ctx)

    @patch("tasks.libs.dynamic_test.executor.get_modified_packages")
    def test_get_tests_to_run_with_no_matching_packages(self, mock_get_modified_packages):
        # Arrange: modified packages that don't exist in the index
        mock_get_modified_packages.return_value = {
            "./.": ["pkg/does/not/match.go"],
        }

        ctx = MagicMock()
        executor = E2EDynTestExecutor(ctx, "s3://unknown-bucket/path")
        executor.index = {
            "test_job": {
                "pkg/collector/python": ["test_test"],
            }
        }

        # Act
        tests_to_run = executor.get_tests_to_run("test_job")

        # Assert
        self.assertEqual(tests_to_run, [])
        mock_get_modified_packages.assert_called_once_with(ctx)

    @patch("tasks.libs.dynamic_test.executor.get_modified_packages")
    def test_get_tests_to_run_per_job(self, mock_get_modified_packages):
        # Arrange
        mock_get_modified_packages.return_value = {
            "./.": ["pkg/collector/python"],
            "pkg/util/log": ["./."],
        }

        ctx = MagicMock()
        executor = E2EDynTestExecutor(ctx, "s3://unknown-bucket/path")
        executor.index = {
            "test_job": {
                "pkg/collector/python": ["test_test"],
            },
            "other_job": {
                "pkg/util/log": ["test_other"],
            },
        }

        # Act
        result = executor.get_tests_to_run_per_job(ctx)

        # Assert
        self.assertEqual(
            result,
            {
                "test_job": ["test_test"],
                "other_job": ["test_other"],
            },
        )
        mock_get_modified_packages.assert_called_once_with(ctx)


if __name__ == "__main__":
    unittest.main()
