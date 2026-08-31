import os
import unittest
from unittest.mock import MagicMock, patch

from tasks.new_e2e_tests import cleanup_remote_stacks
from tasks.tools.e2e_stacks import destroy_remote_stack_api


class TestDestroyRemoteStackApi(unittest.TestCase):
    @patch.dict(
        os.environ,
        {
            "CI_JOB_NAME": "new-e2e-job",
            "CI_JOB_ID": "123",
            "CI_PIPELINE_ID": "456",
            "CI_COMMIT_REF_NAME": "test-branch",
        },
    )
    @patch("tasks.tools.e2e_stacks.run")
    def test_submits_cleanup_with_cancel_first(self, run_mock):
        stack = "organization/e2eci/ci-123-4670-e2e-suite"

        exit_code, stdout, stderr, returned_stack = destroy_remote_stack_api(stack, MagicMock())

        self.assertEqual(exit_code, 0)
        self.assertEqual(stdout, "Stack cleanup request submitted to stackcleaner")
        self.assertEqual(stderr, "")
        self.assertEqual(returned_stack, stack)
        self.assertIn("cancel_first=bool:true", run_mock.call_args.kwargs["attrs"])

    @patch.dict(
        os.environ,
        {
            "CI_JOB_NAME": "new-e2e-job",
            "CI_JOB_ID": "123",
            "CI_PIPELINE_ID": "456",
            "CI_COMMIT_REF_NAME": "test-branch",
        },
    )
    @patch("tasks.tools.e2e_stacks.run", side_effect=RuntimeError("request failed"))
    def test_reports_submission_failure(self, _run_mock):
        stack = "organization/e2eci/ci-123-4670-e2e-suite"

        exit_code, stdout, stderr, returned_stack = destroy_remote_stack_api(stack, MagicMock())

        self.assertEqual(exit_code, 1)
        self.assertEqual(stdout, "")
        self.assertEqual(stderr, "request failed")
        self.assertEqual(returned_stack, stack)


class TestCleanupRemoteStacks(unittest.TestCase):
    @patch.dict(os.environ, {"REMOTE_STACK_CLEANING": "true"})
    @patch("tasks.new_e2e_tests.list_stacks")
    @patch("tasks.new_e2e_tests.multiprocessing.Pool")
    @patch("builtins.print")
    def test_reports_remote_cleanup_as_submitted(self, print_mock, pool_mock, list_stacks_mock):
        stack = "organization/e2eci/ci-123-4670-e2e-suite"
        list_stacks_mock.return_value = [{"name": stack}]
        pool_mock.return_value.__enter__.return_value.map.return_value = [
            (0, "Stack cleanup request submitted to stackcleaner", "", stack)
        ]

        cleanup_remote_stacks.body(MagicMock(), r"^ci-123-4670-.*$")

        print_mock.assert_any_call(f"Stack {stack} cleanup request submitted successfully")
        self.assertNotIn(f"Stack {stack} destroyed successfully", [call.args[0] for call in print_mock.call_args_list])
