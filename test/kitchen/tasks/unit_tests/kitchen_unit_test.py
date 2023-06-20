import os
import unittest

from invoke import Exit, MockContext

from ..kitchen import should_rerun_failed


class TestKitchenInvokeMethod(unittest.TestCase):
    def test_no_rerun_gotest(self):
        mock_context = MockContext()
        with self.assertRaises(Exit):
            should_rerun_failed(mock_context, "tasks/unit_tests/gotest-failed-runlog", "gotest")

    def test_rerun_gotest(self):
        mock_context = MockContext()
        try:
            should_rerun_failed(mock_context, "tasks/unit_tests/gotest-infra-failed-runlog", "gotest")
        except Exit:
            self.fail("should_rerun_failed returned non-zero exit code")

    def test_no_rerun_rspec(self):
        mock_context = MockContext()
        with self.assertRaises(Exit):
            should_rerun_failed(mock_context, "tasks/unit_tests/test-failed-runlog")

    def test_rerun_rspec(self):
        mock_context = MockContext()
        try:
            should_rerun_failed(mock_context, "tasks/unit_tests/infra-failed-runlog")
        except Exit:
            self.fail("should_rerun_failed returned non-zero exit code")


if __name__ == '__main__':
    unittest.main()
