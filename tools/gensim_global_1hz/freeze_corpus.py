#!/usr/bin/env python3
"""Validate the immutable GenSim global 1 Hz corpus freeze manifest."""

from __future__ import annotations

import argparse
import copy
import hashlib
import json
import re
from collections import Counter
from pathlib import Path
from typing import Any

CORE_COMPOSITION = {
    "resolution_exposed": 7,
    "metric_neutral": 2,
    "log_trace_led": 3,
}
OUTCOME_BLIND_COMPOSITION = {
    "resolution_exposed": 5,
    "metric_neutral": 4,
    "mixed_secondary": 3,
    "resolution_irrelevant": 3,
}
OUTCOME_BLIND_NON_GATES = {
    "observed positive 1 Hz effect",
    "pre-existing one-second versus fifteen-second counterfactual",
    "pre-existing RCA score improvement",
}
PILOT_ROLES = {
    "resolution_exposed_count",
    "resolution_exposed_gauge_or_recurring",
    "metric_neutral",
    "log_trace_led",
}
HEX_SHA256 = re.compile(r"^[a-f0-9]{64}$")


def canonical_payload(manifest: dict[str, Any]) -> bytes:
    payload = copy.deepcopy(manifest)
    payload.setdefault("freeze", {}).pop("content_sha256", None)
    return (json.dumps(payload, sort_keys=True, separators=(",", ":"), ensure_ascii=False) + "\n").encode()


def content_sha256(manifest: dict[str, Any]) -> str:
    return hashlib.sha256(canonical_payload(manifest)).hexdigest()


def _validate_scenario(item: Any, location: str, require_ready: bool) -> list[str]:
    errors: list[str] = []
    if not isinstance(item, dict):
        return [f"{location} must be an object"]
    episode_id = item.get("episode_id")
    if not isinstance(episode_id, int) or episode_id <= 0:
        errors.append(f"{location}.episode_id must be a positive integer")
    stratum = item.get("stratum")
    if stratum not in CORE_COMPOSITION:
        errors.append(f"{location}.stratum must be one of {sorted(CORE_COMPOSITION)}")

    source = item.get("source_pin")
    if not isinstance(source, dict):
        errors.append(f"{location}.source_pin must be an object")
    else:
        version = source.get("app_bundle_version")
        if not isinstance(version, int) or version <= 0:
            errors.append(f"{location}.source_pin.app_bundle_version must be a positive integer")
        for field in ("tar_sha256", "canonical_episode_sha256"):
            if not isinstance(source.get(field), str) or not HEX_SHA256.fullmatch(source[field]):
                errors.append(f"{location}.source_pin.{field} must be a lowercase SHA-256")
        path = source.get("canonical_episode_path")
        if not isinstance(path, str) or not path.startswith("episodes/") or not path.endswith((".yaml", ".yml")):
            errors.append(f"{location}.source_pin.canonical_episode_path must name one episode YAML")

    truth = item.get("ground_truth")
    if not isinstance(truth, dict):
        errors.append(f"{location}.ground_truth must be an object")
    else:
        if not isinstance(truth.get("summary"), str) or not truth["summary"].strip():
            errors.append(f"{location}.ground_truth.summary must be non-empty")
        digest = truth.get("long_description_sha256")
        if not isinstance(digest, str) or not HEX_SHA256.fullmatch(digest):
            errors.append(f"{location}.ground_truth.long_description_sha256 must be a lowercase SHA-256")

    qualification = item.get("qualification")
    if not isinstance(qualification, dict):
        errors.append(f"{location}.qualification must be an object")
    else:
        if qualification.get("g0_source_pin") != "passed":
            errors.append(f"{location}.qualification.g0_source_pin must be passed")
        expected_g1a = "not_applicable" if stratum == "log_trace_led" else "passed"
        if qualification.get("g1a_static_source_eligibility") != expected_g1a:
            errors.append(
                f"{location}.qualification.g1a_static_source_eligibility must be {expected_g1a}"
            )
        if require_ready and qualification.get("g1b_judge_qualification") != "passed":
            errors.append(f"{location}.qualification.g1b_judge_qualification must be passed")
    return errors


