import json
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
DRAFTS = ROOT / "docs/dev/gensim-global-1hz-judge-calibration-drafts.json"
ACTIVE = ROOT / "docs/dev/gensim-global-1hz-judge-calibration-cases.json"
BASELINE = ROOT / "docs/dev/gensim-global-1hz-classification-baseline.json"


class JudgeCalibrationDraftsTest(unittest.TestCase):
    def setUp(self):
        self.drafts = json.loads(DRAFTS.read_text())
        self.active = json.loads(ACTIVE.read_text())
        baseline = json.loads(BASELINE.read_text())
        self.identities = {
            item["episode_id"]: item["source_identity"] for item in baseline["records"]
        }

    def test_drafts_cover_exact_missing_current_panel_cases(self):
        self.assertEqual(
            {2497, 2500, 2574, 2601},
            {item["episode_id"] for item in self.drafts["cases"]},
        )
        self.assertFalse(self.drafts["documentation"]["treatment_outcomes_used"])

    def test_active_cases_are_bound_to_current_source_identity(self):
        for item in self.active["cases"]:
            with self.subTest(episode_id=item["episode_id"]):
                self.assertEqual(
                    self.identities[item["episode_id"]],
                    self.active["source_identity_bindings"][str(item["episode_id"])],
                )

    def test_every_draft_is_bound_to_current_source_identity(self):
        for item in self.drafts["cases"]:
            with self.subTest(episode_id=item["episode_id"]):
                self.assertEqual(
                    self.identities[item["episode_id"]], item["source_identity"]
                )

    def test_variants_are_complete_distinct_and_pending_approval(self):
        expected = {
            "canonical_correct",
            "symptom_only",
            "wrong_mechanism",
            "distractor",
        }
        for item in self.drafts["cases"]:
            with self.subTest(episode_id=item["episode_id"]):
                self.assertGreater(len(item["ground_truth"]), 200)
                self.assertEqual(expected, set(item["variants"]))
                self.assertEqual(4, len(set(item["variants"].values())))
                self.assertTrue(all(value.strip() for value in item["variants"].values()))
                self.assertEqual(
                    {"scenario_curator": "pending", "judge_owner": "pending"},
                    item["approval"],
                )


if __name__ == "__main__":
    unittest.main()
