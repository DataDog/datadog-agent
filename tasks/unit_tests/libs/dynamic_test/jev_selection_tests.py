import unittest
from types import SimpleNamespace
from unittest.mock import MagicMock, patch

from tasks.libs.dynamic_test.evaluator import DatadogDynTestEvaluator
from tasks.libs.dynamic_test.index import DynamicTestIndex, IndexKind
from tasks.libs.dynamic_test.jev_selection import (
    JevDynTestExecutor,
    NothingToEvaluateError,
    generate_job_candidates,
    jev_selection,
)

MODULE = "tasks.libs.dynamic_test.jev_selection"
SHA = "a" * 40


class TestJevSelection(unittest.TestCase):
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


class TestJevDynTestExecutor(unittest.TestCase):
    @patch(f"{MODULE}.get_pipeline")
    def test_rejects_wrong_commit_and_empty_pipeline(self, get_pipeline):
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

    @patch(f"{MODULE}._job_candidates", return_value={"new-e2e-fleet": {"TestA"}})
    @patch(f"{MODULE}.get_pipeline")
    def test_sha_mismatch_can_be_allowed_explicitly(self, get_pipeline, candidates):
        pipeline = get_pipeline.return_value
        pipeline.sha = "b" * 40  # differs from the checkout SHA
        pipeline.jobs.list.side_effect = [
            iter([SimpleNamespace(name="new-e2e-fleet", id=1, allow_failure=False)]),
            iter([]),
        ]
        executor = JevDynTestExecutor(MagicMock(), SHA, "42", require_pipeline_commit=False)
        executor.init_index()
        self.assertEqual(executor.jobs, ["new-e2e-fleet"])
        self.assertEqual(executor.index().get_indexed_tests_for_job("new-e2e-fleet"), {"TestA"})

    @patch(f"{MODULE}._job_candidates")
    @patch(f"{MODULE}.get_pipeline")
    def test_index_is_the_whole_candidate_file(self, get_pipeline, candidates):
        """The index: the ENTIRE committed candidate file - every job it knows,
        whether or not that job ran in this pipeline - so the evaluation
        shows Jev's over-selection. Pipeline jobs absent from the file are
        reported (their executed tests are not decidable)."""
        pipeline = get_pipeline.return_value
        pipeline.sha = SHA
        pipeline.jobs.list.side_effect = [
            iter([SimpleNamespace(name="new-e2e-job-a", id=7, allow_failure=False)]),
            iter(
                [
                    SimpleNamespace(name="new-e2e-job-b", id=8, allow_failure=True),
                    SimpleNamespace(name="new-e2e-missing", id=9, allow_failure=False),
                    SimpleNamespace(name="unit-tests", id=10, allow_failure=False),
                ]
            ),
        ]
        candidates.return_value = {
            "new-e2e-job-a": ["TestA"],
            "new-e2e-job-b": ["TestB"],
            "new-e2e-never-ran": ["TestX"],  # not a completed job of this pipeline
        }
        executor = JevDynTestExecutor(MagicMock(), SHA, "42")
        executor.init_index()
        # Every file job is indexed, ran or not
        self.assertEqual(sorted(executor.index().get_jobs()), ["new-e2e-job-a", "new-e2e-job-b", "new-e2e-never-ran"])
        self.assertEqual(executor.index().get_indexed_tests_for_job("new-e2e-job-a"), {"TestA"})
        self.assertEqual(executor.index().get_indexed_tests_for_job("new-e2e-never-ran"), {"TestX"})
        self.assertNotIn("new-e2e-missing", executor.index().get_jobs())  # ran but absent from the file: reported
        # The GitLab facts still recorded: latest job ids
        self.assertEqual(executor.job_ids, {"new-e2e-job-a": "7", "new-e2e-job-b": "8", "new-e2e-missing": "9"})
        # and the jobs the pipeline allows to fail (never a critical miss)
        self.assertEqual(executor.unreliable_jobs, {"new-e2e-job-b"})

    @patch(f"{MODULE}._job_candidates", return_value={"new-e2e-other": ["TestX"]})
    @patch(f"{MODULE}.get_pipeline")
    def test_no_jobs_in_the_candidate_index_is_benign(self, get_pipeline, candidates):
        pipeline = get_pipeline.return_value
        pipeline.sha = SHA
        pipeline.jobs.list.side_effect = [
            iter([SimpleNamespace(name="new-e2e-fleet", id=1, allow_failure=False)]),
            iter([]),
        ]
        executor = JevDynTestExecutor(MagicMock(), SHA, "42")
        with self.assertRaisesRegex(NothingToEvaluateError, "candidate index"):
            executor.init_index()

    def test_predictions_decide_via_the_suites_with_candidates(self):
        executor = JevDynTestExecutor(MagicMock(), SHA, "42")
        executor._index = DynamicTestIndex()
        executor._index.add_tests("job", "candidates", {"TestA", "TestDup"})
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