def _validate_outcome_blind_scenario(item: Any, location: str, require_ready: bool) -> list[str]:
    if not isinstance(item, dict):
        return [f"{location} must be an object"]
    errors: list[str] = []
    episode_id = item.get("episode_id")
    if not isinstance(episode_id, int) or episode_id <= 0:
        errors.append(f"{location}.episode_id must be a positive integer")
    if item.get("admission_class") not in OUTCOME_BLIND_COMPOSITION:
        errors.append(
            f"{location}.admission_class must be one of {sorted(OUTCOME_BLIND_COMPOSITION)}"
        )
    for field in ("scenario_name", "app_name", "ground_truth_summary"):
        if not isinstance(item.get(field), str) or not item[field].strip():
            errors.append(f"{location}.{field} must be non-empty")
    source = item.get("source_pin")
    if not isinstance(source, dict):
        errors.append(f"{location}.source_pin must be an object")
    else:
        if source.get("episode_id") != episode_id:
            errors.append(f"{location}.source_pin.episode_id must match episode_id")
        version = source.get("app_bundle_version")
        if not isinstance(version, int) or version <= 0:
            errors.append(f"{location}.source_pin.app_bundle_version must be a positive integer")
        for field in ("archive_sha256", "manifest_sha256", "canonical_episode_sha256"):
            if not isinstance(source.get(field), str) or not HEX_SHA256.fullmatch(source[field]):
                errors.append(f"{location}.source_pin.{field} must be a lowercase SHA-256")
        path = source.get("canonical_episode_path")
        if not isinstance(path, str) or not path.startswith("episodes/") or not path.endswith((".yaml", ".yml")):
            errors.append(f"{location}.source_pin.canonical_episode_path must name one episode YAML")
    if "observed_outcome" in item:
        errors.append(f"{location} must not contain an observed outcome before execution")
    if require_ready and item.get("capability_status") not in {
        "passed_nonconfirmatory_mandatory_1hz_capability",
        "passed_nonconfirmatory_capability_smoke",
    }:
        errors.append(f"{location}.capability_status must be passed before execution readiness")
    return errors


def _validate_outcome_blind(manifest: dict[str, Any], require_ready: bool) -> list[str]:
    errors: list[str] = []
    if manifest.get("experiment_version") != "global_1hz_outcome_blind_v1":
        errors.append("experiment_version must be global_1hz_outcome_blind_v1")
    panel = manifest.get("score_panel")
    if not isinstance(panel, list):
        errors.append("score_panel must be a list")
        panel = []
    for index, item in enumerate(panel):
        errors.extend(_validate_outcome_blind_scenario(item, f"score_panel[{index}]", require_ready))
    ids = [item.get("episode_id") for item in panel if isinstance(item, dict)]
    duplicates = sorted(episode_id for episode_id, count in Counter(ids).items() if count > 1)
    if duplicates:
        errors.append(f"duplicate episode_id values in score_panel: {duplicates}")
    composition = Counter(item.get("admission_class") for item in panel if isinstance(item, dict))
    if dict(composition) != OUTCOME_BLIND_COMPOSITION:
        errors.append(
            f"outcome-blind panel composition must be {OUTCOME_BLIND_COMPOSITION}, got {dict(composition)}"
        )
    if manifest.get("composition") != OUTCOME_BLIND_COMPOSITION:
        errors.append("recorded composition must match the outcome-blind panel composition")
    admission = manifest.get("admission_policy")
    if not isinstance(admission, dict):
        errors.append("admission_policy must be an object")
    else:
        non_gates = admission.get("explicit_non_gates")
        if not isinstance(non_gates, list):
            errors.append("admission_policy.explicit_non_gates must be a list")
        else:
            missing = sorted(OUTCOME_BLIND_NON_GATES - set(non_gates))
            for item in missing:
                errors.append(f"admission_policy must explicitly exclude {item} from admission")
    replacement = manifest.get("replacement_policy")
    if not isinstance(replacement, dict):
        errors.append("replacement_policy must be an object")
    else:
        if replacement.get("completed_semantic_results_are_replaceable") is not False:
            errors.append("completed semantic results must not be replaceable")
        if replacement.get("arm_blind_decision_required") is not True:
            errors.append("replacement decisions must be arm-blind")
        if replacement.get("null_negative_or_regressive_results_are_retained") is not True:
            errors.append("null, negative, and regressive results must be retained")
    score_policy = manifest.get("score_policy")
    if not isinstance(score_policy, dict) or score_policy.get("membership_frozen_before_treatment_outcomes") is not True:
        errors.append("score_policy must freeze membership before treatment outcomes")
    judge = manifest.get("judge_calibration")
    if not isinstance(judge, dict):
        errors.append("judge_calibration must be an object")
    freeze = manifest.get("freeze")
    if not isinstance(freeze, dict):
        errors.append("freeze must be an object")
    else:
        recorded = freeze.get("content_sha256")
        if not isinstance(recorded, str) or not HEX_SHA256.fullmatch(recorded):
            errors.append("freeze.content_sha256 must be a non-empty lowercase SHA-256")
        elif recorded != content_sha256(manifest):
            errors.append("freeze.content_sha256 does not match canonical manifest content")
        if require_ready:
            if manifest.get("status") != "frozen_execution_ready":
                errors.append("status must be frozen_execution_ready")
            if freeze.get("owner_approval_status") != "approved":
                errors.append("freeze.owner_approval_status must be approved")
            if not isinstance(judge, dict) or judge.get("status") != "passed" or judge.get("cases_complete") is not True:
                errors.append("judge calibration must be complete and passed")
            elif not isinstance(judge.get("evidence_sha256"), str) or not HEX_SHA256.fullmatch(judge["evidence_sha256"]):
                errors.append("judge calibration evidence must have a lowercase SHA-256")
    return errors


