import unittest
from types import SimpleNamespace
from unittest.mock import MagicMock, patch

from tasks.libs.dynamic_test.evaluator import ExecutedTest
from tasks.libs.dynamic_test.index import DynamicTestIndex, IndexKind
from tasks.libs.dynamic_test.jev_selection import (
    JevDynTestEvaluator,
    JevDynTestExecutor,
    NothingToEvaluateError,
    jev_selection,
)

MODULE = "tasks.libs.dynamic_test.jev_selection"
SHA = "a" * 40


class JevSelectionTests(unittest.TestCase):
    @patch(f"{MODULE}.select_suite")
    def test_selector_runs_in_process_with_overrides(self, select):
        select.return_value = {"run": ["TestA"], "skip": [], "decisions": []}
        with patch.dict("os.environ", {"JEV_DC": "us1.ddbuild.io", "JEV_TOKEN_CMD": "custom-token"}):
            self.assertEqual(jev_selection("installer"), select.return_value)
        # In-process call, no interpreter/module/output-file arguments
        self.assertEqual(select.call_args.args, ("installer",))
        self.assertEqual(select.call_args.kwargs, {"dc": "us1.ddbuild.io", "token_cmd": "custom-token"})

    @patch(f"{MODULE}.select_suite")
    def test_selector_failures_return_no_skip_decisions(self, select):
        for error in (RuntimeError("gateway unreachable"), TimeoutError("request timed out"), ValueError("bad args")):
            select.side_effect = error
            self.assertEqual(jev_selection("fleet"), {})

    @patch(f"{MODULE}.select_suite")
    def test_invalid_selector_summary(self, select):
        for summary in ({"skip": "TestA"}, {"run": ["TestA"], "skip": [], "decisions": "nope"}, None):
            select.return_value = summary
            self.assertEqual(jev_selection("fleet"), {})


def _event(name, job, job_id, status="pass", flaky=False):
    return {
        "attributes": {
            "attributes": {
                "test": {"name": name, "status": status, "agent_is_flaky_failure": flaky},
                "ci": {"job": {"id": job_id, "name": job}, "pipeline": {"id": "42"}},
            }
        }
    }


class JevDynTestExecutorTests(unittest.TestCase):
    @patch(f"{MODULE}.get_ci_test_events", return_value=[])
    @patch(f"{MODULE}.suite_entry_points", return_value={})
    @patch(f"{MODULE}.get_pipeline")
    def test_rejects_wrong_commit_and_empty_pipeline(self, get_pipeline, _, __):
        pipeline = get_pipeline.return_value
        pipeline.sha = "b" * 40
        executor = JevDynTestExecutor(MagicMock(), SHA, "42")
        with self.assertRaisesRegex(RuntimeError, "Pipeline 42 ran"):
            executor.init_index()
        pipeline.jobs.list.assert_not_called()
        pipeline.sha = SHA
        pipeline.jobs.list.side_effect = [iter([]), iter([])]
        with self.assertRaisesRegex(NothingToEvaluateError, "No completed E2E jobs"):
            executor.init_index()

    @patch(f"{MODULE}.get_ci_test_events", return_value=[])
    @patch(f"{MODULE}.suite_entry_points", return_value={})
    @patch(f"{MODULE}.get_pipeline")
    def test_sha_mismatch_can_be_allowed_explicitly(self, get_pipeline, _, __):
        pipeline = get_pipeline.return_value
        pipeline.sha = "b" * 40  # differs from the checkout SHA
        pipeline.jobs.list.side_effect = [
            iter([SimpleNamespace(name="new-e2e-fleet", id=1, allow_failure=False)]),
            iter([]),
        ]
        executor = JevDynTestExecutor(MagicMock(), SHA, "42", require_pipeline_commit=False)
        executor.init_index()
        self.assertEqual(executor.jobs, ["new-e2e-fleet"])

    @patch(f"{MODULE}.suite_entry_points")
    @patch(f"{MODULE}.get_ci_test_events")
    @patch(f"{MODULE}.get_pipeline")
    def test_index_is_the_observed_execution_map(self, get_pipeline, events, suites):
        """One pipeline-wide query builds the index: job -> executed entry points."""
        pipeline = get_pipeline.return_value
        pipeline.sha = SHA
        pipeline.jobs.list.side_effect = [
            iter([SimpleNamespace(name="new-e2e-job-a", id=7, allow_failure=False)]),
            iter(
                [
                    SimpleNamespace(name="new-e2e-job-b", id=8, allow_failure=True),
                    SimpleNamespace(name="unit-tests", id=9, allow_failure=False),
                ]
            ),
        ]
        events.return_value = [
            _event("TestA", "new-e2e-job-a", "7"),
            _event("TestA", "new-e2e-job-a", "6"),  # older attempt of job-a: dropped
            _event("TestB", "new-e2e-job-b", "8", status="fail"),
            _event("TestRoot/Sub", "new-e2e-job-a", "7"),  # subtest: dropped
            _event("TestSkipped", "new-e2e-job-a", "7", status="skip"),  # not executed: dropped
            _event("TestC", "other-job", "99"),  # not a latest e2e job: dropped
            _event("TestNotAnEntry", "new-e2e-job-a", "7"),  # not a filetree entry point: not decidable
        ]
        suites.return_value = {"fleet": {"TestA", "TestB", "TestXOnlyFiletree"}}
        executor = JevDynTestExecutor(MagicMock(), SHA, "42")
        executor.init_index()
        # The query is pipeline-wide: no per-job scoping, no job-name escaping
        query, days = events.call_args.args
        self.assertIn("@ci.pipeline.id:42", query)
        self.assertNotIn("@ci.job.name:", query)
        self.assertEqual(days, 3)
        # The index: only latest-attempt executed entry points, per job
        self.assertEqual(sorted(executor.index().get_jobs()), ["new-e2e-job-a", "new-e2e-job-b"])
        self.assertEqual(executor.index().get_indexed_tests_for_job("new-e2e-job-a"), {"TestA"})
        self.assertEqual(executor.index().get_indexed_tests_for_job("new-e2e-job-b"), {"TestB"})
        # The evaluator's data: statuses and allow-failure marking preserved
        job_a = executor.executed["new-e2e-job-a"]
        self.assertEqual([t.name for t in job_a], ["TestA", "TestNotAnEntry"])
        self.assertTrue(all(t.status == "pass" for t in job_a))
        self.assertTrue(executor.executed["new-e2e-job-b"][0].unreliable_status)
        self.assertFalse(job_a[0].unreliable_status)

    def test_predictions_decide_via_the_suites_that_executed(self):
        executor = JevDynTestExecutor(MagicMock(), SHA, "42")
        executor._index = DynamicTestIndex()
        executor._index.add_tests("job", "executed", {"TestA", "TestDup"})
        with (
            patch(f"{MODULE}.suite_entry_points", return_value={"a": {"TestA", "TestDup"}, "b": {"TestDup"}}),
            patch(
                f"{MODULE}.jev_selection",
                side_effect=[
                    {"run": [], "skip": ["TestA", "TestDup"]},  # suite a: skips both
                    {},  # suite b: selector failure, fail open
                ],
            ),
        ):
            self.assertEqual(executor.tests_to_run_per_job([]), {"job": {"TestDup"}})
            self.assertEqual(executor.tests_to_run("job", []), {"TestDup"})
            self.assertEqual(executor.tests_to_skip("job", []), {"TestA"})
        self.assertEqual(executor.triggering_paths("job", "TestA"), [])


