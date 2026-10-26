import copy
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
import freeze_corpus


HEX_A = "a" * 64
HEX_B = "b" * 64


def scenario(episode_id, stratum, pilot_role=None):
    value = {
        "episode_id": episode_id,
        "stratum": stratum,
        "source_pin": {
            "app_bundle_version": 1,
            "tar_sha256": HEX_A,
            "canonical_episode_path": f"episodes/{episode_id}.yaml",
            "canonical_episode_sha256": HEX_B,
        },
        "ground_truth": {
            "summary": f"ground truth {episode_id}",
            "long_description_sha256": HEX_A,
        },
        "qualification": {
            "g0_source_pin": "passed",
            "g1a_static_source_eligibility": "passed" if stratum != "log_trace_led" else "not_applicable",
            "g1b_judge_qualification": "passed",
        },
    }
    if pilot_role:
        value["pilot_role"] = pilot_role
    return value


def valid_manifest():
    scenarios = []
    for episode_id in range(1, 8):
        role = None
        if episode_id == 1:
            role = "resolution_exposed_count"
        elif episode_id == 2:
            role = "resolution_exposed_gauge_or_recurring"
        scenarios.append(scenario(episode_id, "resolution_exposed", role))
    scenarios.extend(
        [
            scenario(8, "metric_neutral", "metric_neutral"),
            scenario(9, "metric_neutral"),
            scenario(10, "log_trace_led", "log_trace_led"),
            scenario(11, "log_trace_led"),
            scenario(12, "log_trace_led"),
        ]
    )
    return {
        "schema_version": 1,
        "experiment_version": "global_1hz_eval_v3",
        "status": "frozen_pilot_ready",
        "core_scenarios": scenarios,
        "reserves": [
            scenario(101, "resolution_exposed"),
            scenario(102, "metric_neutral"),
            scenario(103, "log_trace_led"),
        ],
        "replacement_policy": {
            "completed_semantic_results_are_replaceable": False,
            "allowed_failure_classes": ["pre_investigation_infrastructure_failure"],
        },
        "freeze": {
            "owner_approval_status": "approved",
            "judge_owner_approval_status": "approved",
            "content_sha256": "",
        },
    }


class FreezeCorpusTest(unittest.TestCase):
    def test_valid_ready_manifest_passes(self):
        manifest = valid_manifest()
        manifest["freeze"]["content_sha256"] = freeze_corpus.content_sha256(manifest)

        self.assertEqual([], freeze_corpus.validate(manifest, require_ready=True))

    def test_rejects_duplicate_episode_ids_across_core_and_reserves(self):
        manifest = valid_manifest()
        manifest["reserves"][0]["episode_id"] = 1

        errors = freeze_corpus.validate(manifest)

        self.assertTrue(any("duplicate episode_id" in error for error in errors))

    def test_rejects_wrong_core_composition(self):
        manifest = valid_manifest()
        manifest["core_scenarios"][0]["stratum"] = "metric_neutral"

        errors = freeze_corpus.validate(manifest)

        self.assertTrue(any("core composition" in error for error in errors))

    def test_rejects_missing_reserve_stratum(self):
        manifest = valid_manifest()
        manifest["reserves"] = manifest["reserves"][:2]

        errors = freeze_corpus.validate(manifest)

        self.assertTrue(any("reserve composition" in error for error in errors))

    def test_rejects_invalid_source_hash(self):
        manifest = valid_manifest()
        manifest["core_scenarios"][0]["source_pin"]["tar_sha256"] = "not-a-hash"

        errors = freeze_corpus.validate(manifest)

        self.assertTrue(any("tar_sha256" in error for error in errors))

    def test_require_ready_rejects_pending_judge_or_owner_approval(self):
        manifest = valid_manifest()
        manifest["core_scenarios"][0]["qualification"]["g1b_judge_qualification"] = "pending"
        manifest["freeze"]["owner_approval_status"] = "pending"

        errors = freeze_corpus.validate(manifest, require_ready=True)

        self.assertTrue(any("g1b_judge_qualification" in error for error in errors))
        self.assertTrue(any("owner_approval_status" in error for error in errors))

    def test_content_hash_ignores_only_its_own_field(self):
        manifest = valid_manifest()
        first = freeze_corpus.content_sha256(manifest)
        manifest["freeze"]["content_sha256"] = first
        self.assertEqual(first, freeze_corpus.content_sha256(manifest))

        changed = copy.deepcopy(manifest)
        changed["core_scenarios"][0]["ground_truth"]["summary"] = "changed"
        self.assertNotEqual(first, freeze_corpus.content_sha256(changed))


def outcome_blind_scenario(episode_id, admission_class):
    return {
        "episode_id": episode_id,
        "scenario_name": f"scenario-{episode_id}",
        "app_name": f"app-{episode_id}",
        "admission_class": admission_class,
        "capability_status": "not_run",
        "ground_truth_summary": f"ground truth {episode_id}",
        "classification_evidence": "source-only telemetry classification",
        "disruption_duration_seconds": 600,
        "monitors": [],
        "source_pin": {
            "episode_id": episode_id,
            "app_bundle_version": 1,
            "archive_key": f"episode/{episode_id}/archive.tar",
            "archive_sha256": HEX_A,
            "archive_bytes": 100,
            "manifest_sha256": HEX_B,
            "canonical_episode_path": f"episodes/{episode_id}.yaml",
            "canonical_episode_sha256": HEX_A,
        },
    }


