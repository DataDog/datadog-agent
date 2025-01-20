import unittest
from unittest.mock import MagicMock, patch

from tasks.new_e2e_tests import post_process_output, pretty_print_logs


class TestE2ETesting(unittest.TestCase):
    @patch("tasks.new_e2e_tests.pretty_print_test_logs")
    @patch("tasks.libs.common.utils.running_in_ci", new=MagicMock(return_value=True))
    def test_pretty_print(self, p):
        flakes_file = "tasks/unit_tests/testdata/flakes_2.yaml"
        path = "tasks/unit_tests/testdata/test_output_failure_marker.json"

        pretty_print_logs(path, post_process_output(path), flakes_file=flakes_file)

        # Failing / flaky, successful / non flaky
        self.assertEqual(p.call_count, 2)
        args1 = p.call_args_list[0][0][0][0]
        args2 = p.call_args_list[1][0][0][0]
        args3 = p.call_args_list[1][0][0][1]
        self.assertEqual(args1[1], "TestGetPayload")
        self.assertEqual(args2[1], "TestGetPayloadContainerized")
        self.assertEqual(args3[1], "TestGetPayloadContainerizedWithDocker0")

    @patch("tasks.new_e2e_tests.pretty_print_test_logs")
    @patch("tasks.libs.common.utils.running_in_ci", new=MagicMock(return_value=True))
    def test_pretty_print2(self, p=None):
        flakes_file = "tasks/unit_tests/testdata/flakes_1.yaml"
        path = "tasks/unit_tests/testdata/test_output_failure_no_marker.json"

        pretty_print_logs(path, post_process_output(path), flakes_file=flakes_file)

        # Failing / flaky, successful / non flaky
        self.assertEqual(p.call_count, 2)
        args1 = p.call_args_list[0][0][0][0]
        args2 = p.call_args_list[1][0][0][0]
        args3 = p.call_args_list[1][0][0][1]
        self.assertEqual(args1[1], "TestGetPayload")
        self.assertEqual(args2[1], "TestFilterDev")
        self.assertEqual(args3[1], "TestGetTimeout")
