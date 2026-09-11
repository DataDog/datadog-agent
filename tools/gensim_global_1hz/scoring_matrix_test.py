import json
import unittest
from collections import Counter, defaultdict
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
MATRIX = ROOT / "docs/dev/gensim-global-1hz-scoring-matrix.json"
MARKDOWN = ROOT / "docs/dev/gensim-global-1hz-scoring-matrix.md"
CORPUS_MATRIX = ROOT / "docs/dev/gensim-global-1hz-corpus-matrix.md"


class ScoringMatrixTest(unittest.TestCase):
    def setUp(self):
        self.matrix = json.loads(MATRIX.read_text())
        self.arms = self.matrix["arms"]
        self.hypotheses = self.matrix["scenario_hypotheses"]

    def test_matrix_has_exact_paired_panel(self):
        self.assertEqual(1, self.matrix["schema_version"])
        self.assertEqual("global_1hz_unblinded_scoring_matrix", self.matrix["kind"])
        self.assertEqual("evaluator_only_not_agent_visible", self.matrix["assignment_visibility"])
        self.assertEqual(15, len(self.hypotheses))
        self.assertEqual(30, len(self.arms))
        self.assertEqual(30, len({arm["run_id"] for arm in self.arms}))

        by_episode = defaultdict(list)
        for arm in self.arms:
            by_episode[arm["episode_id"]].append(arm)
        self.assertEqual(15, len(by_episode))
        for episode_id, arms in by_episode.items():
            with self.subTest(episode_id=episode_id):
                self.assertEqual({"arm-a", "arm-b"}, {arm["arm"] for arm in arms})
                self.assertEqual({False, True}, {arm["uses_1hz"] for arm in arms})
                self.assertEqual(1, sum(arm["uses_1hz"] for arm in arms))

    def test_hypotheses_are_scenario_level_and_explain_direction(self):
        expected_directions = {
            "resolution_exposed": "improve",
            "mixed_secondary": "weak_improve",
            "metric_neutral": "neutral",
            "resolution_irrelevant": "neutral",
        }
        hypotheses = {item["episode_id"]: item for item in self.hypotheses}
        self.assertEqual({arm["episode_id"] for arm in self.arms}, set(hypotheses))
        for arm in self.arms:
            item = hypotheses[arm["episode_id"]]
            with self.subTest(episode_id=arm["episode_id"], arm=arm["arm"]):
                self.assertEqual(arm["admission_class"], item["admission_class"])
                self.assertEqual(expected_directions[item["admission_class"]], item["expected_direction"])
                self.assertEqual(
                    item["expected_direction"] in {"improve", "weak_improve"},
                    item["expected_to_improve"],
                )
                self.assertEqual(item["expected_to_improve"], arm["expected_to_improve"])
                self.assertTrue(item["prediction_reason"].strip())

    def test_packet_adaptations_are_recorded_for_every_scenario(self):
        contract = self.matrix["packet_adaptation_contract"]
        self.assertEqual("v3_with_corrected_2601_v1", contract["selected_corpus_version"])
        self.assertEqual(
            ["chart/templates/datadog-agent.yaml", "chart/values.yaml"],
            contract["shared_agent_paths"],
        )

        adaptations = {
            item["episode_id"]: item["packet_adaptation"]
            for item in self.hypotheses
        }
        self.assertEqual(15, len(adaptations))
        self.assertEqual(
            {"standard_env_double": 13, "empty_env_single": 2},
            dict(Counter(item["agent_values_adapter"] for item in adaptations.values())),
        )
        self.assertEqual("remove_two_invalid_simple_alert_new_group_delay_fields", adaptations[2483]["special_change"])
        self.assertEqual("corrected_2601_v1", adaptations[2601]["selected_packet"])
        self.assertIn("12 generated service app.py files", adaptations[2601]["special_change"])

    def test_current_ddeval_state_is_explicit_and_outcome_fields_are_empty(self):
        cohort_counts = Counter(arm["cohort"]["role"] for arm in self.arms)
        self.assertEqual({"primary": 24, "supplementary": 1, "excluded": 5}, dict(cohort_counts))

        status_counts = Counter(arm["observed"]["ddeval"]["status"] for arm in self.arms)
        self.assertEqual(
            {
                "pending_blocked_edp_projection_size": 25,
                "not_planned_event_identity_ineligible": 3,
                "not_planned_telemetry_empty": 2,
            },
            dict(status_counts),
        )
        for arm in self.arms:
            ddeval = arm["observed"]["ddeval"]
            with self.subTest(run_id=arm["run_id"]):
                self.assertEqual(0, ddeval["run_count"])
                self.assertIsNone(ddeval["workflow_run_id"])
                self.assertIsNone(ddeval["bits_trace_id"])
                self.assertIsNone(ddeval["judge"]["deepjudge_score"])
                self.assertIsNone(ddeval["metric_retrieval"]["metric_call_count"])
                self.assertIsNone(ddeval["reliability_efficiency"]["duration_seconds"])

    def test_scoring_criteria_cover_smoke_and_quality_contracts(self):
        self.assertEqual(
            {
                "admission_gates",
                "rca_quality",
                "metric_retrieval",
                "evidence_use",
                "safety",
                "reliability_efficiency",
            },
            set(self.matrix["scoring_criteria"]),
        )
        self.assertEqual(
            set(self.matrix["scoring_criteria"]),
            set(self.matrix["scoring_criteria_provenance"]),
        )
        for group, provenance in self.matrix["scoring_criteria_provenance"].items():
            with self.subTest(group=group):
                self.assertIn(provenance["collection_mode"], {"copied", "copied_and_derived", "derived_review"})
                self.assertTrue(provenance["source_contracts"])

        ids = {
            criterion["id"]
            for group in self.matrix["scoring_criteria"].values()
            for criterion in group
        }
        self.assertTrue(
            {
                "immediate_cause_found",
                "deepjudge_score",
                "metric_tool_called",
                "interval_1000_requested",
                "raw_data_requested",
                "complete_1hz_points_returned",
                "one_hz_evidence_used",
                "false_resolution_claim",
                "noise_introduced_contradiction",
                "duration_seconds",
                "tool_error_count",
            }.issubset(ids)
        )

    def test_markdown_is_concise_and_indexes_every_arm(self):
        text = MARKDOWN.read_text()
        self.assertIn("## Scoring key", text)
        self.assertIn("## Scenario expectations and packet adaptations", text)
        self.assertIn("`corrected_2601_v1`", text)
        self.assertIn("12 generated service app.py files", text)
        self.assertIn("## Cohorts", text)
        self.assertIn("## 30-arm results", text)
        self.assertIn("## Update rule and current blocker", text)
        self.assertLess(len(text.splitlines()), 120)
        self.assertLess(len(text.split()), 2_300)
        self.assertEqual(
            {"primary", "supplementary", "excluded"},
            set(self.matrix["cohort_definitions"]),
        )
        for arm in self.arms:
            with self.subTest(run_id=arm["run_id"]):
                self.assertEqual(1, text.count(f"`{arm['run_id']}`"))

        corpus_text = CORPUS_MATRIX.read_text()
        self.assertIn("## 15-scenario corpus matrix", corpus_text)
        self.assertIn("## Gate key", corpus_text)
        self.assertIn("## Packet adaptation key", corpus_text)
        self.assertIn("`v3` means", corpus_text)
        self.assertIn("`corrected_2601_v1`", corpus_text)
        table_lines = sum(line.startswith("|") for line in corpus_text.splitlines())
        prose_lines = sum(bool(line) and not line.startswith(("#", "|", ">")) for line in corpus_text.splitlines())
        self.assertGreater(table_lines, prose_lines)


if __name__ == "__main__":
    unittest.main()
