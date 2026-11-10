import tempfile
import unittest
from pathlib import Path
from unittest.mock import MagicMock, patch

from gitlab.exceptions import GitlabGetError
from invoke import Context
from invoke.exceptions import Exit

from tasks.dyntest import _read_pipeline_pr_summary, evaluate_index, generate_jev_pr_summary
from tasks.libs.dynamic_test.index import IndexKind
from tasks.libs.dynamic_test.jev_selection import NothingToEvaluateError
from tasks.libs.dynamic_test.telemetry import ConsoleTelemetryHandler


class TestEvaluateIndex(unittest.TestCase):
    @patch("tasks.dyntest.get_commit_sha", return_value="abc")
    @patch("tasks.dyntest.is_enabled", return_value=True)
    @patch("tasks.dyntest.S3Backend")
    @patch("tasks.dyntest.JevDynTestExecutor")
    @patch("tasks.dyntest.DatadogDynTestEvaluator")
    def test_jev_uses_shared_evaluator_without_s3_or_publishing(self, evaluator, executor, s3, _, enabled):
        executor.return_value.kind = IndexKind.JEV
        result = MagicMock()
        result.actual_count.return_value = 1
        evaluator.return_value.evaluate.return_value = [result]
        evaluate_index.body(Context(), pipeline_id="42", selector="jev", send_stats=False)
        s3.assert_not_called()
        self.assertEqual(executor.call_args.args[1:], ("abc", "42"))
        self.assertTrue(executor.call_args.kwargs["require_pipeline_commit"])
        self.assertEqual(executor.call_args.kwargs["pr_summary"], "")
        evaluate_index.body(Context(), pipeline_id="42", selector="jev", send_stats=False, ignore_sha_mismatch=True)
        self.assertFalse(executor.call_args.kwargs["require_pipeline_commit"])
        # The shared evaluator is constructed exactly like for coverage: the
        # Jev executor plugs in through the standard interface only (the
        # allow-failure job filtering is the evaluator's own, not wired here)
        options = evaluator.call_args.kwargs
        self.assertNotIn("unreliable_jobs", options)
        self.assertNotIn("test_env", options)
        self.assertNotIn("lookback_days", options)
        self.assertIsInstance(options["telemetry_handler"], ConsoleTelemetryHandler)
        evaluator.return_value.send_stats_to_datadog.assert_not_called()

    @patch("tasks.dyntest.get_commit_sha", return_value="abc")
    @patch("tasks.dyntest.get_modified_files", return_value=["pkg/file.go"])
    @patch("tasks.dyntest.S3Backend")
    @patch("tasks.dyntest.sleep")
    @patch("tasks.dyntest.DynTestExecutor")
    @patch("tasks.dyntest.DatadogDynTestEvaluator")
    def test_coverage_still_evaluates_three_indexes(self, evaluator, executor, sleep, s3, *_):
        executor.return_value.kind = IndexKind.PACKAGE
        result = MagicMock()
        result.actual_count.return_value = 1
        evaluator.return_value.evaluate.return_value = [result]
        evaluate_index.body(Context(), "s3://bucket", "abc", "42", send_stats=False)
        s3.assert_called_once_with("s3://bucket")
        self.assertEqual(
            [call.args[2] for call in executor.call_args_list],
            [IndexKind.PACKAGE, IndexKind.FILE, IndexKind.DIFFED_PACKAGE],
        )
        self.assertEqual(evaluator.call_count, 3)
        self.assertEqual(sleep.call_count, 2)
        self.assertNotIn("test_env", evaluator.call_args.kwargs)
        self.assertEqual(set(evaluator.return_value.evaluate.call_args.args[0]), {"pkg", "pkg/file.go"})

    def test_invalid_arguments(self):
        for args in (
            {"selector": "typo", "pipeline_id": "42"},
            {"pipeline_id": ""},
        ):
            with self.subTest(args=args), self.assertRaises(Exit):
                evaluate_index.body(Context(), **args)

    @patch("tasks.dyntest.get_commit_sha", return_value="abc")
    @patch("tasks.dyntest.is_enabled", return_value=True)
    def test_jev_requires_matching_checkout(self, _, enabled):
        with self.assertRaisesRegex(Exit, "check out"):
            evaluate_index.body(Context(), commit_sha="other", pipeline_id="42", selector="jev")

    @patch("tasks.dyntest.get_commit_sha", return_value="abc")
    @patch("tasks.dyntest.is_enabled", return_value=True)
    @patch("tasks.dyntest.JevDynTestExecutor")
    @patch("tasks.dyntest.DatadogDynTestEvaluator")
    def test_empty_evaluation_fails_without_sending_stats(self, evaluator, executor, _, enabled):
        executor.return_value.kind = IndexKind.JEV
        for results in ([], [MagicMock(actual_count=lambda: 0)]):
            evaluator.return_value.evaluate.return_value = results
            with self.assertRaisesRegex(Exit, "incomplete"):
                evaluate_index.body(Context(), pipeline_id="42", selector="jev")
            evaluator.return_value.send_stats_to_datadog.assert_not_called()

    @patch("tasks.dyntest.get_commit_sha", return_value="abc")
    @patch("tasks.dyntest.is_enabled", return_value=True)
    @patch("tasks.dyntest.JevDynTestExecutor")
    @patch("tasks.dyntest.DatadogDynTestEvaluator")
    def test_nothing_to_evaluate_is_benign(self, evaluator, executor, _, enabled):
        """A pipeline with no completed E2E test jobs exits cleanly, not red."""
        executor.return_value.kind = IndexKind.JEV
        evaluator.return_value.initialize.return_value = False
        evaluator.return_value.initialization_error = NothingToEvaluateError("No completed E2E jobs in pipeline 42")
        # Must not raise
        evaluate_index.body(Context(), pipeline_id="42", selector="jev", send_stats=False)
        evaluator.return_value.evaluate.assert_not_called()

    @patch("tasks.dyntest.get_commit_sha", return_value="abc")
    @patch("tasks.dyntest.is_enabled", return_value=False)
    @patch("tasks.dyntest.JevDynTestExecutor")
    def test_jev_disabled_by_feature_flag_exits_0(self, executor, enabled, _):
        """Flag disabled: the task exits 0 and builds no executor at all."""
        evaluate_index.body(Context(), pipeline_id="42", selector="jev", send_stats=False)
        enabled.assert_called_once()
        executor.assert_not_called()

    @patch.dict("os.environ", {"CI_PIPELINE_ID": "99", "DDCI_REQUEST_ID": "request"})
    @patch("tasks.dyntest.get_commit_sha", return_value="abc")
    @patch("tasks.dyntest.is_enabled", return_value=True)
    @patch("tasks.dyntest._read_pipeline_pr_summary", return_value="parent summary")
    @patch("tasks.dyntest.JevDynTestExecutor")
    @patch("tasks.dyntest.DatadogDynTestEvaluator")
    @patch("tasks.dyntest.DatadogTelemetryHandler")
    def test_child_evaluates_parent_with_parent_summary(self, telemetry, evaluator, executor, summary, *_):
        executor.return_value.kind = IndexKind.JEV
        evaluator.return_value.evaluate.return_value = [MagicMock(actual_count=lambda: 1)]
        evaluate_index.body(Context(), pipeline_id="42", selector="jev", pr_summary_from_pipeline=True)
        summary.assert_called_once_with("42")
        self.assertEqual(executor.call_args.args[1:], ("abc", "42"))
        self.assertEqual(executor.call_args.kwargs["pr_summary"], "parent summary")
        self.assertEqual(evaluator.call_args.args[3], "42")
        self.assertIn("pipeline_id:42", telemetry.call_args.kwargs["default_tags"])
        self.assertIn("jev_llm_summary:true", telemetry.call_args.kwargs["default_tags"])

    @patch("tasks.dyntest.get_commit_sha", return_value="abc")
    @patch("tasks.dyntest.is_enabled", return_value=False)
    @patch("tasks.dyntest.get_gitlab_repo")
    def test_disabled_child_does_not_retrieve_summary(self, repo, *_):
        evaluate_index.body(Context(), pipeline_id="42", selector="jev", pr_summary_from_pipeline=True)
        repo.assert_not_called()

    @patch("tasks.dyntest.get_commit_sha", return_value="abc")
    @patch("tasks.dyntest.is_enabled", return_value=True)
    @patch("tasks.dyntest.JevDynTestExecutor")
    @patch("tasks.dyntest.DatadogDynTestEvaluator")
    @patch("tasks.dyntest.DatadogTelemetryHandler")
    def test_pr_summary_file_is_injected_and_tagged(self, telemetry, evaluator, executor, *_):
        executor.return_value.kind = IndexKind.JEV
        result = MagicMock()
        result.actual_count.return_value = 1
        evaluator.return_value.evaluate.return_value = [result]
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory, "summary.txt")
            # Missing file (generation disabled or failed): the raw diff
            evaluate_index.body(Context(), pipeline_id="42", selector="jev", pr_summary_file=str(path))
            self.assertEqual(executor.call_args.kwargs["pr_summary"], "")
            self.assertIn("jev_llm_summary:false", telemetry.call_args.kwargs["default_tags"])
            path.write_text("LLM SUMMARY\n")
            evaluate_index.body(Context(), pipeline_id="42", selector="jev", pr_summary_file=str(path))
            self.assertEqual(executor.call_args.kwargs["pr_summary"], "LLM SUMMARY")
            self.assertIn("jev_llm_summary:true", telemetry.call_args.kwargs["default_tags"])

    @patch("tasks.dyntest.get_commit_sha", return_value="abc")
    @patch("tasks.dyntest.is_enabled", return_value=True)
    @patch("tasks.dyntest.JevDynTestExecutor")
    @patch("tasks.dyntest.DatadogDynTestEvaluator")
    def test_initialization_failure_is_visible(self, evaluator, executor, _, enabled):
        executor.return_value.kind = IndexKind.JEV
        evaluator.return_value.initialize.return_value = False
        with self.assertRaisesRegex(Exit, "incomplete"):
            evaluate_index.body(Context(), pipeline_id="42", selector="jev", send_stats=False)
        evaluator.return_value.evaluate.assert_not_called()


