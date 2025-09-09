"""
Unit tests for dynamic test executor
"""

import unittest
from unittest.mock import MagicMock

from tasks.libs.dynamic_test.executor import DynTestExecutor
from tasks.libs.dynamic_test.index import DynamicTestIndex, IndexKind


class TestDynTestExecutor(unittest.TestCase):
    def test_get_tests_to_run_with_matching_packages(self):
        # Arrange: mock modified packages per module

        ctx = MagicMock()
        ctx.run.return_value.ok = True
        backend = MagicMock()
        backend.list_indexed_keys.return_value = ["commit_sha"]
        executor = DynTestExecutor(ctx, IndexKind.PACKAGE, backend, "commit_sha")

        # The executor expects an index mapping job_name -> package -> [tests]
        executor._index = DynamicTestIndex(
            data={
                "test_job": {
                    "pkg/collector/python/aggregator.go": ["test_test"],
                }
            }
        )

        # Act
        tests_to_run = executor.tests_to_run("test_job", ["pkg/collector/python/aggregator.go"])

        # Assert
        self.assertEqual(tests_to_run, {"test_test"})

    def test_get_tests_to_run_with_no_matching_packages(self):
        # Arrange: modified packages that don't exist in the index

        ctx = MagicMock()
        ctx.run.return_value.ok = True
        backend = MagicMock()
        backend.list_indexed_keys.return_value = ["commit_sha"]
        executor = DynTestExecutor(ctx, IndexKind.PACKAGE, backend, "commit_sha")
        executor._index = DynamicTestIndex(
            data={
                "test_job": {
                    "pkg/collector/python": ["test_test"],
                }
            }
        )

        # Act
        tests_to_run = executor.tests_to_run("test_job", ["pkg/collector/python/aggregator.go"])

        # Assert
        self.assertEqual(tests_to_run, set())

    def test_get_tests_to_run_per_job(self):
        # Arrange

        ctx = MagicMock()
        ctx.run.return_value.ok = True
        backend = MagicMock()
        backend.list_indexed_keys.return_value = ["commit_sha"]
        executor = DynTestExecutor(ctx, IndexKind.PACKAGE, backend, "commit_sha")
        executor._index = DynamicTestIndex(
            data={
                "test_job": {
                    "pkg/collector/python": ["test_test"],
                },
                "other_job": {
                    "pkg/util/log": ["test_other"],
                },
            }
        )
        # Act
        result = executor.tests_to_run_per_job(["pkg/collector/python", "pkg/util/log"])

        # Assert
        self.assertEqual(
            result,
            {
                "test_job": {"test_test"},
                "other_job": {"test_other"},
            },
        )


if __name__ == "__main__":
    unittest.main()