class TestGenerateJobCandidates(unittest.TestCase):
    @patch(f"{MODULE}.suite_entry_points")
    @patch(f"{MODULE}.get_ci_test_events")
    @patch(f"{MODULE}.get_pipeline")
    def test_unions_latest_attempt_entry_points_across_pipelines(self, get_pipeline, events, suites):
        pipeline = get_pipeline.return_value
        pipeline.jobs.list.side_effect = [
            iter([SimpleNamespace(name="new-e2e-job-a", id=7, allow_failure=False)]),
            iter([]),
            iter([SimpleNamespace(name="new-e2e-job-a", id=70, allow_failure=False)]),
            iter([]),
        ]
        events.side_effect = [
            # pipeline 42: latest attempt id 7, an older attempt id 6, an
            # unknown-name event, a non-e2e job event
            [
                _event("TestA", "new-e2e-job-a", "7"),
                _event("TestA", "new-e2e-job-a", "6"),  # older attempt: dropped
                _event("TestNotAnEntry", "new-e2e-job-a", "7"),  # not an entry point: dropped
                _event("TestC", "other-job", "99"),  # not a completed e2e job: dropped
            ],
            # pipeline 43: same job, new attempt adds a test
            [_event("TestB", "new-e2e-job-a", "70")],
        ]
        suites.return_value = {"fleet": {"TestA", "TestB"}}
        candidates = generate_job_candidates(["42", "43"])
        self.assertEqual(candidates, {"new-e2e-job-a": {"TestA", "TestB"}})
        # One pipeline-wide query per pipeline
        for i, c in enumerate(events.call_args_list):
            self.assertIn(f"@ci.pipeline.id:{['42', '43'][i]}", c.args[0])
            self.assertNotIn("@ci.job.name:", c.args[0])


class TestSharedEvaluator(unittest.TestCase):
    """The payoff: the plain shared evaluator runs the Jev executor untouched."""

    @patch("tasks.libs.dynamic_test.evaluator.get_ci_test_events")
    @patch.object(JevDynTestExecutor, "_jev_run", return_value={"TestPass", "TestSkipKeep"})
    def test_evaluate_runs_the_jev_executor_end_to_end(self, _, events):
        events.return_value = [
            _event("TestPass", "job", "7"),
            _event("TestFail", "job", "7", status="fail"),
            _event("TestFlaky", "job", "7", status="fail", flaky=True),
            _event("TestNotAnEntry", "job", "7", status="fail"),  # not a candidate: not decidable
        ]
        executor = JevDynTestExecutor(MagicMock(), SHA, "42")
        executor._index = DynamicTestIndex()
        executor._index.add_tests("job", "candidates", {"TestPass", "TestFail", "TestFlaky", "TestSkipKeep"})
        evaluator = DatadogDynTestEvaluator(MagicMock(), IndexKind.JEV, executor, "42", telemetry_handler=MagicMock())
        evaluator.index = executor.index()  # normally set by initialize()
        results = evaluator.evaluate([])  # DatadogDynTestEvaluator.evaluate, no subclass
        self.assertEqual(len(results), 1)
        result = results[0]
        self.assertEqual(result.job_name, "job")
        # Actual universe = executed candidates only
        self.assertEqual(result.actual_executed_tests, {"TestPass", "TestFail", "TestFlaky"})
        # TestSkipKeep is a predicted candidate that did not run: the new
        # over-selection signal (the + line in the diff), not a miss
        self.assertEqual(result.predicted_executed_tests, {"TestPass", "TestSkipKeep"})
        # Only reliable failures that Jev would skip count as misses
        self.assertEqual(result.not_executed_failing_tests, {"TestFail"})


if __name__ == "__main__":
    unittest.main()
