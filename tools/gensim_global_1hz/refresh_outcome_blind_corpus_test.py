import copy
import hashlib
import json
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
import refresh_outcome_blind_corpus as refresh


def candidate(episode_id, admission_class, status="admitted"):
    return {
        "episode_id": episode_id,
        "admission_class": admission_class,
        "admission_status": status,
        "scenario_name": f"scenario-{episode_id}",
    }


class RefreshOutcomeBlindCorpusTest(unittest.TestCase):
    def test_classifies_source_strata_without_using_observed_outcomes(self):
        cases = [
            (2483, "High-confidence resolution-exposed candidates", "resolution_exposed", "admitted"),
            (2502, "High-confidence metric-neutral controls", "metric_neutral", "admitted"),
            (2574, "Source-eligible mixed secondary candidates", "mixed_secondary", "admitted"),
            (2684, "Log/trace-led future candidates", "resolution_irrelevant", "admitted"),
            (2586, "Excluded or repair-required", "resolution_irrelevant", "admitted"),
            (2657, "Excluded or repair-required", "resolution_irrelevant", "admitted"),
            (2578, "Excluded or repair-required", "source_defect", "blocked"),
        ]

        for episode_id, section, expected_class, expected_status in cases:
            with self.subTest(episode_id=episode_id):
                result = refresh.classify_source(episode_id, section)
                self.assertEqual(expected_class, result["admission_class"])
                self.assertEqual(expected_status, result["admission_status"])

    def test_admission_does_not_require_rca_chain_counterfactual_or_benefit(self):
        source = refresh.classify_source(2483, "High-confidence resolution-exposed candidates")
        record = {
            **source,
            "causalgen_gates": "passed",
            "do_not_use": False,
            "canonical_ground_truth_available": True,
            "runnable_episode_yaml": True,
            "rca_causal_chain_present": False,
            "counterfactual_available": False,
            "observed_1hz_benefit": None,
        }

        self.assertEqual("admitted", refresh.admission_status(record))

    def test_classification_identity_guard_rejects_changed_bundle(self):
        baseline = {
            "app_bundle_version": 3,
            "archive_key": "episode/2483/v3/archive.tar",
            "manifest_sha256": "a" * 64,
            "canonical_episode_sha256": "b" * 64,
        }
        current = dict(baseline)
        self.assertTrue(refresh.classification_identity_matches(current, baseline))

        for field, changed in [
            ("app_bundle_version", 4),
            ("archive_key", "episode/2483/v4/archive.tar"),
            ("manifest_sha256", "c" * 64),
            ("canonical_episode_sha256", "d" * 64),
        ]:
            with self.subTest(field=field):
                mutated = {**current, field: changed}
                self.assertFalse(refresh.classification_identity_matches(mutated, baseline))

    def test_refresh_fails_closed_on_changed_classification_identity(self):
        candidates = [
            {"episode_id": 2483, "blocking_reasons": []},
            {
                "episode_id": 2500,
                "blocking_reasons": ["classification_identity_changed_requires_static_review"],
            },
        ]

        with self.assertRaisesRegex(ValueError, "2500"):
            refresh.ensure_no_classification_identity_changes(candidates)

    def test_admission_rejects_quality_or_source_defects(self):
        base = {
            "admission_status": "admitted",
            "causalgen_gates": "passed",
            "do_not_use": False,
            "canonical_ground_truth_available": True,
            "runnable_episode_yaml": True,
        }
        mutations = [
            {"causalgen_gates": "soft-pass"},
            {"do_not_use": True},
            {"canonical_ground_truth_available": False},
            {"runnable_episode_yaml": False},
            {"admission_status": "blocked"},
        ]

        for mutation in mutations:
            with self.subTest(mutation=mutation):
                record = {**base, **mutation}
                self.assertEqual("blocked", refresh.admission_status(record))

    def test_panel_selection_is_deterministic_and_stratified(self):
        candidates = []
        candidates.extend(candidate(i, "resolution_exposed") for i in range(1, 8))
        candidates.extend(candidate(i, "metric_neutral") for i in range(8, 11))
        candidates.extend(candidate(i, "mixed_secondary") for i in range(11, 13))
        candidates.extend(candidate(i, "resolution_irrelevant") for i in range(13, 30))
        candidates.append(candidate(100, "source_defect", "blocked"))

        first = refresh.select_panel(candidates, "snapshot-sha", irrelevant_count=3)
        second = refresh.select_panel(list(reversed(candidates)), "snapshot-sha", irrelevant_count=3)

        self.assertEqual(first, second)
        self.assertEqual(15, len(first))
        counts = refresh.class_counts(first)
        self.assertEqual(
            {
                "resolution_exposed": 7,
                "metric_neutral": 3,
                "mixed_secondary": 2,
                "resolution_irrelevant": 3,
            },
            counts,
        )
        self.assertNotIn(100, [item["episode_id"] for item in first])

    def test_panel_selection_ignores_capability_and_expected_effect_fields(self):
        candidates = [candidate(i, "resolution_irrelevant") for i in range(1, 20)]
        baseline = refresh.select_panel(candidates, "snapshot-sha", irrelevant_count=3)
        enriched = copy.deepcopy(candidates)
        for index, item in enumerate(enriched):
            item["capability_status"] = "passed" if index % 2 else "not_run"
            item["expected_1hz_effect"] = "positive" if index % 3 else "negative"
            item["observed_score"] = index / 10

        self.assertEqual(
            [item["episode_id"] for item in baseline],
            [item["episode_id"] for item in refresh.select_panel(enriched, "snapshot-sha", irrelevant_count=3)],
        )

    def test_legacy_minute_duration_is_metadata_not_subsecond_duration(self):
        self.assertEqual(600.0, refresh.duration_seconds("phases.disruption.duration_minutes", 10))
        self.assertEqual(12.0, refresh.duration_seconds("disruption_duration_seconds", 12))
        self.assertIsNone(refresh.duration_seconds("threshold_seconds", 12))

    def test_refresh_requires_baseline_or_explicit_bootstrap(self):
        with self.assertRaisesRegex(ValueError, "classification baseline"):
            refresh.load_classification_baseline(None, bootstrap=False)
        self.assertEqual({}, refresh.load_classification_baseline(None, bootstrap=True))

    def test_freeze_hash_uses_validator_canonical_newline(self):
        manifest = {"schema_version": 2, "freeze": {"content_sha256": "ignored"}, "value": 1}
        canonical = copy.deepcopy(manifest)
        canonical["freeze"].pop("content_sha256")
        expected = hashlib.sha256(
            (json.dumps(canonical, sort_keys=True, separators=(",", ":"), ensure_ascii=False) + "\n").encode()
        ).hexdigest()

        self.assertEqual(expected, refresh.freeze_hash(manifest))


if __name__ == "__main__":
    unittest.main()