class JevDynTestEvaluatorTests(unittest.TestCase):
    def _executor_with_index(self, executed):
        executor = JevDynTestExecutor(MagicMock(), SHA, "42")
        executor.executed = executed
        executor._index = DynamicTestIndex()
        for job, tests in executed.items():
            executor._index.add_tests(job, "executed", {t.name for t in tests if t.name != "TestNotAnEntry"})
        return executor

    @patch.object(JevDynTestExecutor, "_jev_run", return_value={"TestPass", "TestSkipKeep"})
    def test_base_evaluation_flow_runs_untouched(self, _):
        """The shared evaluate() works against the Jev executor's index."""
        executed = {
            "job": [
                ExecutedTest("TestPass", "pass", "42", "7", "job", False),
                ExecutedTest("TestFail", "fail", "42", "7", "job", False),
                ExecutedTest("TestFlaky", "fail", "42", "7", "job", True),
                ExecutedTest("TestNotAnEntry", "fail", "42", "7", "job", False),
            ]
        }
        executor = self._executor_with_index(executed)
        evaluator = JevDynTestEvaluator(MagicMock(), IndexKind.JEV, executor, "42", telemetry_handler=MagicMock())
        evaluator.index = executor.index()  # normally set by initialize()
        results = evaluator.evaluate([])  # DatadogDynTestEvaluator.evaluate, not overridden
        self.assertEqual(len(results), 1)
        result = results[0]
        self.assertEqual(result.job_name, "job")
        # Actual universe = executed entry points only
        self.assertEqual(result.actual_executed_tests, {"TestPass", "TestFail", "TestFlaky"})
        # TestSkipKeep is predicted but did not run: visible as prediction, not a miss
        self.assertEqual(result.predicted_executed_tests, {"TestPass"})
        # Only reliable failures that Jev would skip count as misses
        self.assertEqual(result.not_executed_failing_tests, {"TestFail"})

    def test_list_tests_for_job_reads_the_executor_index_fetch(self):
        executed = {"job": [ExecutedTest("TestPass", "pass", "42", "7", "job", False)]}
        executor = self._executor_with_index(executed)
        evaluator = JevDynTestEvaluator(MagicMock(), IndexKind.JEV, executor, "42", telemetry_handler=MagicMock())
        self.assertEqual(evaluator.list_tests_for_job("job"), executed["job"])
        self.assertEqual(evaluator.list_tests_for_job("unknown-job"), [])


if __name__ == "__main__":
    unittest.main()
