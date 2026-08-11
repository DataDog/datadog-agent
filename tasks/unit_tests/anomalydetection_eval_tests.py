import json
import os
import shlex
import tempfile
import unittest
from types import SimpleNamespace
from unittest.mock import Mock

from invoke import Context

from tasks.anomalydetection import (
    _bayesian_evaluation_inputs,
    _load_completed_bayesian_report,
    eval_scenarios,
)
from tasks.libs.anomalydetection.eval import (
    ABLATION_CORRELATORS,
    DETECTORS,
    SUPPORTED_CORRELATORS,
    _anchor_combos,
    _build_optuna_config,
    _combo_to_config,
    _full_stack_combo,
    _sample_component_params,
    default_eval_config,
    random_component_combinations,
)


class MinimumTrial:
    def suggest_float(self, _name, low, _high, **_kwargs):
        return low

    def suggest_int(self, _name, low, _high, **_kwargs):
        return low


class TestAblationConfig(unittest.TestCase):
    def test_default_eval_config_uses_scorer_and_keeps_testbench_defaults(self):
        components = default_eval_config()["components"]

        self.assertEqual(set(components), {"anomaly_scorer", "time_cluster"})
        self.assertTrue(components["anomaly_scorer"]["enabled"])
        self.assertTrue(components["anomaly_scorer"]["correlation_events"])
        self.assertEqual(components["anomaly_scorer"]["cooldown_secs"], 0)
        self.assertFalse(components["time_cluster"]["enabled"])

    def test_eval_scenarios_passes_default_scorer_config_to_testbench(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            scenario = "scenario-a"
            parquet_dir = os.path.join(tmpdir, scenario, "parquet")
            os.makedirs(parquet_dir)
            with open(os.path.join(parquet_dir, "input.parquet"), "w"):
                pass

            report_path = os.path.join(tmpdir, "report.json")

            def run(command, **_kwargs):
                args = shlex.split(command)
                if args[0] == "bin/anomalydetection-testbench":
                    output_path = args[args.index("--output") + 1]
                    with open(output_path, "w") as f:
                        json.dump({}, f)
                    return SimpleNamespace(failed=False, stderr="", stdout="")
                return SimpleNamespace(
                    failed=False,
                    stderr="",
                    stdout=(
                        '{"f1": 1.0, "precision": 1.0, "recall": 1.0, '
                        '"alpha": 0, "num_predictions": 1, "num_baseline_fps": 0, '
                        '"num_filtered_warmup": 0, "num_filtered_cascading": 0}'
                    ),
                )

            ctx = Context()
            ctx.run = Mock(side_effect=run)
            eval_scenarios(
                ctx,
                scenario=scenario,
                scenarios_dir=tmpdir,
                build=False,
                main_report_path=report_path,
                scenario_output_dir=tmpdir,
            )

            testbench_args = shlex.split(ctx.run.call_args_list[0].args[0])
            config_path = testbench_args[testbench_args.index("--config") + 1]
            with open(config_path) as f:
                components = json.load(f)["components"]
            self.assertTrue(components["anomaly_scorer"]["enabled"])
            self.assertFalse(components["time_cluster"]["enabled"])

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

    def test_force_enabled_manual_correlators_are_preserved(self):
        for correlator in ("cross_signal", "time_cluster"):
            with self.subTest(correlator=correlator):
                combos = [
                    _full_stack_combo(force_enable=[correlator]),
                    *_anchor_combos(force_enable=[correlator]),
                    *random_component_combinations(5, seed=42, force_enable=[correlator]),
                ]
                self.assertTrue(combos)
                self.assertTrue(all(correlator in combo["correlators"] for combo in combos))

                config = _combo_to_config(detectors=["bocpd"], correlators=[correlator])
                self.assertTrue(config["components"][correlator]["enabled"])

    def test_robust_detectors_have_constrained_tuning_spaces(self):
        self.assertIn("holt_residual", DETECTORS)
        self.assertIn("tukey_biweight", DETECTORS)

        trial = MinimumTrial()
        holt = _sample_component_params(trial, "holt_residual")
        tukey = _sample_component_params(trial, "tukey_biweight")

        self.assertLessEqual(holt["beta"], holt["alpha"])
        self.assertNotIn("residual_window", holt)
        self.assertNotIn("window_size", tukey)
        self.assertNotIn("min_points", tukey)

    def test_generated_configs_include_testbench_warmup_profile(self):
        components = _build_optuna_config(
            trial=None,
            components=["anomaly_scorer"],
            locked={"anomaly_scorer"},
        )["components"]

        self.assertEqual(components["bocpd"]["warmup_points"], 40)
        self.assertEqual(components["holt_residual"]["warmup_points"], 15)
        self.assertEqual(components["holt_residual"]["residual_window"], 25)
        self.assertEqual(components["tukey_biweight"]["window_size"], 40)
        self.assertEqual(components["tukey_biweight"]["min_points"], 40)

    def test_optuna_cannot_override_testbench_warmup_profile(self):
        components = _build_optuna_config(
            trial=MinimumTrial(),
            components=["bocpd", "holt_residual", "tukey_biweight"],
            locked=set(),
        )["components"]

        self.assertEqual(components["bocpd"]["warmup_points"], 40)
        self.assertEqual(components["holt_residual"]["warmup_points"], 15)
        self.assertEqual(components["holt_residual"]["residual_window"], 25)
        self.assertEqual(components["tukey_biweight"]["window_size"], 40)
        self.assertEqual(components["tukey_biweight"]["min_points"], 40)


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
