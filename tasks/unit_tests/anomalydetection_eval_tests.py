import json
import os
import tempfile
import unittest

from tasks.anomalydetection import (
    _bayesian_evaluation_inputs,
    _ddeval_experiment_config,
    _DDEvalOptions,
    _load_completed_bayesian_report,
)
from tasks.libs.anomalydetection.eval import _build_optuna_config, _combo_to_config


class TestAblationConfig(unittest.TestCase):
    def test_time_cluster_is_explicitly_disabled(self):
        config = _combo_to_config(
            detectors=["bocpd"],
            correlators=["anomaly_scorer"],
        )

        components = config["components"]
        self.assertTrue(components["anomaly_scorer"]["enabled"])
        self.assertFalse(components["time_cluster"]["enabled"])

    def test_time_cluster_is_explicitly_disabled_in_optuna_config(self):
        config = _build_optuna_config(
            trial=None,
            components=["anomaly_scorer"],
            locked={"anomaly_scorer"},
        )

        components = config["components"]
        self.assertTrue(components["anomaly_scorer"]["enabled"])
        self.assertFalse(components["time_cluster"]["enabled"])


class TestPipelineResume(unittest.TestCase):
    def setUp(self):
        self.evaluation_inputs = _bayesian_evaluation_inputs(
            scenarios_dir="/tmp/scenarios",
            sigma=30.0,
            timeout=0,
            scenarios="",
            lock="",
            eval_backend="ddeval",
            ddeval_options=ddeval_options(),
        )

    def _write_report(self, output_dir, **overrides):
        report = {
            "n_trials": 5,
            "completed_trials": 5,
            "failed_trials": 0,
            "seed": 42,
            "components": ["anomaly_scorer", "bocpd"],
            "eval_backend": "ddeval",
            "evaluation_inputs": self.evaluation_inputs,
        }
        report.update(overrides)
        with open(os.path.join(output_dir, "report.json"), "w") as f:
            json.dump(report, f)
        return report

    def test_loads_fully_completed_matching_report(self):
        with tempfile.TemporaryDirectory() as output_dir:
            expected = self._write_report(output_dir)

            actual = _load_completed_bayesian_report(
                output_dir,
                components=["bocpd", "anomaly_scorer"],
                n_trials=5,
                seed=42,
                eval_backend="ddeval",
                evaluation_inputs=self.evaluation_inputs,
            )

        self.assertEqual(actual, expected)

    def test_rejects_partial_report(self):
        with tempfile.TemporaryDirectory() as output_dir:
            self._write_report(output_dir, completed_trials=3)

            actual = _load_completed_bayesian_report(
                output_dir,
                components=["anomaly_scorer", "bocpd"],
                n_trials=5,
                seed=42,
                eval_backend="ddeval",
                evaluation_inputs=self.evaluation_inputs,
            )

        self.assertIsNone(actual)

    def test_rejects_report_with_different_evaluation_inputs(self):
        with tempfile.TemporaryDirectory() as output_dir:
            self._write_report(output_dir)
            changed_inputs = {**self.evaluation_inputs, "sigma": 60.0}

            actual = _load_completed_bayesian_report(
                output_dir,
                components=["anomaly_scorer", "bocpd"],
                n_trials=5,
                seed=42,
                eval_backend="ddeval",
                evaluation_inputs=changed_inputs,
            )

        self.assertIsNone(actual)


class TestDDEvalConfig(unittest.TestCase):
    def test_uses_worker_testbench_config_input(self):
        options = ddeval_options()
        trial_config = {"components": {"anomaly_scorer": {"enabled": True}}}

        config = _ddeval_experiment_config(
            options=options,
            trial_config=trial_config,
            trial_config_path="/tmp/config.json",
            sigma=30.0,
        )

        input_parameters = config["input_parameters"]
        self.assertEqual(input_parameters["testbench_config"], trial_config)
        self.assertNotIn("component_config", input_parameters)


def ddeval_options(**overrides):
    values = {
        "config_template": "",
        "ddsource_dir": "",
        "command": "ddeval",
        "service": "eval-worker",
        "project": "observer-log-ad",
        "dataset": "dataset",
        "env": "staging",
        "test_drive": "test-drive",
        "jobs": 9,
        "max_attempts": 3,
        "limit": 0,
        "where_in": "",
        "testbench_binary_s3_uri": "s3://bucket/testbench",
        "scorer_binary_s3_uri": "s3://bucket/scorer",
    }
    values.update(overrides)
    return _DDEvalOptions(**values)


if __name__ == "__main__":
    unittest.main()
