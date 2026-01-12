import unittest
from unittest.mock import MagicMock, patch

from tasks.libs.dynamic_test.evaluator import DynTestEvaluator, EvaluationResult, ExecutedTest
from tasks.libs.dynamic_test.index import IndexKind


class _FakeEvaluator(DynTestEvaluator):
    def list_tests_for_job(self, job_name: str):
        # two executed tests, one failed (reliable)
        return [
            ExecutedTest("pkg/a.TestOne", "passed", "p", "j", job_name, False),
            ExecutedTest("pkg/a.TestTwo", "failed", "p", "j", job_name, False),
        ]


class TestDynTestEvaluator(unittest.TestCase):
    def test_evaluate_builds_results(self):
        ctx = MagicMock()
        executor = MagicMock()
        # predictions per job
        executor.tests_to_run_per_job.return_value = {"job": {"pkg/a.TestTwo", "pkg/a.TestThree"}}
        # index is a field on the executor
        index = MagicMock()
        index.to_dict.return_value = {"job": {"pkg": ["ignored"]}}
        index.get_indexed_tests_for_job.return_value = {
            "pkg/a.TestOne",
            "pkg/a.TestTwo",
            "pkg/a.TestThree",
        }
        executor.index = index

        ev = _FakeEvaluator(ctx, IndexKind.PACKAGE, executor, pipeline_id="123")
        # Manually set the index since the constructor expects it
        ev.index = index
        results = ev.evaluate(["ignored"])

        self.assertEqual(len(results), 1)
        r: EvaluationResult = results[0]
        self.assertEqual(r.job_name, "job")
        self.assertEqual(r.actual_executed_tests, {"pkg/a.TestOne", "pkg/a.TestTwo"})
        self.assertEqual(r.predicted_executed_tests, {"pkg/a.TestTwo", "pkg/a.TestThree"})
        # failed test is TestTwo but it is predicted, so not_executed_failing_tests should be empty
        self.assertEqual(r.not_executed_failing_tests, set())

    @patch("tasks.libs.dynamic_test.evaluator.send_event")
    def test_initialize_success(self, mock_send_event):
        """Test successful index initialization."""
        ctx = MagicMock()
        executor = MagicMock()
        index = MagicMock()
        executor.init_index.return_value = None
        executor.index = index

        ev = _FakeEvaluator(ctx, IndexKind.PACKAGE, executor, pipeline_id="123")
        result = ev.initialize(IndexKind.PACKAGE, "commit123")

        self.assertTrue(result)
        self.assertEqual(ev.index, index)
        self.assertEqual(ev.kind, IndexKind.PACKAGE)
        executor.init_index.assert_called_once_with(IndexKind.PACKAGE, "commit123")
        mock_send_event.assert_not_called()

    @patch("tasks.libs.dynamic_test.evaluator.send_event")
    def test_initialize_index_not_found(self, mock_send_event):
        """Test error reporting when no ancestor commit with index is found."""
        ctx = MagicMock()
        executor = MagicMock()
        executor.init_index.side_effect = RuntimeError("No ancestor commit found for commit123")

        ev = _FakeEvaluator(ctx, IndexKind.PACKAGE, executor, pipeline_id="456")
        result = ev.initialize(IndexKind.PACKAGE, "commit123")

        self.assertFalse(result)
        mock_send_event.assert_called_once()

        # Check the event details
        call_args = mock_send_event.call_args
        self.assertEqual(call_args[1]["title"], "Dynamic Test Evaluator Error: index_not_found")
        self.assertIn("No ancestor commit with index found for commit123", call_args[1]["text"])
        self.assertEqual(call_args[1]["alert_type"], "error")
        self.assertIn("commit_sha:commit123", call_args[1]["tags"])
        self.assertIn("pipeline_id:456", call_args[1]["tags"])
        self.assertIn("error_type:index_not_found", call_args[1]["tags"])

    @patch("tasks.libs.dynamic_test.evaluator.send_event")
    def test_initialize_other_runtime_error(self, mock_send_event):
        """Test error reporting for other RuntimeErrors."""
        ctx = MagicMock()
        executor = MagicMock()
        executor.init_index.side_effect = RuntimeError("Backend connection failed")

        ev = _FakeEvaluator(ctx, IndexKind.PACKAGE, executor, pipeline_id="789")
        result = ev.initialize(IndexKind.PACKAGE, "commit456")

        self.assertFalse(result)
        mock_send_event.assert_called_once()

        call_args = mock_send_event.call_args
        self.assertEqual(call_args[1]["title"], "Dynamic Test Evaluator Error: index_initialization_failed")
        self.assertIn("Backend connection failed", call_args[1]["text"])
        self.assertIn("error_type:index_initialization_failed", call_args[1]["tags"])

    @patch("tasks.libs.dynamic_test.evaluator.send_event")
    def test_initialize_unexpected_error(self, mock_send_event):
        """Test error reporting for unexpected exceptions."""
        ctx = MagicMock()
        executor = MagicMock()
        executor.init_index.side_effect = ValueError("Unexpected value error")

        ev = _FakeEvaluator(ctx, IndexKind.PACKAGE, executor, pipeline_id="101")
        result = ev.initialize(IndexKind.PACKAGE, "commit789")

        self.assertFalse(result)
        mock_send_event.assert_called_once()

        call_args = mock_send_event.call_args
        self.assertEqual(call_args[1]["title"], "Dynamic Test Evaluator Error: unexpected_error")
        self.assertIn("Unexpected error initializing index: Unexpected value error", call_args[1]["text"])
        self.assertIn("error_type:unexpected_error", call_args[1]["tags"])

    @patch("tasks.libs.dynamic_test.evaluator.send_event")
    def test_send_error_fallback_on_datadog_failure(self, mock_send_event):
        """Test fallback to console logging when Datadog event sending fails."""
        mock_send_event.side_effect = Exception("Datadog API failed")
        ctx = MagicMock()
        executor = MagicMock()
        executor.init_index.side_effect = RuntimeError("Test error")

        ev = _FakeEvaluator(ctx, IndexKind.PACKAGE, executor, pipeline_id="test")

        with patch('builtins.print') as mock_print:
            result = ev.initialize(IndexKind.PACKAGE, "test_commit")

        self.assertFalse(result)
        mock_send_event.assert_called_once()
        mock_print.assert_called()  # Should print fallback messages


if __name__ == "__main__":
    unittest.main()
