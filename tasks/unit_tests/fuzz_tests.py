import unittest
from unittest.mock import MagicMock, patch

from invoke.exceptions import Exit

from tasks.fuzz import fuzz, is_fuzztime_boundary_timeout


class TestFuzz(unittest.TestCase):
    @patch("tasks.fuzz.schema_codegen")
    @patch("tasks.fuzz.search_fuzz_tests")
    def test_limits_discovery_to_requested_package(self, search, _):
        target = "pkg/rshell/builtins/tests/head"
        search.return_value = [(target, "FuzzHeadLines")]
        ctx = MagicMock()

        fuzz.body(ctx, targets=target, fuzztime="1s")

        search.assert_called_once_with(target)
        ctx.cd.assert_called_once_with(target)
        command = ctx.run.call_args.args[0]
        self.assertIn("-fuzz=FuzzHeadLines$", command)
        self.assertIn("-fuzztime=1s", command)

    @patch("tasks.fuzz.schema_codegen")
    @patch("tasks.fuzz.search_fuzz_tests")
    @patch("tasks.fuzz.subprocess.run")
    def test_jobs_run_targets_concurrently_and_report_failures(self, run, search, _):
        search.return_value = [("pkg/a", "FuzzA"), ("pkg/b", "FuzzB")]
        run.side_effect = lambda command, cwd, **_: MagicMock(
            returncode=1 if cwd == "pkg/b" else 0, stdout="--- FAIL: FuzzB (1.00s)\n    boom\n"
        )

        with self.assertRaisesRegex(Exit, "pkg/b FuzzB"):
            fuzz.body(MagicMock(), targets="pkg", jobs=2)

        self.assertEqual(run.call_count, 2)
        self.assertIn("-parallel=1", run.call_args.args[0])


class TestFuzztimeBoundaryTimeout(unittest.TestCase):
    def test_tolerates_bare_coordinator_timeout(self):
        output = "=== FUZZ  FuzzA\n--- FAIL: FuzzA (30.02s)\n    context deadline exceeded\nFAIL\n"
        self.assertTrue(is_fuzztime_boundary_timeout("FuzzA", output))

    def test_rejects_other_failures(self):
        for output in [
            "--- FAIL: FuzzA (30.02s)\n    a_test.go:12: context deadline exceeded\nFAIL\n",
            "--- FAIL: FuzzA (30.02s)\n    context deadline exceeded\n    Failing input written to testdata/fuzz/FuzzA/x\nFAIL\n",
            "context deadline exceeded\npanic: boom\n--- FAIL: FuzzA (30.02s)\n    context deadline exceeded\nFAIL\n",
            "--- FAIL: FuzzB (30.02s)\n    context deadline exceeded\nFAIL\n",
            "build failed: context deadline exceeded\n",
        ]:
            with self.subTest(output=output):
                self.assertFalse(is_fuzztime_boundary_timeout("FuzzA", output))