def validate(manifest: Any, require_ready: bool = False) -> list[str]:
    if not isinstance(manifest, dict):
        return ["manifest must be a JSON object"]
    if manifest.get("schema_version") == 2:
        return _validate_outcome_blind(manifest, require_ready)
    errors: list[str] = []
    if manifest.get("schema_version") != 1:
        errors.append("schema_version must be 1")
    if manifest.get("experiment_version") != "global_1hz_eval_v3":
        errors.append("experiment_version must be global_1hz_eval_v3")

    core = manifest.get("core_scenarios")
    reserves = manifest.get("reserves")
    if not isinstance(core, list):
        errors.append("core_scenarios must be a list")
        core = []
    if not isinstance(reserves, list):
        errors.append("reserves must be a list")
        reserves = []

    for index, item in enumerate(core):
        errors.extend(_validate_scenario(item, f"core_scenarios[{index}]", require_ready))
    for index, item in enumerate(reserves):
        errors.extend(_validate_scenario(item, f"reserves[{index}]", require_ready))

    ids = [item.get("episode_id") for item in core + reserves if isinstance(item, dict)]
    duplicates = sorted(episode_id for episode_id, count in Counter(ids).items() if count > 1)
    if duplicates:
        errors.append(f"duplicate episode_id values across core and reserves: {duplicates}")

    composition = Counter(item.get("stratum") for item in core if isinstance(item, dict))
    if dict(composition) != CORE_COMPOSITION:
        errors.append(f"core composition must be {CORE_COMPOSITION}, got {dict(composition)}")
    reserve_composition = Counter(item.get("stratum") for item in reserves if isinstance(item, dict))
    missing_reserves = sorted(stratum for stratum in CORE_COMPOSITION if reserve_composition[stratum] < 1)
    if missing_reserves:
        errors.append(f"reserve composition must include every core stratum; missing {missing_reserves}")

    pilot_roles = [item.get("pilot_role") for item in core if isinstance(item, dict) and item.get("pilot_role")]
    role_counts = Counter(pilot_roles)
    if set(role_counts) != PILOT_ROLES or any(count != 1 for count in role_counts.values()):
        errors.append(f"pilot roles must contain exactly one of each {sorted(PILOT_ROLES)}, got {role_counts}")

    policy = manifest.get("replacement_policy")
    if not isinstance(policy, dict):
        errors.append("replacement_policy must be an object")
    else:
        if policy.get("completed_semantic_results_are_replaceable") is not False:
            errors.append("completed semantic results must not be replaceable")
        allowed = policy.get("allowed_failure_classes")
        if not isinstance(allowed, list) or not allowed:
            errors.append("replacement_policy.allowed_failure_classes must be a non-empty list")

    freeze = manifest.get("freeze")
    if not isinstance(freeze, dict):
        errors.append("freeze must be an object")
    else:
        recorded = freeze.get("content_sha256")
        if recorded and recorded != content_sha256(manifest):
            errors.append("freeze.content_sha256 does not match canonical manifest content")
        if require_ready:
            if manifest.get("status") != "frozen_pilot_ready":
                errors.append("status must be frozen_pilot_ready")
            for field in ("owner_approval_status", "judge_owner_approval_status"):
                if freeze.get(field) != "approved":
                    errors.append(f"freeze.{field} must be approved")
    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("manifest", type=Path)
    parser.add_argument("--require-ready", action="store_true")
    parser.add_argument("--print-content-sha256", action="store_true")
    args = parser.parse_args()
    manifest = json.loads(args.manifest.read_text())
    errors = validate(manifest, require_ready=args.require_ready)
    if args.print_content_sha256:
        print(content_sha256(manifest))
    if errors:
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print(f"PASS: {args.manifest} is valid" + (" and pilot-ready" if args.require_ready else ""))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
