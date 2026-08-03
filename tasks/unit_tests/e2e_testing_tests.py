import datetime
import os
import unittest
from tempfile import TemporaryDirectory
from unittest.mock import MagicMock, patch

from tasks.libs.testing.e2e import create_test_selection_gotest_regex, filter_only_leaf_tests
from tasks.new_e2e_tests import (
    DEFAULT_GO_TEST_TIMEOUT,
    GO_TEST_MIN_TIMEOUT_SECONDS,
    _compute_go_test_timeout,
    _format_go_duration,
    post_process_output,
    pretty_print_logs,
    write_result_to_log_files,
)


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


class TestFilterOnlyLeafTests(unittest.TestCase):
    def test_basic(self):
        tests = {
            ("mypackage", "TestParent"),
            ("mypackage", "TestParent/Child1"),
            ("mypackage", "TestParent/Child2"),
            ("mypackage", "TestParent/SubParent"),
            ("mypackage", "TestParent/SubParent/GrandChild"),
        }

        leaf_tests = filter_only_leaf_tests(tests)
        expected_leaf_tests = {
            ("mypackage", "TestParent/Child1"),
            ("mypackage", "TestParent/Child2"),
            ("mypackage", "TestParent/SubParent/GrandChild"),
        }
        self.assertSetEqual(leaf_tests, expected_leaf_tests)

    def test_multiple_packages(self):
        tests = {
            ("mypackage", "TestParent"),
            ("mypackage", "TestParent/Child"),
            ("otherpackage", "TestParent"),
            ("otherpackage", "TestParent/Child"),
        }
        leaf_tests = filter_only_leaf_tests(tests)
        expected_leaf_tests = {
            ("mypackage", "TestParent/Child"),
            ("otherpackage", "TestParent/Child"),
        }
        self.assertSetEqual(leaf_tests, expected_leaf_tests)

    def test_deep_hierarchy(self):
        tests = {
            ("mypackage", "TestParent"),
            ("mypackage", "TestParent/Child1"),
            ("mypackage", "TestParent/Child1/GrandChild"),
            ("mypackage", "TestParent/Child1/GrandChild/GrandGrandChild"),
            ("mypackage", "TestParent/Child2"),
            ("mypackage", "TestParent/Child3"),
            ("mypackage", "TestParent/Child3/GrandChild"),
        }
        leaf_tests = filter_only_leaf_tests(tests)
        expected_leaf_tests = {
            ("mypackage", "TestParent/Child1/GrandChild/GrandGrandChild"),
            ("mypackage", "TestParent/Child3/GrandChild"),
            ("mypackage", "TestParent/Child2"),
        }
        self.assertSetEqual(leaf_tests, expected_leaf_tests)


class TestCreateTestSelectionGotestRegex(unittest.TestCase):
    def test_empty(self):
        self.assertEqual(create_test_selection_gotest_regex([]), "")

    def test_single(self):
        self.assertEqual(create_test_selection_gotest_regex(["TestFoo"]), '"^(?:TestFoo)$"')

    def test_multiple_flat(self):
        self.assertEqual(create_test_selection_gotest_regex(["TestFoo", "TestBar"]), '"^(?:TestBar|TestFoo)$"')

    def test_nested(self):
        self.assertEqual(
            create_test_selection_gotest_regex(["TestFoo", "TestBar/Baz"]), '"^(?:TestBar|TestFoo)$/^(?:Baz)$"'
        )

    def test_multiple_nested(self):
        self.assertEqual(
            create_test_selection_gotest_regex(["TestFoo", "TestBar/Ba", "TestBar/Baz"]),
            '"^(?:TestBar|TestFoo)$/^(?:Ba|Baz)$"',
        )

    def test_deep_nesting(self):
        self.assertEqual(
            create_test_selection_gotest_regex(["TestA/B/C", "TestA/B/D", "TestX/Y"]),
            '"^(?:TestA|TestX)$/^(?:B|Y)$/^(?:C|D)$"',
        )

    def test_segments_with_overlap(self):
        self.assertEqual(
            create_test_selection_gotest_regex(["TestA/B", "TestA/C", "TestB/B"]), '"^(?:TestA|TestB)$/^(?:B|C)$"'
        )


class TestComputeGoTestTimeout(unittest.TestCase):
    _CI_VARS = ("CI_JOB_TIMEOUT", "CI_JOB_STARTED_AT")

    def setUp(self):
        self._saved = {var: os.environ.pop(var, None) for var in self._CI_VARS}

    def tearDown(self):
        for var, value in self._saved.items():
            if value is None:
                os.environ.pop(var, None)
            else:
                os.environ[var] = value

    @staticmethod
    def _utc(year, month, day, hour=0, minute=0, second=0):
        return datetime.datetime(year, month, day, hour, minute, second, tzinfo=datetime.timezone.utc)

    def test_explicit_argument_wins_over_env(self):
        os.environ["CI_JOB_TIMEOUT"] = "7200"
        self.assertEqual(_compute_go_test_timeout("30m"), "30m")

    def test_ci_env_no_started_at_uses_full_budget(self):
        os.environ["CI_JOB_TIMEOUT"] = "7200"
        # Without CI_JOB_STARTED_AT, elapsed is treated as 0.
        self.assertEqual(_compute_go_test_timeout(""), "1h55m0s")

    def test_ci_env_with_started_at_subtracts_elapsed(self):
        os.environ["CI_JOB_TIMEOUT"] = "7200"
        os.environ["CI_JOB_STARTED_AT"] = "2026-05-12T10:00:00Z"
        now = self._utc(2026, 5, 12, 10, 10, 0)  # 10 min elapsed
        # 7200 - 600 - 300 = 6300s = 1h45m
        self.assertEqual(_compute_go_test_timeout("", now=now), "1h45m0s")

    def test_remaining_too_small_clamps_to_minimum(self):
        os.environ["CI_JOB_TIMEOUT"] = "60"
        result = _compute_go_test_timeout("")
        # 60s budget - 300s buffer = -240s → clamped to GO_TEST_MIN_TIMEOUT_SECONDS
        self.assertEqual(result, _format_go_duration(GO_TEST_MIN_TIMEOUT_SECONDS))

    def test_ci_env_not_integer_falls_back_to_default(self):
        os.environ["CI_JOB_TIMEOUT"] = "not-a-number"
        self.assertEqual(_compute_go_test_timeout(""), DEFAULT_GO_TEST_TIMEOUT)

    def test_invalid_started_at_treated_as_no_elapsed(self):
        os.environ["CI_JOB_TIMEOUT"] = "7200"
        os.environ["CI_JOB_STARTED_AT"] = "not-a-date"
        self.assertEqual(_compute_go_test_timeout(""), "1h55m0s")

    def test_no_ci_env_returns_default(self):
        self.assertEqual(_compute_go_test_timeout(""), DEFAULT_GO_TEST_TIMEOUT)

    def test_format_go_duration(self):
        self.assertEqual(_format_go_duration(6900), "1h55m0s")
        self.assertEqual(_format_go_duration(60), "0h1m0s")
        self.assertEqual(_format_go_duration(0), "0h0m0s")
        self.assertEqual(_format_go_duration(-10), "0h0m0s")
