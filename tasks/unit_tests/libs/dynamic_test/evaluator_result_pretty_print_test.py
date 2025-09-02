import unittest
from contextlib import redirect_stdout
from io import StringIO

from tasks.libs.dynamic_test.evaluator import EvaluationResult


class TestEvaluationResultPrettyPrint(unittest.TestCase):
    def test_git_style_diff_and_failed_warning(self):
        res = EvaluationResult(
            job_name="job-a",
            actual_executed_tests={"pkg/a.TestOne", "pkg/a.TestTwo"},
            predicted_executed_tests={"pkg/a.TestTwo", "pkg/a.TestThree"},
            not_executed_failing_tests={"pkg/a.TestOne"},
        )

        buf = StringIO()
        with redirect_stdout(buf):
            res.pretty_print()
        out = buf.getvalue()

        self.assertIn("[Job] job-a", out)
        self.assertIn("--- actual", out)
        self.assertIn("+++ predicted", out)
        # common/context
        self.assertIn("  pkg/a.TestTwo", out)
        # removed and flagged failed
        self.assertIn("- pkg/a.TestOne", out)
        self.assertIn("[FAILED]", out)
        # added
        self.assertIn("+ pkg/a.TestThree", out)
        # summary
        self.assertIn("summary: actual=2 predicted=2", out)


if __name__ == "__main__":
    unittest.main()
