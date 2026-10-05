import unittest
from unittest.mock import MagicMock, patch

from tasks.libs.dynamic_test.evaluator import DatadogDynTestEvaluator, DynTestEvaluator, ExecutedTest
from tasks.libs.dynamic_test.index import DynamicTestIndex, IndexKind
from tasks.libs.dynamic_test.telemetry import ConsoleTelemetryHandler


class _FakeEvaluator(DynTestEvaluator):
    def list_tests_for_job(self, job_name):
        return [
            ExecutedTest("TestPass", "pass", "42", "1", job_name, False),
            ExecutedTest("TestFail", "fail", "42", "1", job_name, False),
            ExecutedTest("TestFlaky", "fail", "42", "1", job_name, True),
            ExecutedTest("TestNotIndexed", "fail", "42", "1", job_name, False),
        ]


class TestDynTestEvaluator(unittest.TestCase):
    def setUp(self):
        self.executor = MagicMock()
        self.executor.commit_sha = "commit123"
        self.index = DynamicTestIndex({"job": {"pkg": ["TestPass", "TestFail", "TestFlaky"]}})
        self.executor.index.return_value = self.index
        self.telemetry = MagicMock()
        self.evaluator = _FakeEvaluator(
            MagicMock(), IndexKind.PACKAGE, self.executor, "42", telemetry_handler=self.telemetry
        )

    def test_initialize_success(self):
        self.assertTrue(self.evaluator.initialize())
        self.executor.init_index.assert_called_once_with()
        self.assertIs(self.evaluator.index, self.index)
        self.telemetry.send_event.assert_called_once()
        self.assertEqual(self.telemetry.send_event.call_args.args[0].alert_type, "success")

    def test_initialize_errors(self):
        for error, error_type in [
            (RuntimeError("No ancestor commit found for commit123"), "index_not_found"),
            (RuntimeError("Backend connection failed"), "index_initialization_failed"),
            (ValueError("Unexpected value"), "unexpected_error"),
        ]:
            with self.subTest(error_type=error_type):
                self.telemetry.reset_mock()
                self.executor.init_index.side_effect = error
                self.assertFalse(self.evaluator.initialize())
                event = self.telemetry.send_event.call_args.args[0]
                self.assertEqual(event.title, f"Dynamic Test Evaluator Error: {error_type}")
                self.assertIn("commit_sha:commit123", event.tags)

    def test_failed_telemetry_falls_back_to_console(self):
        self.executor.init_index.side_effect = RuntimeError("offline")
        self.telemetry.send_event.return_value = False
        with patch("builtins.print") as output:
            self.assertFalse(self.evaluator.initialize())
        self.assertTrue(any("offline" in str(call) for call in output.call_args_list))

    def test_misses_and_unindexed_tests(self):
        self.executor.tests_to_run_per_job.return_value = {"job": {"TestPass", "Unrelated"}}
        self.assertTrue(self.evaluator.initialize())
        result = self.evaluator.evaluate([])[0]
        self.assertEqual(result.actual_executed_tests, {"TestPass", "TestFail", "TestFlaky"})
        self.assertEqual(result.predicted_executed_tests, {"TestPass"})
        self.assertEqual(result.not_executed_failing_tests, {"TestFail"})

    def test_console_telemetry_never_publishes(self):
        with (
            patch("tasks.libs.dynamic_test.telemetry.send_event") as event,
            patch("tasks.libs.dynamic_test.telemetry.send_metrics") as metrics,
        ):
            self.evaluator.telemetry_handler = ConsoleTelemetryHandler()
            self.assertTrue(self.evaluator.initialize())
            self.executor.tests_to_run_per_job.return_value = {}
            self.evaluator.send_stats_to_datadog(self.evaluator.evaluate([]))
            event.assert_not_called()
            metrics.assert_not_called()


class TestDatadogDynTestEvaluator(unittest.TestCase):
    @staticmethod
    def event(name, status="pass", flaky=False):
        return {
            "attributes": {
                "attributes": {
                    "test": {"name": name, "status": status, "agent_is_flaky_failure": flaky},
                    "ci": {"job": {"id": "7", "name": "job"}, "pipeline": {"id": "42"}},
                }
            }
        }

    @patch("tasks.libs.dynamic_test.evaluator.get_ci_test_events")
    def test_query_environment_window_job_attempt_and_filtering(self, events):
        events.return_value = [
            self.event("TestPass"),
            self.event("TestFail", "fail"),
            self.event("TestSkip", "skip"),
            self.event("TestRoot/Subtest", "fail"),
            self.event("", "fail"),
            self.event("TestFlaky", "fail", "true"),
            self.event("TestFlakyBool", "fail", True),
        ]
        ids = {}
        evaluator = DatadogDynTestEvaluator(
            MagicMock(),
            IndexKind.JEV,
            MagicMock(),
            "42",
            telemetry_handler=MagicMock(),
            test_env="nativetest",
            lookback_days=7,
            job_ids=ids,
        )
        ids['job: ["matrix"]'] = "7"
        tests = evaluator.list_tests_for_job('job: ["matrix"]')
        query, days = events.call_args.args
        self.assertIn("env:nativetest", query)
        self.assertIn("@ci.pipeline.id:42", query)
        self.assertIn("@ci.job.id:7", query)
        self.assertIn(r'@ci.job.name:"job: [\"matrix\"]"', query)
        self.assertEqual(days, 7)
        self.assertEqual([test.name for test in tests], ["TestPass", "TestFail", "TestFlaky", "TestFlakyBool"])
        self.assertFalse(tests[1].unreliable_status)
        self.assertTrue(tests[2].unreliable_status)
        self.assertTrue(tests[3].unreliable_status)

    @patch("tasks.libs.dynamic_test.evaluator.get_ci_test_events")
    def test_allow_failure_and_coverage_defaults(self, events):
        events.return_value = [self.event("TestFail", "fail")]
        unreliable = set()
        evaluator = DatadogDynTestEvaluator(
            MagicMock(), IndexKind.FILE, MagicMock(), "42", telemetry_handler=MagicMock(), unreliable_jobs=unreliable
        )
        unreliable.add("job")
        self.assertTrue(evaluator.list_tests_for_job("job")[0].unreliable_status)
        query, days = events.call_args.args
        self.assertIn("env:prod", query)
        self.assertNotIn("@ci.job.id:", query)
        self.assertEqual(days, 3)
