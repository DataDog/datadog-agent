import unittest
from types import SimpleNamespace
from unittest.mock import MagicMock, patch

from tasks.libs.dynamic_test.index import IndexKind
from tasks.libs.dynamic_test.jev_replay import (
    BulkDatadogDynTestEvaluator,
    _executed_by_job,
    recent_pipeline_ids,
    replay_jev,
)

MODULE = "tasks.libs.dynamic_test.jev_replay"
SELECTION = "tasks.libs.dynamic_test.jev_selection"
SHA = "a" * 40


def _event(name, job, job_id, status="pass", flaky=False):
    return {
        "attributes": {
            "attributes": {
                "test": {"name": name, "status": status, "agent_is_flaky_failure": flaky},
                "ci": {"job": {"id": job_id, "name": job}, "pipeline": {"id": "42"}},
            }
        }
    }


def _pipeline(jobs, sha=SHA):
    p = MagicMock()
    p.sha = sha
    p.jobs.list.side_effect = [iter(jobs), iter([])]
    return p


class TestBulkEvaluator(unittest.TestCase):
    def test_reads_the_pre_fetched_query(self):
        executed = {"job": ["test"]}
        evaluator = BulkDatadogDynTestEvaluator(
            MagicMock(), IndexKind.JEV, MagicMock(), "42", executed_by_job=executed, telemetry_handler=MagicMock()
        )
        self.assertEqual(evaluator.list_tests_for_job("job"), ["test"])
        self.assertEqual(evaluator.list_tests_for_job("missing"), [])


class TestExecutedByJob(unittest.TestCase):
    @patch(f"{MODULE}.get_ci_test_events")
    def test_groups_latest_attempts_only(self, events):
        events.return_value = [
            _event("TestA", "new-e2e-job", "7"),
            _event("TestA", "new-e2e-job", "6"),  # older attempt: dropped
            _event("TestB", "other-job", "99"),  # not a completed e2e job: dropped
        ]
        grouped = _executed_by_job("42", {"7"})
        self.assertEqual({job: [t.name for t in ts] for job, ts in grouped.items()}, {"new-e2e-job": ["TestA"]})


class TestRecentPipelineIds(unittest.TestCase):
    @patch(f"{MODULE}.get_gitlab_api")
    def test_takes_the_most_recent_of_the_ref(self, api):
        api.return_value.projects.get.return_value.pipelines.list.return_value = [
            SimpleNamespace(id=3),
            SimpleNamespace(id=2),
            SimpleNamespace(id=1),
        ]
        self.assertEqual(recent_pipeline_ids("some/branch", 3), ["3", "2", "1"])
        api.return_value.projects.get.return_value.pipelines.list.assert_called_once_with(
            ref="some/branch", per_page=3, sort="desc"
        )


class TestReplay(unittest.TestCase):
    """Two pipelines: one evaluates (a miss counted), one has no completed e2e
    test jobs (skipped); the Jev run-set is decided once and shared."""

    def _pipeline(self, jobs, sha=SHA):
        p = MagicMock()
        p.sha = sha
        p.jobs.list.side_effect = [iter(jobs), iter([])]
        return p

    @patch(f"{SELECTION}._job_candidates")
    @patch(f"{SELECTION}.get_pipeline")
    @patch(f"{MODULE}.get_pipeline")
    @patch(f"{MODULE}.jev_run_for", return_value={"TestA"})
    @patch(f"{MODULE}.get_ci_test_events")
    def test_replays_and_aggregates(self, events, run_for, get_pipeline, get_pipeline_selection, candidates):
        # Pipeline 42: one completed e2e job with two candidates
        # Pipeline 43: no completed e2e jobs -> skipped
        pipelines = {
            "42": self._pipeline([SimpleNamespace(name="new-e2e-job", id=7, allow_failure=False)]),
            "43": self._pipeline([]),
        }
        get_pipeline.side_effect = lambda _, pid: pipelines[pid]
        get_pipeline_selection.side_effect = lambda _, pid: pipelines[pid]
        candidates.return_value = {"new-e2e-job": ["TestA", "TestFail"]}
        events.return_value = [
            _event("TestA", "new-e2e-job", "7"),
            _event("TestFail", "new-e2e-job", "7", status="fail"),
            _event("TestSkipped", "new-e2e-job", "7", status="skip"),  # not executed: dropped by the parser
        ]

        summary = replay_jev(MagicMock(), ["42", "43"], output="/tmp/test-replay.json")

        # The run-set: decided ONCE, over the union of the candidates
        run_for.assert_called_once_with({"TestA", "TestFail"})
        # One pipeline evaluated, one skipped with its reason
        self.assertEqual(summary["pipelines_evaluated"], 1)
        self.assertEqual(
            summary["pipelines_skipped"], [{"pipeline_id": "43", "reason": "No completed E2E jobs in pipeline 43"}]
        )
        # The comparison: TestFail executed+failed and not in the run-set -> miss
        self.assertEqual(summary["miss_occurrences"], 1)
        self.assertEqual(summary["missed_tests"], {"TestFail": 1})
        self.assertEqual(summary["executed_test_occurrences"], 2)
        # TestA kept (executed and in the run-set), TestFail skipped-but-executed
        self.assertEqual(summary["executed_would_skip"], 1)
        # The per-pipeline detail
        self.assertEqual(
            summary["pipelines"],
            [
                {
                    "pipeline_id": "42",
                    "jobs": 1,
                    "executed": 2,
                    "predicted": 1,
                    "skipped_executed": 1,
                    "misses": ["TestFail"],
                }
            ],
        )
        with open("/tmp/test-replay.json") as f:
            import json

            self.assertEqual(json.load(f), summary)


if __name__ == "__main__":
    unittest.main()
