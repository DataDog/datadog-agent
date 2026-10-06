import tempfile
import unittest
from types import SimpleNamespace
from unittest.mock import MagicMock, call, patch

from tasks.libs.dynamic_test.index import IndexKind
from tasks.libs.dynamic_test.jev_replay import (
    BulkDatadogDynTestEvaluator,
    _executed_by_job,
    _fetch_pipeline_commit,
    recent_pipeline_ids,
    replay_jev,
)

MODULE = "tasks.libs.dynamic_test.jev_replay"
SELECTION = "tasks.libs.dynamic_test.jev_selection"
SHA_A = "a" * 40
SHA_B = "b" * 40


def _event(name, job, job_id, status="pass", flaky=False):
    return {
        "attributes": {
            "attributes": {
                "test": {"name": name, "status": status, "agent_is_flaky_failure": flaky},
                "ci": {"job": {"id": job_id, "name": job}, "pipeline": {"id": "42"}},
            }
        }
    }


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
    """The branch-independence: each pipeline's run-set is decided from ITS
    OWN commit, inside the worktree checked out at its SHA - the current
    checkout only provides the tooling."""

    def _pipeline(self, pid, sha, jobs):
        p = MagicMock()
        p.id, p.sha, p.ref = pid, sha, "some/branch"
        p.jobs.list.side_effect = [iter(jobs), iter([])]
        return p

    def _setup(self, pipelines, candidates, events):
        """Common mock wiring; returns the mocks dict."""
        by_id = {str(p.id): p for p in pipelines}
        mocks = {}
        for name, target in [
            ("get_pipeline_selection", f"{SELECTION}.get_pipeline"),
            ("get_pipeline", f"{MODULE}.get_pipeline"),
            ("candidates", f"{SELECTION}._job_candidates"),
            ("events", f"{MODULE}.get_ci_test_events"),
            ("run_for", f"{MODULE}.jev_run_for"),
            ("commit_present", f"{MODULE}._commit_present"),
            ("worktree", f"{MODULE}._worktree"),
            ("git", f"{MODULE}._git"),
        ]:
            decorator = patch(target)
            mocks[name] = decorator.start()
            self.addCleanup(decorator.stop)
        mocks["get_pipeline"].side_effect = lambda _, pid: by_id[pid]
        mocks["get_pipeline_selection"].side_effect = lambda _, pid: by_id[pid]
        mocks["candidates"].return_value = candidates
        mocks["events"].return_value = events
        mocks["commit_present"].return_value = True
        workdir = tempfile.mkdtemp(prefix="jev-replay-test-")
        # A no-op worktree context (the real one is exercised by the live runs);
        # _selector_context is NOT mocked: it really chdirs into the tempdir
        from contextlib import nullcontext

        mocks["worktree"].side_effect = lambda: nullcontext(workdir)
        return mocks, workdir

    def test_decisions_come_from_each_pipelines_own_commit(self):
        # Two pipelines, two commits: each executor gets ITS OWN run-set
        p1 = self._pipeline(1, SHA_A, [SimpleNamespace(name="new-e2e-job", id=7, allow_failure=False)])
        p2 = self._pipeline(2, SHA_B, [SimpleNamespace(name="new-e2e-job", id=70, allow_failure=False)])
        events = [
            _event("TestA", "new-e2e-job", "7", status="fail"),
            _event("TestA", "new-e2e-job", "70", status="fail"),
        ]
        mocks, workdir = self._setup([p1, p2], {"new-e2e-job": ["TestA", "TestB"]}, events)
        # Pipeline 1 keeps TestA (no miss), pipeline 2 skips it: its failing
        # executed TestA is a miss
        mocks["run_for"].side_effect = [{"TestA", "TestB"}, {"TestB"}]

        summary = replay_jev(MagicMock(), ["1", "2"])

        # One selection per distinct commit, decided from the worktree
        self.assertEqual(mocks["run_for"].call_count, 2)
        for c in mocks["run_for"].call_args_list:
            self.assertEqual(c.args[0], {"TestA", "TestB"})
            self.assertEqual(c.kwargs["root"], workdir)
        # Pipeline 2's decision -> its failing executed TestA is a miss
        self.assertEqual(summary["miss_occurrences"], 1)
        self.assertEqual(summary["missed_tests"], {"TestA": 1})
        self.assertEqual(summary["pipelines_evaluated"], 2)

    def test_same_commit_decides_once(self):
        # Two pipelines at the same SHA (a retry): one selection, shared
        p1 = self._pipeline(1, SHA_A, [SimpleNamespace(name="new-e2e-job", id=7, allow_failure=False)])
        p2 = self._pipeline(2, SHA_A, [SimpleNamespace(name="new-e2e-job", id=70, allow_failure=False)])
        events = [_event("TestA", "new-e2e-job", "7")]
        mocks, _ = self._setup([p1, p2], {"new-e2e-job": ["TestA"]}, events)
        mocks["run_for"].return_value = {"TestA"}

        summary = replay_jev(MagicMock(), ["1", "2"])

        self.assertEqual(mocks["run_for"].call_count, 1)
        self.assertEqual(summary["pipelines_evaluated"], 2)
        self.assertEqual(summary["miss_occurrences"], 0)

    def test_pipeline_without_e2e_jobs_is_skipped(self):
        p1 = self._pipeline(1, SHA_A, [SimpleNamespace(name="new-e2e-job", id=7, allow_failure=False)])
        p2 = self._pipeline(2, SHA_B, [])  # no completed e2e jobs
        events = [_event("TestA", "new-e2e-job", "7")]
        mocks, _ = self._setup([p1, p2], {"new-e2e-job": ["TestA"]}, events)
        mocks["run_for"].return_value = {"TestA"}

        summary = replay_jev(MagicMock(), ["1", "2"])

        self.assertEqual(summary["pipelines_evaluated"], 1)
        self.assertEqual(
            summary["pipelines_skipped"], [{"pipeline_id": "2", "reason": "No completed E2E jobs in pipeline 2"}]
        )

    def test_unfetchable_commit_is_skipped_not_fatal(self):
        p1 = self._pipeline(1, SHA_A, [SimpleNamespace(name="new-e2e-job", id=7, allow_failure=False)])
        events = [_event("TestA", "new-e2e-job", "7")]
        mocks, _ = self._setup([p1], {"new-e2e-job": ["TestA"]}, events)
        mocks["run_for"].return_value = {"TestA"}
        # The commit is absent locally and cannot be fetched: skipped, not fatal
        mocks["commit_present"].return_value = False
        with patch(f"{MODULE}._fetch_pipeline_commit", side_effect=RuntimeError("cannot fetch commit")):
            summary = replay_jev(MagicMock(), ["1"])
        self.assertEqual(summary["pipelines_evaluated"], 0)
        self.assertIn("cannot fetch commit", summary["pipelines_skipped"][0]["reason"])

    @patch(f"{MODULE}._pull_head_ref", return_value=None)
    @patch(f"{MODULE}._connected_to_main")
    @patch(f"{MODULE}._git")
    def test_fetch_falls_back_to_the_pull_head_mirror_and_deepens(self, git, connected, pull_ref):
        """A gone branch fetches via the refs/pull/N/head mirror, and a
        shallow clone deepens until the history connects to main."""
        pipeline = SimpleNamespace(sha=SHA_A, ref="gone/branch")
        pull_ref.return_value = "refs/pull/57209/head"
        connected.side_effect = [False, False, True]  # plain fetch, deepen=2000, deepen=10000
        git.side_effect = [RuntimeError("ref gone"), None, None, None]
        _fetch_pipeline_commit(pipeline)
        self.assertEqual(
            git.call_args_list,
            [
                call("fetch", "origin", "gone/branch"),
                call("fetch", "origin", "refs/pull/57209/head"),
                call("fetch", "origin", "refs/pull/57209/head", "--deepen=2000"),
                call("fetch", "origin", "refs/pull/57209/head", "--deepen=10000"),
            ],
        )

    @patch(f"{MODULE}._pull_head_ref", return_value=None)
    @patch(f"{MODULE}._connected_to_main", return_value=False)
    @patch(f"{MODULE}._git")
    def test_fetch_reports_a_never_connecting_history(self, git, connected, pull_ref):
        """Shallow histories that never connect fail the pipeline - never a
        bogus tip-to-tip diff."""
        pipeline = SimpleNamespace(sha=SHA_A, ref="some/branch")
        with self.assertRaisesRegex(RuntimeError, "never connects to origin/main"):
            _fetch_pipeline_commit(pipeline)
        self.assertEqual(
            [c.args for c in git.call_args_list],
            [
                ("fetch", "origin", "some/branch"),
                ("fetch", "origin", "some/branch", "--deepen=2000"),
                ("fetch", "origin", "some/branch", "--deepen=10000"),
            ],
        )


if __name__ == "__main__":
    unittest.main()
