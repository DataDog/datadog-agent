import os
import unittest
from tempfile import TemporaryDirectory
from unittest.mock import MagicMock, patch

from tasks.new_e2e_tests import post_process_output, pretty_print_logs, write_result_to_log_files


class TestE2ETesting(unittest.TestCase):
    @patch("tasks.new_e2e_tests.pretty_print_test_logs")
    @patch("tasks.libs.common.utils.running_in_ci", new=MagicMock(return_value=True))
    def test_pretty_print(self, p):
        flakes_file = "tasks/unit_tests/testdata/flakes_2.yaml"
        path = "tasks/unit_tests/testdata/test_output_failure_marker.json"

        pretty_print_logs(path, post_process_output(path), flakes_files=[flakes_file])

        # Failing / flaky, successful / non flaky
        self.assertEqual(p.call_count, 2)
        args1 = p.call_args_list[0][0][0]
        args2 = p.call_args_list[1][0][0]
        self.assertEqual({name for (_, name) in args1.keys()}, {"TestGetPayload"})
        self.assertEqual(
            {name for (_, name) in args2.keys()},
            {"TestGetPayloadContainerized", "TestGetPayloadContainerizedWithDocker0"},
        )

    @patch("tasks.new_e2e_tests.pretty_print_test_logs")
    @patch("tasks.libs.common.utils.running_in_ci", new=MagicMock(return_value=True))
    def test_pretty_print2(self, p=None):
        flakes_file = "tasks/unit_tests/testdata/flakes_1.yaml"
        path = "tasks/unit_tests/testdata/test_output_failure_no_marker.json"

        pretty_print_logs(path, post_process_output(path), flakes_files=[flakes_file])

        # Failing / flaky, successful / non flaky
        self.assertEqual(p.call_count, 2)
        args1 = p.call_args_list[0][0][0]
        args2 = p.call_args_list[1][0][0]
        self.assertEqual({name for (_, name) in args1.keys()}, {"TestGetPayload"})
        self.assertEqual(
            {name for (_, name) in args2.keys()},
            {
                "TestFilterDev",
                "TestAsJSON",
                "TestCollectInfo",
                "TestGetTimeout",
                "TestGetPayloadContainerized",
                "TestGetPayloadContainerizedWithDocker0",
            },
        )

    @patch("tasks.new_e2e_tests.pretty_print_test_logs")
    @patch("tasks.libs.common.utils.running_in_ci", new=MagicMock(return_value=True))
    def test_pretty_print_inner_depth1(self, p=None):
        flakes_file = "tasks/unit_tests/testdata/flakes_1.yaml"
        path = "tasks/unit_tests/testdata/test_output_inner.json"

        pretty_print_logs(path, post_process_output(path), flakes_files=[flakes_file], test_depth=1)

        # Successful / non flaky, successful / flaky
        self.assertEqual(p.call_count, 2)
        args1 = p.call_args_list[0][0][0]
        args2 = p.call_args_list[1][0][0]

        # TestParent contains both flaky and not flaky tests
        self.assertEqual({name for (_, name) in args1.keys()}, {"TestParent"})
        self.assertEqual({name for (_, name) in args2.keys()}, {"TestParent", "TestParentFlaky"})

    @patch("tasks.new_e2e_tests.pretty_print_test_logs")
    @patch("tasks.libs.common.utils.running_in_ci", new=MagicMock(return_value=True))
    def test_pretty_print_inner_depth2(self, p=None):
        flakes_file = "tasks/unit_tests/testdata/flakes_1.yaml"
        path = "tasks/unit_tests/testdata/test_output_inner.json"

        pretty_print_logs(path, post_process_output(path), flakes_files=[flakes_file], test_depth=2)

        # Successful / non flaky, successful / flaky
        self.assertEqual(p.call_count, 2)
        args1 = p.call_args_list[0][0][0]
        args2 = p.call_args_list[1][0][0]

        # Both have at least one test with this state
        self.assertEqual({name for (_, name) in args1.keys()}, {"TestParent", "TestParent/Child"})
        self.assertEqual(
            {name for (_, name) in args2.keys()}, {"TestParentFlaky", "TestParentFlaky/Child", "TestParent/Child2"}
        )


class TestWriteResultToLogFiles(unittest.TestCase):
    def test_depth1(self):
        logs_per_test = [
            ('mypackage', 'garfield', ['line1', 'line2']),
            ('mypackage', 'bd/tomtom', ['line0', 'line1']),
            ('mypackage', 'bd/nana', ['line10', 'line11']),
        ]

        with TemporaryDirectory() as tmpdir:
            write_result_to_log_files(logs_per_test, tmpdir, test_depth=1)

            files = set(os.listdir(tmpdir))
            self.assertSetEqual(files, {'mypackage.garfield.log', 'mypackage.bd.log'})

    def test_depth2(self):
        logs_per_test = [
            ('mypackage', 'garfield', ['line1', 'line2']),
            ('mypackage', 'bd/tomtom', ['line0', 'line1']),
            ('mypackage', 'bd/nana', ['line10', 'line11']),
        ]

        with TemporaryDirectory() as tmpdir:
            write_result_to_log_files(logs_per_test, tmpdir, test_depth=2)

            files = set(os.listdir(tmpdir))
            self.assertSetEqual(files, {'mypackage.garfield.log', 'mypackage.bd_tomtom.log', 'mypackage.bd_nana.log'})
