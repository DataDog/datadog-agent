import json
import os
import tempfile
import unittest

from tasks.anomalydetection import (
    _bayesian_evaluation_inputs,
    _load_completed_bayesian_report,
)
from tasks.libs.anomalydetection.eval import (
    ABLATION_CORRELATORS,
    SUPPORTED_CORRELATORS,
    _build_optuna_config,
    _combo_to_config,
)


class TestAblationConfig(unittest.TestCase):
    def test_generated_configs_enable_scorer_and_disable_time_cluster(self):
        self.assertEqual(ABLATION_CORRELATORS, ["anomaly_scorer"])
        self.assertEqual(SUPPORTED_CORRELATORS, ["anomaly_scorer", "cross_signal", "time_cluster"])

        configs = {
            "combination": _combo_to_config(detectors=["bocpd"], correlators=["anomaly_scorer"]),
            "optuna": _build_optuna_config(
                trial=None,
                components=["anomaly_scorer"],
                locked={"anomaly_scorer"},
            ),
        }

        for name, config in configs.items():
            with self.subTest(name=name):
                components = config["components"]
                self.assertTrue(components["anomaly_scorer"]["enabled"])
                self.assertFalse(components["time_cluster"]["enabled"])

        manual = _build_optuna_config(
            trial=None,
            components=["cross_signal", "time_cluster"],
            locked={"cross_signal", "time_cluster"},
        )["components"]
        self.assertTrue(manual["cross_signal"]["enabled"])
        self.assertTrue(manual["time_cluster"]["enabled"])
        self.assertFalse(manual["anomaly_scorer"]["enabled"])


class TestPipelineResume(unittest.TestCase):
    def setUp(self):
        self.evaluation_inputs = _bayesian_evaluation_inputs(
            scenarios_dir="/tmp/scenarios",
            sigma=30.0,
            timeout=0,
            scenarios="scenario-a",
            lock="",
            eval_backend="local",
            ddeval_options=None,
        )

    def _write_report(self, output_dir, **overrides):
        report = {
            "n_trials": 5,
            "completed_trials": 5,
            "failed_trials": 0,
            "seed": 42,
            "components": ["anomaly_scorer", "bocpd"],
            "eval_backend": "local",
            "evaluation_inputs": self.evaluation_inputs,
        }
        report.update(overrides)
        with open(os.path.join(output_dir, "report.json"), "w") as f:
            json.dump(report, f)
        return report

    def test_resume_only_reuses_complete_matching_reports(self):
        cases = [
            ("matching", {}, self.evaluation_inputs, True),
            ("partial", {"completed_trials": 3}, self.evaluation_inputs, False),
            ("changed inputs", {}, {**self.evaluation_inputs, "sigma": 60.0}, False),
        ]

        for name, report_overrides, evaluation_inputs, should_reuse in cases:
            with self.subTest(name=name), tempfile.TemporaryDirectory() as output_dir:
                expected = self._write_report(output_dir, **report_overrides)
                actual = _load_completed_bayesian_report(
                    output_dir,
                    components=["bocpd", "anomaly_scorer"],
                    n_trials=5,
                    seed=42,
                    eval_backend="local",
                    evaluation_inputs=evaluation_inputs,
                )

                if should_reuse:
                    self.assertEqual(actual, expected)
                else:
                    self.assertIsNone(actual)


if __name__ == "__main__":
    unittest.main()
