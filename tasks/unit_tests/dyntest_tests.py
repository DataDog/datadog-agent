import unittest
from unittest.mock import MagicMock, patch

from invoke import Context
from invoke.exceptions import Exit

from tasks.dyntest import evaluate_index
from tasks.libs.dynamic_test.index import IndexKind
from tasks.libs.dynamic_test.jev_selection import NothingToEvaluateError
from tasks.libs.dynamic_test.telemetry import ConsoleTelemetryHandler


class EvaluateIndexTests(unittest.TestCase):
    @patch("tasks.dyntest.get_commit_sha", return_value="abc")
    @patch("tasks.dyntest.S3Backend")
    @patch("tasks.dyntest.JevDynTestExecutor")
    @patch("tasks.dyntest.DatadogDynTestEvaluator")
    def test_jev_uses_shared_evaluator_without_s3_or_publishing(self, evaluator, executor, s3, _):
        executor.return_value.kind = IndexKind.JEV
        result = MagicMock()
        result.actual_count.return_value = 1
        evaluator.return_value.evaluate.return_value = [result]
        evaluate_index.body(Context(), pipeline_id="42", selector="jev", send_stats=False)
        s3.assert_not_called()
        self.assertEqual(executor.call_args.args[1:], ("abc", "42"))
        options = evaluator.call_args.kwargs
        self.assertEqual(options["test_env"], "nativetest")
        self.assertIs(options["job_ids"], executor.return_value.job_ids)
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
        self.assertEqual(evaluator.call_args.kwargs["test_env"], "prod")
        self.assertEqual(set(evaluator.return_value.evaluate.call_args.args[0]), {"pkg", "pkg/file.go"})

    def test_invalid_arguments(self):
        for args in (
            {"selector": "typo", "pipeline_id": "42"},
            {"pipeline_id": ""},
            {"pipeline_id": "42", "lookback_days": 0},
        ):
            with self.subTest(args=args), self.assertRaises(Exit):
                evaluate_index.body(Context(), **args)

    @patch("tasks.dyntest.get_commit_sha", return_value="abc")
    def test_jev_requires_matching_checkout(self, _):
        with self.assertRaisesRegex(Exit, "check out"):
            evaluate_index.body(Context(), commit_sha="other", pipeline_id="42", selector="jev")

    @patch("tasks.dyntest.get_commit_sha", return_value="abc")
    @patch("tasks.dyntest.JevDynTestExecutor")
    @patch("tasks.dyntest.DatadogDynTestEvaluator")
    def test_empty_evaluation_fails_without_sending_stats(self, evaluator, executor, _):
        executor.return_value.kind = IndexKind.JEV
        for results in ([], [MagicMock(actual_count=lambda: 0)]):
            evaluator.return_value.evaluate.return_value = results
            with self.assertRaisesRegex(Exit, "incomplete"):
                evaluate_index.body(Context(), pipeline_id="42", selector="jev")
            evaluator.return_value.send_stats_to_datadog.assert_not_called()

    @patch("tasks.dyntest.get_commit_sha", return_value="abc")
    @patch("tasks.dyntest.JevDynTestExecutor")
    @patch("tasks.dyntest.DatadogDynTestEvaluator")
    def test_nothing_to_evaluate_is_benign(self, evaluator, executor, _):
        """A pipeline with no completed E2E test jobs exits cleanly, not red."""
        executor.return_value.kind = IndexKind.JEV
        evaluator.return_value.initialize.return_value = False
        evaluator.return_value.initialization_error = NothingToEvaluateError("No completed E2E jobs in pipeline 42")
        # Must not raise
        evaluate_index.body(Context(), pipeline_id="42", selector="jev", send_stats=False)
        evaluator.return_value.evaluate.assert_not_called()

    @patch("tasks.dyntest.get_commit_sha", return_value="abc")
    @patch("tasks.dyntest.JevDynTestExecutor")
    @patch("tasks.dyntest.DatadogDynTestEvaluator")
    def test_initialization_failure_is_visible(self, evaluator, executor, _):
        executor.return_value.kind = IndexKind.JEV
        evaluator.return_value.initialize.return_value = False
        with self.assertRaisesRegex(Exit, "incomplete"):
            evaluate_index.body(Context(), pipeline_id="42", selector="jev", send_stats=False)
        evaluator.return_value.evaluate.assert_not_called()
