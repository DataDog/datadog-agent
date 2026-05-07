import json
import os
import tempfile
import unittest

from tasks.libs.q.eval import DEFAULT_STACK_CORRELATORS, DEFAULT_STACK_DETECTORS, DETECTORS, summarize_eval_reports
from tasks.q import _candidate_eval_stack


def _write_report(path, score, rows):
    with open(path, "w") as f:
        json.dump(
            {
                "score": score,
                "metadata": rows,
                "component_configs": None,
            },
            f,
        )


class TestObserverEvalSummaries(unittest.TestCase):
    def test_manual_eval_candidates_are_known_detectors(self):
        for name in [
            "holt_residual",
            "tukey_biweight",
        ]:
            self.assertIn(name, DETECTORS)

    def test_incremental_default_stack_matches_manual_eval_baseline(self):
        self.assertEqual(DEFAULT_STACK_DETECTORS, ["bocpd"])
        self.assertEqual(DEFAULT_STACK_CORRELATORS, ["time_cluster"])

    def test_candidate_eval_stack_replaces_metric_detector(self):
        detectors, correlators = _candidate_eval_stack(["tukey_biweight"])

        self.assertEqual(detectors, ["tukey_biweight"])
        self.assertEqual(correlators, ["time_cluster"])

    def test_candidate_eval_stack_keeps_default_detector_for_correlator(self):
        detectors, correlators = _candidate_eval_stack(["cross_signal"])

        self.assertEqual(detectors, ["bocpd"])
        self.assertEqual(correlators, ["cross_signal", "time_cluster"])

    def test_summarize_eval_reports_computes_matrix_fields(self):
        with tempfile.TemporaryDirectory() as tmp:
            baseline_path = os.path.join(tmp, "baseline.json")
            candidate_path = os.path.join(tmp, "candidate.json")
            _write_report(
                baseline_path,
                0.2,
                {
                    "scenario_a": {
                        "f1": 0.1,
                        "precision": 0.2,
                        "recall": 0.3,
                        "num_predictions": 2,
                        "num_baseline_fps": 1,
                        "num_filtered_cascading": 5,
                        "tp": 0.5,
                        "fp": 1,
                        "fn": 0.5,
                    },
                    "scenario_b": {
                        "f1": 0.3,
                        "precision": 0.4,
                        "recall": 0.5,
                        "num_predictions": 4,
                        "num_baseline_fps": 2,
                        "num_filtered_cascading": 6,
                        "tp": 0.7,
                        "fp": 2,
                        "fn": 0.3,
                    },
                },
            )
            _write_report(
                candidate_path,
                0.35,
                {
                    "scenario_a": {
                        "f1": 0.4,
                        "precision": 0.5,
                        "recall": 0.6,
                        "num_predictions": 3,
                        "num_baseline_fps": 1,
                        "num_filtered_cascading": 7,
                        "tp": 0.8,
                        "fp": 1,
                        "fn": 0.2,
                    },
                    "scenario_b": {
                        "f1": 0.2,
                        "precision": 0.3,
                        "recall": 0.4,
                        "num_predictions": 5,
                        "num_baseline_fps": 4,
                        "num_filtered_cascading": 8,
                        "tp": 0.6,
                        "fp": 4,
                        "fn": 0.4,
                    },
                },
            )

            matrix = summarize_eval_reports(
                [("baseline", baseline_path), ("candidate", candidate_path)],
                baseline_name="baseline",
            )

        rows = {row["name"]: row for row in matrix["rows"]}
        candidate = rows["candidate"]
        self.assertAlmostEqual(candidate["delta_score"], 0.15)
        self.assertAlmostEqual(candidate["median_f1"], 0.3)
        self.assertAlmostEqual(candidate["median_delta_f1"], 0.1)
        self.assertAlmostEqual(candidate["worst_delta_f1"], -0.1)
        self.assertEqual(candidate["num_predictions"], 8)
        self.assertEqual(candidate["num_baseline_fps"], 5)
        self.assertEqual(candidate["baseline_fp_delta"], 2)
        self.assertEqual(candidate["num_filtered_cascading"], 15)
        self.assertEqual(len(candidate["per_scenario"]), 2)


if __name__ == "__main__":
    unittest.main()
