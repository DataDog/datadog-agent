import unittest
from unittest.mock import MagicMock, patch

from invoke import Context
from invoke.exceptions import Exit

from tasks.dyntest import evaluate_index
from tasks.libs.dynamic_test.index import IndexKind
from tasks.libs.dynamic_test.telemetry import ConsoleTelemetryHandler


class TestEvaluateIndex(unittest.TestCase):
    def setUp(self):
        # The evaluated pipeline ran the checkout commit ("abc") unless a test says otherwise
        patcher = patch("tasks.dyntest.get_pipeline", return_value=MagicMock(sha="abc"))
        self.get_pipeline = patcher.start()
        self.addCleanup(patcher.stop)

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
        self.assertEqual(executor.call_args.args[1:], ())
        self.assertEqual(evaluator.call_args.args[4], "abc")  # the evaluator tags the commit
        # The shared evaluator is constructed exactly like for coverage: the
        # Jev executor plugs in through the standard interface only
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
    def test_jev_requires_the_pipeline_commit(self, executor, _, enabled):
        """An unrelated pipeline is rejected even with --commit-sha omitted."""
        self.get_pipeline.return_value.sha = "def"
        with self.assertRaisesRegex(Exit, "Pipeline 42 ran def"):
            evaluate_index.body(Context(), pipeline_id="42", selector="jev", send_stats=False)
        self.get_pipeline.assert_called_once_with("DataDog/datadog-agent", "42")
        executor.assert_not_called()

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
    @patch("tasks.dyntest.is_enabled", return_value=False)
    @patch("tasks.dyntest.JevDynTestExecutor")
    def test_jev_disabled_by_feature_flag_exits_0(self, executor, enabled, _):
        """Flag disabled: the task exits 0 and builds no executor at all."""
        evaluate_index.body(Context(), pipeline_id="42", selector="jev", send_stats=False)
        enabled.assert_called_once()
        executor.assert_not_called()

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
