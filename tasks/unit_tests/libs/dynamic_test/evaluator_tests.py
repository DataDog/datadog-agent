import unittest
from types import SimpleNamespace
from unittest.mock import MagicMock, patch

from tasks.libs.dynamic_test.evaluator import (
    DatadogDynTestEvaluator,
    DynTestEvaluator,
    ExecutedTest,
    pipeline_jobs_allowed_to_fail,
)
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
        # the allow-failure fetch is networked; the evaluator tests are not
        # about it (see test_unreliable_jobs_fetch_fails_open)
        self.evaluator._unreliable_jobs = set()

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

    def test_jobs_allowed_to_fail_never_miss(self):
        """Failing tests in jobs the pipeline allows to fail are not critical
        misses: GitLab ignores those jobs' result, so a failure there is
        known-unreliable."""
        self.executor.tests_to_run_per_job.return_value = {"job": set()}
        # the cached allow-failure set, as a fetch from GitLab would fill it
        self.evaluator._unreliable_jobs = {"job"}
        self.assertTrue(self.evaluator.initialize())
        result = self.evaluator.evaluate([])[0]
        self.assertEqual(result.not_executed_failing_tests, set())
        self.assertEqual(result.actual_executed_tests, {"TestPass", "TestFail", "TestFlaky"})

    @patch("tasks.libs.dynamic_test.evaluator.pipeline_jobs_allowed_to_fail")
    def test_unreliable_jobs_fetch_fails_open(self, allowed):
        """If the pipeline jobs cannot be fetched, nothing is filtered (the
        misses stay visible) instead of hiding them all."""
        evaluator = _FakeEvaluator(MagicMock(), IndexKind.PACKAGE, self.executor, "42", telemetry_handler=MagicMock())
        evaluator._unreliable_jobs = None  # not fetched yet
        allowed.side_effect = RuntimeError("gitlab down")
        self.assertEqual(evaluator.unreliable_jobs, set())
        # and the failure is cached, not retried per job
        allowed.side_effect = None
        self.assertEqual(evaluator.unreliable_jobs, set())

    @patch("tasks.libs.dynamic_test.evaluator.get_pipeline")
    def test_pipeline_jobs_allowed_to_fail(self, get_pipeline):
        """Allow-failure job names across the success and failed scopes."""
        pipeline = get_pipeline.return_value
        pipeline.jobs.list.side_effect = [
            iter(
                [
                    SimpleNamespace(name="new-e2e-ok", allow_failure=False),
                    SimpleNamespace(name="new-e2e-allowed", allow_failure=True),
                ]
            ),
            iter(
                [
                    SimpleNamespace(name="new-e2e-failed-allowed", allow_failure=True),
                    SimpleNamespace(name="new-e2e-failed", allow_failure=False),
                ]
            ),
        ]
        self.assertEqual(pipeline_jobs_allowed_to_fail("42"), {"new-e2e-allowed", "new-e2e-failed-allowed"})
        self.assertEqual([c.kwargs["scope"] for c in pipeline.jobs.list.call_args_list], ["success", "failed"])

    def test_summary_lists_the_missed_failing_tests(self):
        """The global summary warning names the skipped failing tests and their
        jobs: the per-job reports are long, the summary is what gets read."""
        self.executor.tests_to_run_per_job.return_value = {"job": set()}
        self.assertTrue(self.evaluator.initialize())
        with patch("builtins.print") as output:
            self.evaluator.print_summary(self.evaluator.evaluate([]))
        printed = [str(call) for call in output.call_args_list]
        self.assertTrue(any("1 failing tests would not have been executed" in p for p in printed))
        self.assertTrue(any("TestFail" in p and "job" in p for p in printed))

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
    def event(name, status="pass", flaky=False, job_id="7", job_name="job"):
        return {
            "attributes": {
                "attributes": {
                    "test": {"name": name, "status": status, "agent_is_flaky_failure": flaky},
                    "ci": {"job": {"id": job_id, "name": job_name}, "pipeline": {"id": "42"}},
                }
            }
        }

    @patch("tasks.libs.dynamic_test.evaluator.get_ci_test_events")
    def test_query_environment_window_and_filtering(self, events):
        events.return_value = [
            self.event("TestPass"),
            self.event("TestFail", "fail"),
            self.event("TestSkip", "skip"),
            self.event("TestRoot/Subtest", "fail"),
            self.event("", "fail"),
            self.event("TestFlaky", "fail", "true"),
            self.event("TestFlakyBool", "fail", True),
        ]
        evaluator = DatadogDynTestEvaluator(
            MagicMock(),
            IndexKind.JEV,
            MagicMock(),
            "42",
            telemetry_handler=MagicMock(),
        )
        tests = evaluator.list_tests_for_job('job: ["matrix"]')
        query, days = events.call_args.args
        # The env facet is not part of the query (pipeline + job identify),
        # and the window is main's hardcoded 3 days
        self.assertNotIn("env:", query)
        self.assertIn("@ci.pipeline.id:42", query)
        self.assertIn(r'@ci.job.name:"job: [\"matrix\"]"', query)
        # Skipped tests are excluded by the query itself
        self.assertIn("-@test.status:skip", query)
        self.assertEqual(days, 3)
        self.assertEqual([test.name for test in tests], ["TestPass", "TestFail", "TestFlaky", "TestFlakyBool"])
        self.assertFalse(tests[1].unreliable_status)
        self.assertTrue(tests[2].unreliable_status)
        self.assertTrue(tests[3].unreliable_status)

    @patch("tasks.libs.dynamic_test.evaluator.get_ci_test_events")
    def test_passed_on_retry_counts_as_success(self, events):
        """A GitLab job retry reruns the whole test set: the failed attempt and
        the successful retry carry the same job name but different job ids.
        One success in the attempts means the test passed - it is not a
        critical miss."""
        events.return_value = [
            self.event("TestRetry", "fail", job_id="7"),  # first attempt
            self.event("TestRetry", "pass", job_id="8"),  # retried job
        ]
        evaluator = DatadogDynTestEvaluator(
            MagicMock(), IndexKind.JEV, MagicMock(), "42", telemetry_handler=MagicMock()
        )
        tests = evaluator.list_tests_for_job("job")
        self.assertEqual(len(tests), 1)
        self.assertEqual(tests[0].status, "pass")

        # ...and a test that failed every attempt still fails
        events.return_value = [
            self.event("TestHardFail", "fail", job_id="7"),
            self.event("TestHardFail", "fail", job_id="8"),
        ]
        tests = evaluator.list_tests_for_job("job")
        self.assertEqual(len(tests), 1)
        self.assertEqual(tests[0].status, "fail")

    @patch("tasks.libs.dynamic_test.evaluator.get_ci_test_events")
    def test_pass_in_another_job_is_not_a_retry(self, events):
        """A pass in a DIFFERENT job (e.g. the same test selected by another
        e2e job) does not excuse this job's failure."""
        events.return_value = [
            self.event("TestDup", "fail"),
            self.event("TestDup", "pass", job_name="other-e2e-job"),
        ]
        evaluator = DatadogDynTestEvaluator(
            MagicMock(), IndexKind.JEV, MagicMock(), "42", telemetry_handler=MagicMock()
        )
        tests = evaluator.list_tests_for_job("job")
        fail = [t for t in tests if t.job_name == "job"][0]
        self.assertEqual(fail.status, "fail")

    @patch("tasks.libs.dynamic_test.evaluator.get_ci_test_events")
    def test_flaky_marking_on_any_attempt_wins(self, events):
        """CI Vis marks a flaky failure on the attempt it saw - keep the
        unreliable marking no matter which attempt carried it."""
        events.return_value = [
            self.event("TestFlakyRetry", "fail"),
            self.event("TestFlakyRetry", "fail", flaky=True, job_id="8"),
        ]
        evaluator = DatadogDynTestEvaluator(
            MagicMock(), IndexKind.JEV, MagicMock(), "42", telemetry_handler=MagicMock()
        )
        tests = evaluator.list_tests_for_job("job")
        self.assertEqual(tests[0].status, "fail")
        self.assertTrue(tests[0].unreliable_status)

    @patch("tasks.libs.dynamic_test.evaluator.get_ci_test_events")
    def test_coverage_defaults(self, events):
        events.return_value = [self.event("TestFail", "fail")]
        evaluator = DatadogDynTestEvaluator(
            MagicMock(), IndexKind.FILE, MagicMock(), "42", telemetry_handler=MagicMock()
        )
        self.assertFalse(evaluator.list_tests_for_job("job")[0].unreliable_status)
        query, days = events.call_args.args
        self.assertNotIn("env:", query)
        self.assertEqual(days, 3)
