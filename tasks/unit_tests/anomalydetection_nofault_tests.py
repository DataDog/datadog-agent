import contextlib
import io
import json
import shlex
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import Mock

from invoke import Exit

from tasks.anomalydetection import eval_scenarios
from tasks.libs.anomalydetection.eval import require_f1_report, scenario_mode

HEALTHY = {
    "mode": "healthy-control",
    "expectation": "healthy",
    "evaluation_window": {"start": "2026-01-01T00:05:00Z", "end": "2026-01-01T00:35:00Z"},
    "disruptions": [],
}


class TestNoFaultEvaluation(unittest.TestCase):
    def test_requires_explicit_consistent_labels(self):
        self.assertEqual(scenario_mode(HEALTHY), "no_faults")
        self.assertEqual(scenario_mode({"disruption": {"start": "2026-01-01T00:05:00Z"}}), "faults")
        for episode in (
            {},
            {**HEALTHY, "mode": ""},
            {**HEALTHY, "disruptions": [{"fault": "yes"}]},
            {**HEALTHY, "evaluation_window": {}},
        ):
            with self.subTest(episode=episode), self.assertRaises(ValueError):
                scenario_mode(episode)

    def test_f1_optimization_rejects_counts_and_incomplete_reports(self):
        require_f1_report({"score": 0.5, "complete": True, "metadata": {"fault": {"mode": "faults"}}})
        for report in (
            None,
            {"score": None},
            {"score": 0.5, "complete": False},
            {"score": 3, "complete": True, "metadata": {"healthy": {"mode": "no_faults"}}},
        ):
            with self.subTest(report=report), self.assertRaises(ValueError):
                require_f1_report(report)

    def run_eval(self, failure=None, mixed=False):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            names = ["healthy", "fault"] if mixed else ["healthy"]
            for name in names:
                scenario = root / name
                scenario.mkdir()
                (scenario / "data.parquet").touch()
                (scenario / "episode.json").write_text(
                    json.dumps(HEALTHY if name == "healthy" else {"disruption": {"start": "2026-01-01T00:05:00Z"}})
                )
            # A stale successful output must not hide a replay which writes nothing.
            (root / "observer-eval-healthy.json").write_text("{}")
            ctx = Mock()

            def run(command, **_kwargs):
                args = shlex.split(command)
                if "--headless" in args:
                    if failure == "timeout":
                        raise type("CommandTimedOut", (Exception,), {})("test timeout")
                    if failure == "unordered":
                        return SimpleNamespace(failed=True, stderr="timestamps are not globally ordered")
                    if failure != "missing_output":
                        Path(args[args.index("--output") + 1]).write_text("{}")
                    return SimpleNamespace(failed=False)
                if failure == "scorer":
                    return SimpleNamespace(failed=True, stderr="Replay consumed no input")
                score = {"mode": "no_faults", "false_positives": 3} if "--score-no-faults" in args else {"f1": 0.5}
                return SimpleNamespace(failed=False, stdout=json.dumps(score))

            ctx.run.side_effect = run
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                report = eval_scenarios.body(
                    ctx,
                    local=True,
                    scenarios=",".join(names),
                    scenarios_dir=str(root),
                    build=False,
                    scenario_output_dir=str(root),
                    main_report_path=str(root / "report.json"),
                )
            return report, ctx, output.getvalue()

    def test_count_and_inspect_command_without_f1(self):
        report, ctx, table = self.run_eval()
        self.assertEqual(report["score"], 3)
        self.assertEqual(report["false_positives"], 3)
        self.assertTrue(report["complete"])
        row = report["metadata"]["healthy"]
        self.assertNotIn("f1", row)
        self.assertIn("--local", row["inspect_command"])
        self.assertIn("False positives", table)
        self.assertNotIn("F1", table)
        self.assertIn("--score-no-faults", ctx.run.call_args.args[0])

    def test_incomplete_evaluation_has_no_score(self):
        for failure in ("timeout", "missing_output", "scorer"):
            with self.subTest(failure=failure):
                report, _, _ = self.run_eval(failure=failure)
                self.assertFalse(report["complete"])
                self.assertIsNone(report["score"])
                self.assertNotIn("false_positives", report["metadata"]["healthy"])

    def test_unordered_replay_still_fails_without_retry(self):
        with self.assertRaisesRegex(Exit, "not globally ordered"):
            self.run_eval(failure="unordered")

    def test_mixed_dataset_keeps_scores_separate(self):
        report, _, _ = self.run_eval(mixed=True)
        self.assertTrue(report["complete"])
        self.assertIsNone(report["score"])
        self.assertEqual(report["metadata"]["fault"]["f1"], 0.5)
        self.assertEqual(report["false_positives"], 3)
