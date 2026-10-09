import unittest
from types import SimpleNamespace
from unittest.mock import MagicMock, patch

from tasks.libs.dynamic_test.evaluator import DatadogDynTestEvaluator
from tasks.libs.dynamic_test.index import DynamicTestIndex, IndexKind
from tasks.libs.dynamic_test.jev_selection import (
    JevDynTestExecutor,
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
    @patch(f"{MODULE}._job_candidates")
    @patch(f"{MODULE}.get_pipeline")
    def test_index_is_the_whole_candidate_file(self, get_pipeline, candidates):
        """The index: the ENTIRE committed candidate file, no pipeline lookup."""
        candidates.return_value = {"new-e2e-job-a": ["TestA"], "new-e2e-job-b": ["TestB"]}
        executor = JevDynTestExecutor(MagicMock())
        self.assertEqual(sorted(executor.index().get_jobs()), ["new-e2e-job-a", "new-e2e-job-b"])
        self.assertEqual(executor.index().get_indexed_tests_for_job("new-e2e-job-a"), {"TestA"})
        get_pipeline.assert_not_called()

    def test_predictions_decide_via_the_suites_with_candidates(self):
        executor = JevDynTestExecutor(MagicMock())
        executor._index = DynamicTestIndex()
        executor._index.add_tests("job", "candidates", {"TestA", "TestDup"})
        with (
            patch(f"{MODULE}.suite_entry_points", return_value={"a": {"TestA", "TestDup"}, "b": {"TestDup"}}),
            patch(
                f"{MODULE}.jev_selection",
                side_effect=lambda suite: {
                    "a": {"run": [], "skip": ["TestA", "TestDup"]},  # suite a: skips both
                    "b": {},  # suite b: selector failure, fail open
                }[suite],
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
        executor = JevDynTestExecutor(MagicMock())
        executor._index = DynamicTestIndex()
        executor._index.add_tests("job", "candidates", {"TestPass", "TestFail", "TestFlaky", "TestSkipKeep"})
        evaluator = DatadogDynTestEvaluator(
            MagicMock(), IndexKind.JEV, executor, "42", SHA, telemetry_handler=MagicMock()
        )
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