class TestPipelinePrSummary(unittest.TestCase):
    @patch("tasks.dyntest.get_gitlab_repo")
    def test_exact_parent_and_latest_attempt(self, get_repo):
        repo = get_repo.return_value
        older = MagicMock(id=10, status="success")
        older.name = "jev-pr-summary"
        latest = MagicMock(id=20, status="success")
        latest.name = "jev-pr-summary"
        other = MagicMock(id=30, status="success")
        other.name = "other"
        repo.pipelines.get.return_value.jobs.list.return_value = [older, latest, other]
        repo.jobs.get.return_value.artifact.return_value = b" parent summary\n"
        self.assertEqual(_read_pipeline_pr_summary("42"), "parent summary")
        repo.pipelines.get.assert_called_once_with("42", lazy=True)
        repo.pipelines.get.return_value.jobs.list.assert_called_once_with(iterator=True)
        repo.jobs.get.assert_called_once_with(20, lazy=True)
        repo.jobs.get.return_value.artifact.assert_called_once_with("jev-pr-summary.txt")

    @patch("tasks.dyntest.get_gitlab_repo")
    def test_absent_or_unsuccessful_summary_falls_back(self, get_repo):
        repo = get_repo.return_value
        for status in (None, "failed", "skipped", "canceled"):
            with self.subTest(status=status):
                summary = MagicMock(id=10, status=status)
                summary.name = "jev-pr-summary"
                repo.pipelines.get.return_value.jobs.list.return_value = [summary] if status else []
                self.assertEqual(_read_pipeline_pr_summary("42"), "")
                repo.jobs.get.assert_not_called()

    @patch("tasks.dyntest.get_gitlab_repo")
    def test_optional_artifact_and_visible_errors(self, get_repo):
        repo = get_repo.return_value
        summary = MagicMock(id=10, status="success")
        summary.name = "jev-pr-summary"
        repo.pipelines.get.return_value.jobs.list.return_value = [summary]
        artifact = repo.jobs.get.return_value.artifact
        artifact.side_effect = GitlabGetError("not found", response_code=404)
        self.assertEqual(_read_pipeline_pr_summary("42"), "")
        artifact.side_effect = None
        for content in (b"\xff", b" \n"):
            artifact.return_value = content
            self.assertEqual(_read_pipeline_pr_summary("42"), "")
        for code in (401, 403, 500):
            artifact.side_effect = GitlabGetError("failure", response_code=code)
            with self.subTest(code=code), self.assertRaises(GitlabGetError):
                _read_pipeline_pr_summary("42")
        repo.pipelines.get.return_value.jobs.list.side_effect = RuntimeError("listing unavailable")
        with self.assertRaisesRegex(RuntimeError, "listing unavailable"):
            _read_pipeline_pr_summary("42")


class TestGenerateJevPrSummary(unittest.TestCase):
    @patch("tasks.dyntest.generate_pr_summary", return_value="LLM SUMMARY")
    def test_gated_by_the_feature_flag(self, generate):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory, "summary.txt")
            with patch("tasks.dyntest.is_enabled", return_value=False):
                generate_jev_pr_summary.body(Context(), output=str(path))
            generate.assert_not_called()
            self.assertFalse(path.exists())
            with patch("tasks.dyntest.is_enabled", return_value=True):
                generate_jev_pr_summary.body(Context(), output=str(path))
            generate.assert_called_once()
            self.assertEqual(path.read_text().strip(), "LLM SUMMARY")
