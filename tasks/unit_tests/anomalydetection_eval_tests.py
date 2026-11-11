import json
import os
import shlex
import tempfile
import unittest
from types import SimpleNamespace
from unittest.mock import Mock, patch

from invoke.exceptions import Exit

from tasks.anomalydetection import (
    _bayesian_evaluation_inputs,
    _load_completed_bayesian_report,
    _run_bayesian_runs,
    eval_bayesian,
    eval_combinations,
    eval_component,
    eval_pipeline,
    eval_scenarios,
    eval_tp,
    launch_testbench,
)
from tasks.libs.anomalydetection.eval import (
    ABLATION_CORRELATORS,
    DETECTORS,
    F1_SCORING_VERSION,
    SUPPORTED_CORRELATORS,
    _anchor_combos,
    _build_optuna_config,
    _combo_to_config,
    _full_stack_combo,
    _sample_component_params,
    random_component_combinations,
    validate_workflow_components,
)


class MinimumTrial:
    def suggest_float(self, _name, low, _high, **_kwargs):
        return low

    def suggest_int(self, _name, low, _high, **_kwargs):
        return low


class TestAblationConfig(unittest.TestCase):
    def test_generated_configs_enable_scorer_and_disable_time_cluster(self):
        self.assertEqual(ABLATION_CORRELATORS, ["anomaly_scorer"])
        self.assertEqual(SUPPORTED_CORRELATORS, ["anomaly_scorer", "time_cluster"])

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
                self.assertEqual(components["anomaly_scorer"]["correlation_event_threshold"], "high")
                self.assertTrue(components["anomaly_scorer"]["correlation_events"])
                self.assertEqual(components["anomaly_scorer"]["cooldown_secs"], 0)
                self.assertFalse(components["time_cluster"]["enabled"])

        manual = _build_optuna_config(trial=None, components=["time_cluster"], locked={"time_cluster"})["components"]
        self.assertTrue(manual["time_cluster"]["enabled"])
        self.assertFalse(manual["anomaly_scorer"]["enabled"])

    def test_optuna_keeps_high_severity_episode_contract(self):
        # This trial has no categorical sampler: severity is a fixed evaluation
        # contract, not a parameter that Optuna may change to medium.
        config = _sample_component_params(MinimumTrial(), "anomaly_scorer")
        self.assertEqual(config["correlation_event_threshold"], "high")
        self.assertTrue(config["correlation_events"])
        self.assertEqual(config["cooldown_secs"], 0)
        self.assertLess(config["low_threshold"], config["high_threshold"])

    def test_force_enabled_time_cluster_is_preserved(self):
        combos = [
            _full_stack_combo(force_enable=["time_cluster"]),
            *_anchor_combos(force_enable=["time_cluster"]),
            *random_component_combinations(5, seed=42, force_enable=["time_cluster"]),
        ]
        self.assertTrue(combos)
        self.assertTrue(all("time_cluster" in combo["correlators"] for combo in combos))

        config = _combo_to_config(detectors=["bocpd"], correlators=["time_cluster"])
        self.assertTrue(config["components"]["time_cluster"]["enabled"])

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
        self.assertEqual(self.evaluation_inputs["scoring_version"], F1_SCORING_VERSION)
        cases = [
            ("matching", {}, self.evaluation_inputs, True),
            ("partial", {"completed_trials": 3}, self.evaluation_inputs, False),
            ("different workflow", {}, {**self.evaluation_inputs, "use_metric_pipeline": True}, False),
            (
                "missing workflow identity",
                {"evaluation_inputs": {k: v for k, v in self.evaluation_inputs.items() if k != "use_metric_pipeline"}},
                self.evaluation_inputs,
                False,
            ),
            ("changed inputs", {}, {**self.evaluation_inputs, "sigma": 60.0}, False),
            (
                "five-minute-capped recovery contract",
                {
                    "evaluation_inputs": {
                        **self.evaluation_inputs,
                        "scoring_version": "high-severity-onset-recovery-v1",
                    }
                },
                self.evaluation_inputs,
                False,
            ),
            (
                "old scoring contract",
                {"evaluation_inputs": {k: v for k, v in self.evaluation_inputs.items() if k != "scoring_version"}},
                self.evaluation_inputs,
                False,
            ),
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


class TestLocalF1Pipeline(unittest.TestCase):
    def test_missing_scenarios_abort_before_build_or_output_changes(self):
        tasks = [
            (eval_scenarios, {"local": True}),
            (eval_tp, {"only": "bocpd"}),
            (eval_combinations, {}),
            (eval_bayesian, {}),
            (eval_pipeline, {}),
            (eval_component, {"component": "bocpd"}),
        ]
        for task, kwargs in tasks:
            with (
                self.subTest(task=task.name),
                patch.dict("sys.modules", {"optuna": Mock()}),
                patch("tasks.anomalydetection.build_testbench") as build,
                patch("tasks.anomalydetection._prepare_eval_output_dir") as prepare_output,
                self.assertRaisesRegex(Exit, "Specify --scenario or --scenarios"),
            ):
                task.body(Mock(), **kwargs)
            build.assert_not_called()
            prepare_output.assert_not_called()

    def test_ddeval_keeps_its_remote_dataset_default(self):
        for task in (eval_bayesian, eval_pipeline):
            with (
                self.subTest(task=task.name),
                patch.dict("sys.modules", {"optuna": Mock()}),
                patch("tasks.anomalydetection._remote_ablation") as remote,
            ):
                task.body(Mock(), eval_backend="ddeval")
            remote.assert_called_once()
            self.assertEqual(remote.call_args.kwargs["dataset"], "Golden 25")
        inputs = _bayesian_evaluation_inputs(
            scenarios_dir="/unused",
            sigma=30.0,
            timeout=0,
            scenarios="",
            lock="",
            eval_backend="ddeval",
        )
        self.assertEqual(inputs["scenarios"], [])

    def test_combinations_forward_explicit_scenarios_to_replay(self):
        with (
            tempfile.TemporaryDirectory() as output_dir,
            patch("tasks.anomalydetection.eval_scenarios", return_value={"score": 1.0}) as replay,
            patch("builtins.print"),
        ):
            eval_combinations.body(Mock(), n=1, output_dir=output_dir, build=False, scenarios="scenario-a,scenario-b")
        self.assertEqual(replay.call_args.kwargs["scenarios"], "scenario-a,scenario-b")
        self.assertEqual(replay.call_args.kwargs["_logger"].total, 2)

    def test_only_includes_scorer_and_preserves_recovery_results(self):
        with tempfile.TemporaryDirectory() as root:
            parquet = os.path.join(root, "example", "parquet")
            os.makedirs(parquet)
            with open(os.path.join(parquet, "fixture.parquet"), "w") as f:
                f.write("unused by mocked replay")
            with open(os.path.join(root, "example", "episode.json"), "w") as f:
                json.dump({"disruption": {"start": "2026-01-01T00:05:00Z"}}, f)
            score = {
                "scoring_version": F1_SCORING_VERSION,
                "f1": 0.5,
                "precision": 1 / 3,
                "recall": 1,
                "num_predictions": 3,
                "num_baseline_fps": 1,
                "num_recovery_fps": 1,
                "num_filtered_warmup": 0,
                "num_filtered_cascading": 0,
                "num_filtered_recovery": 0,
            }
            ctx = Mock()

            def run(command, **_kwargs):
                if "--headless" in command:
                    with open(os.path.join(root, "observer-eval-example.json"), "w") as output:
                        json.dump({}, output)
                    return SimpleNamespace(failed=False)
                return SimpleNamespace(failed=False, stdout=json.dumps(score))

            ctx.run.side_effect = run
            with patch("builtins.print"):
                report = eval_scenarios.body(
                    ctx,
                    scenario="example",
                    local=True,
                    scenarios_dir=root,
                    only="bocpd",
                    build=False,
                    main_report_path=os.path.join(root, "report.json"),
                    scenario_output_dir=root,
                )
            self.assertIn("--only anomaly_scorer,bocpd", ctx.run.call_args_list[0].args[0])
            self.assertEqual(report["metadata"]["example"]["num_recovery_fps"], 1)
            self.assertEqual(report["metadata"]["example"]["scoring_version"], F1_SCORING_VERSION)


class TestMetricWorkflow(unittest.TestCase):
    @staticmethod
    def _fault_replay_run(root, score):
        output_path = os.path.join(root, "observer-eval-example.json")

        def run(command, **_kwargs):
            if "--headless" in command:
                with open(output_path, "w") as output:
                    json.dump({}, output)
                return SimpleNamespace(failed=False)
            return SimpleNamespace(failed=False, stdout=json.dumps(score))

        return run

    def test_launch_forwards_workflow_independently_of_config(self):
        for pipeline in (False, True):
            for config in ("", "/tmp/params.json"):
                with self.subTest(pipeline=pipeline, config=config), patch("builtins.print"):
                    ctx = Mock()
                    ctx.run.return_value = SimpleNamespace(failed=False)
                    launch_testbench.body(
                        ctx,
                        use_metric_pipeline=pipeline,
                        headless_scenario="example",
                        config=config,
                        local=True,
                    )
                    command = ctx.run.call_args.args[0]
                    self.assertEqual("--use-metric-pipeline" in shlex.split(command), pipeline)
                    self.assertNotIn("--only", command)
                    self.assertEqual("--config" in command, bool(config))

    def test_pipeline_combinations_do_not_search_legacy_detectors(self):
        full = _full_stack_combo(use_metric_pipeline=True)
        self.assertEqual(full["detectors"], ["metric_pipeline"])
        self.assertEqual(_anchor_combos(use_metric_pipeline=True), [])
        for combo in random_component_combinations(1, seed=42, use_metric_pipeline=True):
            self.assertEqual(combo["detectors"], ["metric_pipeline"])
        config = _build_optuna_config(None, ["metric_pipeline", "anomaly_scorer"], {"anomaly_scorer"})
        self.assertNotIn("metric_pipeline", config["components"])
        for name in DETECTORS:
            self.assertFalse(config["components"][name]["enabled"])
        with self.assertRaisesRegex(ValueError, "unavailable"):
            validate_workflow_components(True, "scanmw")

    def test_remote_workflow_selection_is_rejected_before_submission(self):
        for task in (eval_bayesian, eval_pipeline):
            with self.subTest(task=task.name), patch("tasks.anomalydetection._remote_ablation") as remote:
                with self.assertRaisesRegex(Exit, "only for local"):
                    task.body(Mock(), eval_backend="ddeval", use_metric_pipeline=True)
                remote.assert_not_called()

    def test_nested_bayesian_calls_forward_workflow(self):
        with tempfile.TemporaryDirectory() as root, patch("tasks.anomalydetection.eval_bayesian") as evaluate:
            evaluate.return_value = {"completed_trials": 1, "score": 1.0}
            _run_bayesian_runs(
                Mock(),
                ["metric_pipeline", "anomaly_scorer"],
                1,
                1,
                [42],
                root,
                "/tmp/scenarios",
                30,
                0,
                "example",
                use_metric_pipeline=True,
            )
            self.assertTrue(evaluate.call_args.kwargs["use_metric_pipeline"])

    def test_combinations_forward_workflow(self):
        with (
            tempfile.TemporaryDirectory() as root,
            patch("builtins.print"),
            patch("tasks.anomalydetection.eval_scenarios") as replay,
        ):
            replay.return_value = {"score": 1.0}
            eval_combinations.body(
                Mock(), n=1, build=False, output_dir=root, scenarios="example", use_metric_pipeline=True
            )
            self.assertTrue(replay.call_args.kwargs["use_metric_pipeline"])
            with open(replay.call_args.kwargs["config"]) as f:
                config = json.load(f)
            self.assertTrue(config["use_metric_pipeline"])
            self.assertTrue(all(not config["components"][name]["enabled"] for name in DETECTORS))

    def test_tp_evaluation_forwards_workflow(self):
        with (
            patch("builtins.print"),
            patch("tasks.anomalydetection.os.path.isdir", return_value=True),
            patch("tasks.anomalydetection.os.listdir", return_value=["fixture.parquet"]),
            patch("tasks.anomalydetection.os.path.isfile", return_value=False),
        ):
            ctx = Mock()
            eval_tp.body(ctx, scenario="example", build=False, use_metric_pipeline=True)
            self.assertIn("--use-metric-pipeline", ctx.run.call_args.args[0])
            self.assertIn("passthrough", ctx.run.call_args.args[0])

    def test_pipeline_search_and_tuning_forward_workflow(self):
        with (
            tempfile.TemporaryDirectory() as root,
            patch("builtins.print"),
            patch("tasks.anomalydetection._run_bayesian_runs") as search,
            patch("tasks.anomalydetection.eval_bayesian") as tune,
        ):
            search.return_value = {"max_score": 1.0, "mean_score": 1.0, "failed_runs": 0, "run_details": []}
            tune.return_value = {"completed_trials": 1, "score": 1.0}
            report = eval_pipeline.body(
                Mock(),
                n_combos=1,
                n_trials_search=1,
                n_trials_tune=1,
                output_dir=root,
                build=False,
                seed=42,
                scenarios="example",
                use_metric_pipeline=True,
            )
            self.assertTrue(search.call_args.kwargs["use_metric_pipeline"])
            self.assertTrue(tune.call_args.kwargs["use_metric_pipeline"])
            self.assertIn("metric_pipeline", search.call_args.kwargs["components_list"])
            self.assertTrue(report["use_metric_pipeline"])

    def test_component_comparisons_forward_workflow(self):
        with (
            tempfile.TemporaryDirectory() as root,
            patch("builtins.print"),
            patch("tasks.anomalydetection._run_bayesian_runs") as evaluate,
        ):
            evaluate.return_value = {
                "max_score": 1.0,
                "mean_score": 1.0,
                "failed_runs": 0,
                "run_details": [],
                "run_scores": [1.0],
            }
            eval_component.body(
                Mock(),
                component="log_pattern_extractor",
                n_subsets=1,
                n_trials=1,
                output_dir=root,
                build=False,
                seed=42,
                scenarios="example",
                use_metric_pipeline=True,
            )
            self.assertEqual(evaluate.call_count, 2)
            for call in evaluate.call_args_list:
                self.assertTrue(call.kwargs["use_metric_pipeline"])
                self.assertIn("metric_pipeline", call.kwargs["components_list"])

    def test_headless_evaluation_forwards_workflow(self):
        with tempfile.TemporaryDirectory() as root, patch("builtins.print"):
            os.makedirs(os.path.join(root, "example", "parquet"))
            with open(os.path.join(root, "example", "episode.json"), "w") as f:
                json.dump({"disruption": {"start": "2026-01-01T00:05:00Z"}}, f)
            with open(os.path.join(root, "example", "parquet", "fixture.parquet"), "w") as f:
                f.write("fixture")
            ctx = Mock()
            score = {
                "f1": 1.0,
                "precision": 1.0,
                "recall": 1.0,
                "num_predictions": 1,
                "num_baseline_fps": 0,
                "num_filtered_warmup": 0,
                "num_filtered_cascading": 0,
            }
            ctx.run.side_effect = self._fault_replay_run(root, score)
            report = eval_scenarios.body(
                ctx,
                scenario="example",
                scenarios_dir=root,
                build=False,
                local=True,
                scenario_output_dir=root,
                main_report_path=os.path.join(root, "report.json"),
                use_metric_pipeline=True,
            )
            self.assertIn("--use-metric-pipeline", shlex.split(ctx.run.call_args_list[0].args[0]))
            self.assertTrue(report["use_metric_pipeline"])
            self.assertIn(
                "--use-metric-pipeline",
                shlex.split(report["metadata"]["example"]["inspect_command"]),
            )

    def test_headless_evaluation_preserves_workflow_from_config_and_inspection(self):
        with tempfile.TemporaryDirectory() as root, patch("builtins.print"):
            os.makedirs(os.path.join(root, "example", "parquet"))
            with open(os.path.join(root, "example", "episode.json"), "w") as f:
                json.dump({"disruption": {"start": "2026-01-01T00:05:00Z"}}, f)
            with open(os.path.join(root, "example", "parquet", "fixture.parquet"), "w") as f:
                f.write("fixture")
            config_path = os.path.join(root, "config.json")
            with open(config_path, "w") as f:
                json.dump({"use_metric_pipeline": True, "components": {}}, f)
            ctx = Mock()
            score = {
                "f1": 1.0,
                "precision": 1.0,
                "recall": 1.0,
                "num_predictions": 1,
                "num_baseline_fps": 0,
                "num_filtered_warmup": 0,
                "num_filtered_cascading": 0,
            }
            ctx.run.side_effect = self._fault_replay_run(root, score)
            report = eval_scenarios.body(
                ctx,
                scenario="example",
                scenarios_dir=root,
                config=config_path,
                build=False,
                local=True,
                scenario_output_dir=root,
                main_report_path=os.path.join(root, "report.json"),
            )
            self.assertIn("--use-metric-pipeline", shlex.split(ctx.run.call_args_list[0].args[0]))
            self.assertTrue(report["use_metric_pipeline"])
            inspect_command = report["metadata"]["example"]["inspect_command"]
            self.assertIn("--use-metric-pipeline", shlex.split(inspect_command))


if __name__ == "__main__":
    unittest.main()
