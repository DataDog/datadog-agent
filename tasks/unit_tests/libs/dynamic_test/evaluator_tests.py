import unittest
from unittest.mock import MagicMock

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
        results = ev.evaluate(["ignored"])

        self.assertEqual(len(results), 1)
        r: EvaluationResult = results[0]
        self.assertEqual(r.job_name, "job")
        self.assertEqual(r.actual_executed_tests, {"pkg/a.TestOne", "pkg/a.TestTwo"})
        self.assertEqual(r.predicted_executed_tests, {"pkg/a.TestTwo", "pkg/a.TestThree"})
        # failed test is TestTwo but it is predicted, so not_executed_failing_tests should be empty
        self.assertEqual(r.not_executed_failing_tests, set())


if __name__ == "__main__":
    unittest.main()
