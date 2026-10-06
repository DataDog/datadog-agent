import unittest
from types import SimpleNamespace
from unittest.mock import MagicMock, call, patch

from tasks.libs.dynamic_test.evaluator import ExecutedTest
from tasks.libs.dynamic_test.index import IndexKind
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


class JevDynTestExecutorTests(unittest.TestCase):
    @patch(f"{MODULE}.get_pipeline")
    def test_rejects_wrong_commit_and_empty_pipeline(self, get_pipeline):
        pipeline = get_pipeline.return_value
        pipeline.sha = "b" * 40
        executor = JevDynTestExecutor(MagicMock(), SHA, "42")
        with self.assertRaisesRegex(RuntimeError, "SHA"):
            executor.init_index()
        pipeline.jobs.list.assert_not_called()
        pipeline.sha = SHA
        pipeline.jobs.list.side_effect = [iter([]), iter([])]
        with self.assertRaisesRegex(NothingToEvaluateError, "No completed E2E jobs"):
            executor.init_index()

    @patch(f"{MODULE}.get_pipeline")
    def test_jobs_are_scoped_statuses_paginated_and_recorded(self, get_pipeline):
        pipeline = get_pipeline.return_value
        pipeline.sha = SHA
        # The pipeline-scoped jobs of each status; python-gitlab collapses
        # list-valued scopes, so each status is queried separately
        pipeline.jobs.list.side_effect = [
            iter([SimpleNamespace(name="new-e2e-fleet", id=1, allow_failure=False)]),
            iter(
                [
                    SimpleNamespace(name="new-e2e-installer", id=2, allow_failure=True),
                    SimpleNamespace(name="unit-tests", id=3, allow_failure=False),
                ]
            ),
        ]
        executor = JevDynTestExecutor(MagicMock(), SHA, "42")
        executor.init_index()
        get_pipeline.assert_called_once_with("DataDog/datadog-agent", "42")
        self.assertEqual(
            pipeline.jobs.list.call_args_list,
            [call(scope="success", iterator=True), call(scope="failed", iterator=True)],
        )
        self.assertEqual(executor.kind, IndexKind.JEV)
        self.assertEqual(executor.jobs, ["new-e2e-fleet", "new-e2e-installer"])
        self.assertEqual(executor.job_ids, {"new-e2e-fleet": "1", "new-e2e-installer": "2"})
        self.assertEqual(executor.unreliable_jobs, {"new-e2e-installer"})

    @patch(f"{MODULE}.suite_entry_points")
    @patch(f"{MODULE}.jev_selection")
    def test_jev_run_decides_suites_containing_the_names(self, select, suites):
        suites.return_value = {
            "a": {"TestA", "TestB", "TestDup"},
            # Fail-open suite (selector error): everything runs
            "b": {"TestB", "TestDup"},
            "unused": {"TestOther"},
        }
        select.side_effect = [
            {"run": [], "skip": ["TestA", "TestDup"]},  # suite a: skip TestA and TestDup
            {},  # suite b: selector failed, fail open
        ]
        executor = JevDynTestExecutor(MagicMock(), SHA, "42")
        self.assertEqual(executor.jev_run({"TestA", "TestB", "TestDup"}), {"TestB", "TestDup"})
        self.assertEqual([c.args[0] for c in select.call_args_list], ["a", "b"])


class JevDynTestEvaluatorTests(unittest.TestCase):
    def _evaluator(self, executor, job="job"):
        evaluator = JevDynTestEvaluator(MagicMock(), IndexKind.JEV, executor, "42", telemetry_handler=MagicMock())
        evaluator.job_ids = {job: "7"}
        return evaluator

    def test_evaluate_uses_executed_tests_as_the_per_job_universe(self):
        executor = MagicMock(spec=JevDynTestExecutor)
        executor.jobs = ["job", "cleanup-job"]
        executor.entry_points.return_value = {"TestPass", "TestFail", "TestFlaky", "TestSkipKeep"}
        executor.jev_run.return_value = {"TestPass", "TestSkipKeep"}
        tests = [
            ExecutedTest("TestPass", "pass", "42", "7", "job", False),
            ExecutedTest("TestFail", "fail", "42", "7", "job", False),
            ExecutedTest("TestFlaky", "fail", "42", "7", "job", True),
            ExecutedTest("TestNotAnEntryPoint", "fail", "42", "7", "job", False),
        ]
        evaluator = self._evaluator(executor)
        with patch.object(evaluator, "list_tests_for_job", side_effect=[tests, []]):
            results = evaluator.evaluate([])
        self.assertEqual(len(results), 1)  # the job with no executions is dropped
        result = results[0]
        self.assertEqual(result.job_name, "job")
        # Actual universe = executed entry points only
        self.assertEqual(result.actual_executed_tests, {"TestPass", "TestFail", "TestFlaky"})
        self.assertEqual(
            result.predicted_executed_tests,
            {"TestPass"},
        )  # TestSkipKeep is predicted but did not run; not a miss
        # Only reliable failures that Jev would skip count as misses
        self.assertEqual(result.not_executed_failing_tests, {"TestFail"})
        # Jev is only asked about suites containing executed entry points
        executor.jev_run.assert_called_once_with({"TestPass", "TestFail", "TestFlaky"})

    def test_evaluate_with_no_executions_anywhere_is_empty(self):
        executor = MagicMock(spec=JevDynTestExecutor)
        executor.jobs = ["job"]
        evaluator = self._evaluator(executor)
        with patch.object(evaluator, "list_tests_for_job", return_value=[]):
            self.assertEqual(evaluator.evaluate([]), [])
        executor.jev_run.assert_not_called()
        executor.entry_points.assert_not_called()


if __name__ == "__main__":
    unittest.main()