def valid_outcome_blind_manifest():
    panel = []
    panel.extend(outcome_blind_scenario(i, "resolution_exposed") for i in range(1, 6))
    panel.extend(outcome_blind_scenario(i, "metric_neutral") for i in range(6, 10))
    panel.extend(outcome_blind_scenario(i, "mixed_secondary") for i in range(10, 13))
    panel.extend(outcome_blind_scenario(i, "resolution_irrelevant") for i in range(13, 16))
    return {
        "schema_version": 2,
        "experiment_version": "global_1hz_outcome_blind_v1",
        "status": "source_selected_pending_capability_smokes",
        "documentation": {"role": "active_outcome_blind_readiness_input"},
        "inventory": {"source_screened_count": 96, "admitted_count": 88},
        "admission_policy": {
            "required": ["source quality"],
            "explicit_non_gates": [
                "observed positive 1 Hz effect",
                "pre-existing one-second versus fifteen-second counterfactual",
                "pre-existing RCA score improvement",
            ],
        },
        "composition": {
            "resolution_exposed": 5,
            "metric_neutral": 4,
            "mixed_secondary": 3,
            "resolution_irrelevant": 3,
        },
        "score_panel": panel,
        "reserves": [],
        "capability_reference": {"episode_id": 1, "scope": "plumbing_not_effect"},
        "judge_calibration": {
            "status": "pending_current_panel_cases",
            "cases_complete": False,
            "evidence_sha256": None,
        },
        "replacement_policy": {
            "completed_semantic_results_are_replaceable": False,
            "arm_blind_decision_required": True,
            "allowed_failure_classes": ["pre_disruption_infrastructure_failure"],
            "null_negative_or_regressive_results_are_retained": True,
        },
        "score_policy": {
            "membership_frozen_before_treatment_outcomes": True,
            "same_event_15_second_counterfactual": "post_capture_analysis_not_admission",
        },
        "freeze": {"owner_approval_status": "pending", "content_sha256": ""},
    }


class OutcomeBlindFreezeCorpusTest(unittest.TestCase):
    def test_valid_outcome_blind_manifest_passes(self):
        manifest = valid_outcome_blind_manifest()
        manifest["freeze"]["content_sha256"] = freeze_corpus.content_sha256(manifest)

        self.assertEqual([], freeze_corpus.validate(manifest))

    def test_rejects_empty_content_hash(self):
        manifest = valid_outcome_blind_manifest()

        errors = freeze_corpus.validate(manifest)

        self.assertTrue(any("non-empty lowercase SHA-256" in error for error in errors))

    def test_require_ready_rejects_incomplete_judge_calibration(self):
        manifest = valid_outcome_blind_manifest()
        manifest["status"] = "frozen_execution_ready"
        manifest["freeze"]["owner_approval_status"] = "approved"
        for item in manifest["score_panel"]:
            item["capability_status"] = "passed_nonconfirmatory_capability_smoke"
        manifest["freeze"]["content_sha256"] = freeze_corpus.content_sha256(manifest)

        errors = freeze_corpus.validate(manifest, require_ready=True)

        self.assertTrue(any("judge calibration" in error for error in errors))

    def test_ready_requires_and_accepts_exact_judge_evidence(self):
        manifest = valid_outcome_blind_manifest()
        manifest["status"] = "frozen_execution_ready"
        manifest["freeze"]["owner_approval_status"] = "approved"
        for item in manifest["score_panel"]:
            item["capability_status"] = "passed_nonconfirmatory_capability_smoke"
        manifest["judge_calibration"] = {
            "status": "passed",
            "cases_complete": True,
            "evidence_sha256": HEX_B,
        }
        manifest["freeze"]["content_sha256"] = freeze_corpus.content_sha256(manifest)

        self.assertEqual([], freeze_corpus.validate(manifest, require_ready=True))

    def test_rejects_missing_weakness_strata(self):
        manifest = valid_outcome_blind_manifest()
        manifest["score_panel"] = [
            item for item in manifest["score_panel"] if item["admission_class"] != "resolution_irrelevant"
        ]
        manifest["composition"].pop("resolution_irrelevant")

        errors = freeze_corpus.validate(manifest)

        self.assertTrue(any("outcome-blind panel composition" in error for error in errors))

    def test_rejects_outcome_based_admission_policy(self):
        manifest = valid_outcome_blind_manifest()
        manifest["admission_policy"]["explicit_non_gates"].remove("observed positive 1 Hz effect")

        errors = freeze_corpus.validate(manifest)

        self.assertTrue(any("observed positive 1 Hz effect" in error for error in errors))

    def test_rejects_replaceable_null_or_negative_results(self):
        manifest = valid_outcome_blind_manifest()
        manifest["replacement_policy"]["null_negative_or_regressive_results_are_retained"] = False

        errors = freeze_corpus.validate(manifest)

        self.assertTrue(any("null, negative, and regressive" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
