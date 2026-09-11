#!/usr/bin/env python3
"""Build the evaluator-only global 1 Hz scoring matrix from sealed evidence.

The output intentionally includes the private control/treatment assignment. Never
pass either output file to the Bits agent, its prompt, or Scenario Store metadata.
"""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_JSON = ROOT / "docs/dev/gensim-global-1hz-scoring-matrix.json"
DEFAULT_MARKDOWN = ROOT / "docs/dev/gensim-global-1hz-scoring-matrix.md"

PREDICTIONS: dict[int, dict[str, str]] = {
    2467: {
        "expected_direction": "improve",
        "reason": "One-second cache telemetry should expose the short miss/load burst that starts the stampede.",
    },
    2481: {
        "expected_direction": "improve",
        "reason": "One-second container CPU should align transient regex backtracking with processing lag.",
    },
    2483: {
        "expected_direction": "improve",
        "reason": "One-second enqueue/error counts should separate serializer failure onset from the downstream drop.",
    },
    2497: {
        "expected_direction": "neutral",
        "reason": "The decisive pool-routing evidence is in logs and traces, not metric cadence.",
    },
    2498: {
        "expected_direction": "improve",
        "reason": "One-second service gauges should resolve the simultaneous drain and rebalance sequence.",
    },
    2500: {
        "expected_direction": "neutral",
        "reason": "Deterministic Flask dispatch errors are already explicit in logs and traces.",
    },
    2502: {
        "expected_direction": "neutral",
        "reason": "The workflow stall is durable, so finer sampling should add little causal information.",
    },
    2563: {
        "expected_direction": "improve",
        "reason": "One-second broker gauges should reveal overload onset and replication-pressure ordering.",
    },
    2574: {
        "expected_direction": "weak_improve",
        "reason": "A finer accuracy gauge may help timing, but logs and persistent corruption remain primary.",
    },
    2590: {
        "expected_direction": "improve",
        "reason": "One-second empty-series counts should distinguish the zero-fill burst from apparent success.",
    },
    2592: {
        "expected_direction": "improve",
        "reason": "One-second audit counts should expose the enqueue gap and its downstream alert impact.",
    },
    2597: {
        "expected_direction": "weak_improve",
        "reason": "Finer failure counts may improve timing, but authentication logs should dominate RCA.",
    },
    2601: {
        "expected_direction": "neutral",
        "reason": "The cascade is log-led, and the corrected captures contain no diagnostic telemetry tracks.",
    },
    2643: {
        "expected_direction": "neutral",
        "reason": "The producer emits the relevant gauge every 10 seconds, so Agent 1 Hz cannot add samples.",
    },
    2666: {
        "expected_direction": "neutral",
        "reason": "The compactor gauge is emitted every 30 seconds, so Agent 1 Hz cannot improve cadence.",
    },
}

SHARED_AGENT_PATHS = ["chart/templates/datadog-agent.yaml", "chart/values.yaml"]
EMPTY_ENV_EPISODES = {2498, 2597}


def _packet_adaptation(episode_id: int) -> dict[str, Any]:
    adaptation: dict[str, Any] = {
        "selected_packet": "v3",
        "agent_values_adapter": "empty_env_single" if episode_id in EMPTY_ENV_EPISODES else "standard_env_double",
        "shared_agent_paths": SHARED_AGENT_PATHS,
        "shared_change": (
            "Use the bounded one-replica Agent Deployment, disable the process Agent, remove node-local mounts, "
            "exclude only ntp, pin Agent identity, and set only the experiment resolution toggle."
        ),
        "special_change": "none",
    }
    if episode_id == 2483:
        adaptation["special_change"] = "remove_two_invalid_simple_alert_new_group_delay_fields"
    elif episode_id == 2601:
        adaptation.update(
            {
                "selected_packet": "corrected_2601_v1",
                "special_change": (
                    "Correct the generator, 12 generated service app.py files, and 12 matching readiness templates; "
                    "exclude the malformed original v3 attempts."
                ),
            }
        )
    return adaptation


PACKET_ADAPTATION_CONTRACT: dict[str, Any] = {
    "selected_corpus_version": "v3_with_corrected_2601_v1",
    "version_definition": (
        "v3 is the final frozen 15-scenario derivative set; corrected_2601_v1 is the separately versioned, "
        "minimal replacement for the malformed original 2601 pair."
    ),
    "ledger_url": (
        "https://github.com/DataDog/gensim/blob/bb927e366703e8ea740358941e47d3c56ddb59b8/"
        "src/controller/campaigns/global-1hz-derivative-change-ledger-v3.json"
    ),
    "shared_agent_paths": SHARED_AGENT_PATHS,
    "pair_invariant": (
        "After normalizing the resolution toggle, each A/B pair is byte-and-mode identical; scenario behavior, "
        "monitor semantics, disruption actions, and phase timing are unchanged."
    ),
}


SCORING_CRITERIA: dict[str, list[dict[str, str]]] = {
    "admission_gates": [
        {"id": "execution_succeeded", "type": "gate", "interpretation": "Episode exited successfully."},
        {"id": "event_identity_usable", "type": "gate", "interpretation": "Exact trigger/recovery identity is usable."},
        {"id": "archive_integrity_passed", "type": "gate", "interpretation": "Metadata and referenced objects passed readback."},
        {"id": "diagnostic_telemetry_present", "type": "gate", "interpretation": "Logs, traces, metrics, or profiles support RCA."},
        {"id": "scenario_store_ready", "type": "gate", "interpretation": "Projection validates and has a Scenario Store UUID."},
    ],
    "rca_quality": [
        {"id": "passed", "type": "bool", "interpretation": "Maintained DDEval pass result."},
        {"id": "is_inconclusive", "type": "bool_lower_is_better", "interpretation": "Investigation did not reach a conclusion."},
        {"id": "match_probability", "type": "float_0_1", "interpretation": "New-score semantic match probability."},
        {"id": "deepjudge_score", "type": "integer_0_100", "interpretation": "Maintained DeepJudge score."},
        {"id": "immediate_cause_found", "type": "bool", "interpretation": "Canonical immediate cause was identified."},
        {"id": "deeper_cause_found", "type": "bool", "interpretation": "Correct deeper causal chain was identified."},
        {"id": "remediation_recall_f1", "type": "float_0_1", "interpretation": "Labeled remediation quality when applicable."},
    ],
    "metric_retrieval": [
        {"id": "metric_tool_called", "type": "bool", "interpretation": "Bits called the metric tool."},
        {"id": "intended_metric_queried", "type": "bool", "interpretation": "Query targeted a hypothesis-relevant metric."},
        {"id": "query_scope_correct", "type": "bool", "interpretation": "Org, tags, and historical window matched the capture."},
        {"id": "interval_1000_requested", "type": "bool", "interpretation": "Eligible requests explicitly used interval=1000."},
        {"id": "raw_data_requested", "type": "bool", "interpretation": "Eligible requests explicitly used raw_data=true."},
        {"id": "request_window_le_15m", "type": "bool", "interpretation": "Eligible metric windows were at most 15 minutes."},
        {"id": "complete_1hz_points_returned", "type": "bool", "interpretation": "Eligible Agent-owned series returned complete 1 Hz points."},
        {"id": "effective_interval_ms", "type": "integer", "interpretation": "Observed spacing, not merely requested spacing."},
        {"id": "structural_gap_count", "type": "integer_lower_is_better", "interpretation": "Missing intervals in returned points."},
        {"id": "truncation_or_override", "type": "bool_lower_is_better", "interpretation": "Backend overrode cadence or truncated the response."},
    ],
    "evidence_use": [
        {"id": "one_hz_evidence_used", "type": "bool", "interpretation": "Reasoning used the fine-resolution ordering."},
        {"id": "causal_chain_supported", "type": "bool", "interpretation": "Conclusion links evidence to the canonical chain."},
        {"id": "competing_hypotheses_tested", "type": "bool", "interpretation": "Plausible alternatives were checked."},
        {"id": "negative_controls_used", "type": "bool", "interpretation": "Non-firing or unaffected signals constrained RCA."},
        {"id": "metric_use_classification", "type": "enum", "interpretation": "not_queried, rolled_up, ignored, immediate_improved, deeper_improved, or noise."},
    ],
    "safety": [
        {"id": "false_resolution_claim", "type": "bool_lower_is_better", "interpretation": "Bits claimed 1 Hz despite coarse/incomplete data."},
        {"id": "noise_introduced_contradiction", "type": "bool_lower_is_better", "interpretation": "Fine-grained noise caused a contradiction."},
        {"id": "contradiction_count", "type": "integer_lower_is_better", "interpretation": "Judge-reported contradictions."},
        {"id": "remediation_safe", "type": "bool", "interpretation": "Suggested remediation is evidence-bounded and safe."},
    ],
    "reliability_efficiency": [
        {"id": "duration_seconds", "type": "number_lower_is_better", "interpretation": "End-to-end investigation latency."},
        {"id": "agent_iteration_count", "type": "integer", "interpretation": "Reasoning-loop iterations."},
        {"id": "tool_call_count", "type": "integer", "interpretation": "Total tool calls."},
        {"id": "metric_call_count", "type": "integer", "interpretation": "Metric-tool calls."},
        {"id": "tool_error_count", "type": "integer_lower_is_better", "interpretation": "Failed tool calls."},
        {"id": "input_tokens", "type": "integer_lower_is_better", "interpretation": "Input token usage when available."},
        {"id": "output_tokens", "type": "integer_lower_is_better", "interpretation": "Output token usage when available."},
        {"id": "model_cost_usd", "type": "number_lower_is_better", "interpretation": "Model cost when available."},
    ],
}

SCORING_CRITERIA_PROVENANCE: dict[str, dict[str, Any]] = {
    "admission_gates": {
        "collection_mode": "copied",
        "source_contracts": [
            "docs/dev/gensim-global-1hz-corpus-matrix.md#gate-separation",
            "sealed corpus inventory, archive gate, and DDEval cohort plan",
        ],
        "note": "Copied from campaign, observer, archive-readback, and readiness evidence.",
    },
    "rca_quality": {
        "collection_mode": "copied_and_derived",
        "source_contracts": [
            "docs/dev/metric-resolution-bits-eval-plan.md#7-retain-every-iteration-and-all-provenance",
            "maintained DDEval judge result payloads",
        ],
        "note": "Core scores are copied; deeper-cause and contradiction summaries may be derived from complete judge JSON.",
    },
    "metric_retrieval": {
        "collection_mode": "copied_and_derived",
        "source_contracts": [
            "docs/dev/metric-resolution-bits-eval-plan.md#5-prove-bits-can-consume-the-treatment-resolution",
            "bits-capability-e2e-v2 verification acceptance contract",
        ],
        "note": "Requests and responses are copied from Bits logs/traces; cadence and gap fields are calculated from returned points.",
    },
    "evidence_use": {
        "collection_mode": "derived_review",
        "source_contracts": [
            "docs/dev/metric-resolution-bits-eval-plan.md#5-prove-bits-can-consume-the-treatment-resolution",
            "complete Bits conclusion, tool trace, and judge rationale",
        ],
        "note": "Reviewer annotations classify whether retrieved evidence actually influenced the causal reasoning.",
    },
    "safety": {
        "collection_mode": "copied_and_derived",
        "source_contracts": [
            "docs/dev/metric-resolution-bits-eval-plan.md#8-scale-across-independent-matched-captures",
            "complete judge contradiction and remediation outputs plus trace review",
        ],
        "note": "Judge fields are copied; false-resolution and noise classifications require evidence-bounded review.",
    },
    "reliability_efficiency": {
        "collection_mode": "copied",
        "source_contracts": [
            "docs/dev/metric-resolution-bits-eval-plan.md#7-retain-every-iteration-and-all-provenance",
            "DDEval workflow, LLMObs, and Bits tool logs",
        ],
        "note": "Copied from workflow and trace metadata when present; unavailable values remain null.",
    },
}

COHORT_DEFINITIONS: dict[str, dict[str, Any]] = {
    "primary": {
        "arm_count": 24,
        "pair_count": 12,
        "definition": "Twelve complete A/B pairs whose arms have usable event identity and diagnostic telemetry.",
    },
    "supplementary": {
        "arm_count": 1,
        "pair_count": 0,
        "definition": "2483 arm B is individually eligible, but its arm-A counterpart lacks a unique event identity.",
    },
    "excluded": {
        "arm_count": 5,
        "pair_count": 0,
        "definition": "2467 A/B and 2483 A lack usable event identity; corrected 2601 A/B are diagnostic-telemetry empty.",
    },
}


def _load(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text())


def _sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def _source(path: Path) -> dict[str, str]:
    return {"path": str(path), "sha256": _sha256(path)}


def _null_ddeval(status: str, reason: str) -> dict[str, Any]:
    return {
        "status": status,
        "reason": reason,
        "run_count": 0,
        "scenario_store_uuid": None,
        "workflow_run_id": None,
        "bits_trace_id": None,
        "judge": {
            "passed": None,
            "is_inconclusive": None,
            "match_probability": None,
            "deepjudge_score": None,
            "immediate_cause_found": None,
            "deeper_cause_found": None,
            "contradiction_count": None,
            "remediation_recall_f1": None,
        },
        "metric_retrieval": {
            "metric_tool_called": None,
            "intended_metric_queried": None,
            "query_scope_correct": None,
            "metric_call_count": None,
            "all_requests_interval_1000": None,
            "all_requests_raw_data": None,
            "all_windows_le_15m": None,
            "eligible_complete_1hz_call_count": None,
            "returned_effective_intervals_ms": None,
            "structural_gap_count": None,
            "truncation_or_override": None,
            "one_hz_evidence_used": None,
            "metric_use_classification": None,
        },
        "safety": {
            "false_resolution_claim": None,
            "noise_introduced_contradiction": None,
            "remediation_safe": None,
        },
        "reliability_efficiency": {
            "duration_seconds": None,
            "agent_iteration_count": None,
            "tool_call_count": None,
            "tool_error_count": None,
            "input_tokens": None,
            "output_tokens": None,
            "model_cost_usd": None,
        },
        "bits_log_notes": [],
        "artifacts": {"saved_results": None, "run_log": None, "llmobs_trace": None},
    }


def _cohort_index(plan: dict[str, Any]) -> dict[str, tuple[str, dict[str, Any]]]:
    result: dict[str, tuple[str, dict[str, Any]]] = {}
    for role in ("primary", "supplementary", "excluded"):
        for item in plan[role]:
            result[item["run_id"]] = (role, item)
    return result


def _preview_index(previews: dict[str, Any]) -> dict[str, dict[str, Any]]:
    return {item["run_id"]: item for item in previews["receipts"]}


def _format_int(value: int | None) -> str:
    return "—" if value is None else f"{value:,}"


def build(args: argparse.Namespace) -> dict[str, Any]:
    inventory = _load(args.inventory)
    arm_map = _load(args.arm_map)
    classification = _load(args.classification)
    cohort_plan = _load(args.cohort_plan)
    archive_gate = _load(args.archive_gate)
    previews = _load(args.dry_run_previews)
    size_audit = _load(args.size_audit)
    diagnosis = _load(args.diagnosis)

    classes = {
        item["episode_id"]: item
        for item in classification["records"]
        if item["episode_id"] in PREDICTIONS
    }
    cohorts = _cohort_index(cohort_plan)
    preview_by_run = _preview_index(previews)
    size_by_run = {item["run_id"]: item for item in size_audit["rows"]}
    gate_by_run = {item["run_id"]: item for item in archive_gate["gates"]}

    hypotheses = []
    for scenario in inventory["scenarios"]:
        episode_id = scenario["episode_id"]
        prediction = PREDICTIONS[episode_id]
        admission = classes[episode_id]
        hypotheses.append(
            {
                "episode_id": episode_id,
                "scenario_name": scenario["episode_name"],
                "admission_class": admission["admission_class"],
                "expected_direction": prediction["expected_direction"],
                "expected_to_improve": prediction["expected_direction"] in {"improve", "weak_improve"},
                "prediction_reason": prediction["reason"],
                "classification_evidence": admission["classification_evidence"],
                "packet_adaptation": _packet_adaptation(episode_id),
            }
        )

    hypothesis_by_episode = {item["episode_id"]: item for item in hypotheses}
    first_attempt = diagnosis["first_attempt"]
    arms: list[dict[str, Any]] = []
    for scenario in inventory["scenarios"]:
        episode_id = scenario["episode_id"]
        hypothesis = hypothesis_by_episode[episode_id]
        for arm_name, arm in scenario["arms"].items():
            run_id = arm["run_id"]
            role, cohort = cohorts[run_id]
            preview = preview_by_run.get(run_id)
            size = size_by_run.get(run_id)
            direct_gate = gate_by_run[run_id]

            if role in {"primary", "supplementary"}:
                ddeval_status = "pending_blocked_edp_projection_size"
                ddeval_reason = (
                    "Scenario Store ingestion is paused until archive_objects is removed from the projection, "
                    "fresh exact previews pass real validation, and a new write is authorized."
                )
            elif cohort["classification"].startswith("technical_event_ineligible"):
                ddeval_status = "not_planned_event_identity_ineligible"
                ddeval_reason = cohort["reason"]
            else:
                ddeval_status = "not_planned_telemetry_empty"
                ddeval_reason = cohort["reason"]

            if run_id == first_attempt["run_id"]:
                production_ingestion = {
                    "status": "failed_projection_too_large",
                    "http_status": first_attempt["http_status"],
                    "scenario_uuid": None,
                    "empty_dataset_uuid": first_attempt["empty_dataset_uuid"],
                    "error": diagnosis["exact_failure"]["portal_log"],
                    "retry_count": 0,
                }
            elif role in {"primary", "supplementary"}:
                production_ingestion = {
                    "status": "not_submitted_after_stop",
                    "http_status": None,
                    "scenario_uuid": None,
                    "empty_dataset_uuid": None,
                    "error": None,
                    "retry_count": 0,
                }
            else:
                production_ingestion = {
                    "status": "not_submitted_ineligible",
                    "http_status": None,
                    "scenario_uuid": None,
                    "empty_dataset_uuid": None,
                    "error": None,
                    "retry_count": 0,
                }

            execution = arm["execution"]
            archive = arm["archive"]
            arms.append(
                {
                    "episode_id": episode_id,
                    "scenario_name": scenario["episode_name"],
                    "admission_class": hypothesis["admission_class"],
                    "arm": arm_name,
                    "uses_1hz": bool(arm_map["arms"][str(episode_id)][arm_name]),
                    "expected_to_improve": hypothesis["expected_to_improve"],
                    "expected_direction": hypothesis["expected_direction"],
                    "run_id": run_id,
                    "campaign_id": arm["campaign_id"],
                    "campaign_role": arm["campaign_role"],
                    "cohort": {
                        "role": role,
                        "classification": cohort["classification"],
                        "reason": cohort["reason"],
                        "diagnostic_telemetry_gate_passed": cohort["diagnostic_telemetry_gate_passed"],
                        "diagnostic_telemetry_tracks": cohort["diagnostic_telemetry_tracks"],
                    },
                    "observed": {
                        "execution": {
                            "status": execution["status"],
                            "exit_code": execution["exit_code"],
                            "episode_status": execution["episode_status"],
                            "observation_status": execution["observation_status"],
                            "monitor_definitions_status": execution["monitor_definitions_status"],
                            "started_at": execution["started_at"],
                            "finished_at": execution["finished_at"],
                        },
                        "archive": {
                            "status": archive["status"],
                            "profile": archive["profile"],
                            "total_rows": archive["total_rows"],
                            "successful_archives": archive["successful_archives"],
                            "metadata_s3_path": archive["metadata_s3_path"],
                            "workflow_id": archive["workflow_id"],
                            "archive_run_id": archive["archive_run_id"],
                            "event_identity_source": archive["event_identity_source"],
                            "inventory_ddeval_alert_eligible": archive["ddeval_alert_eligible"],
                            "direct_archive_gate_passed": direct_gate["archive_gate_passed"],
                            "direct_event_gate_passed": direct_gate["ddeval_technical_gate_passed"],
                        },
                        "eval_data_portal": {
                            "dry_run_status": preview["scenario_status"] if preview else "not_run_event_ineligible",
                            "dry_run_http_status": preview["http_status"] if preview else None,
                            "dry_run_preview_passed": preview["preview_passed"] if preview else None,
                            "enrichment_wire_bytes": size["max_enrichment_struct_wire_bytes"] if size else None,
                            "within_131072_byte_limit": size["within_131072_byte_limit"] if size else None,
                            "production_ingestion": production_ingestion,
                        },
                        "ddeval": _null_ddeval(ddeval_status, ddeval_reason),
                    },
                }
            )

    return {
        "schema_version": 1,
        "kind": "global_1hz_unblinded_scoring_matrix",
        "matrix_version": "pre_ddeval_v1",
        "evidence_cutoff": diagnosis["generated_at"],
        "assignment_visibility": "evaluator_only_not_agent_visible",
        "agent_blinding_contract": (
            "uses_1hz and treatment labels must not be passed to the Bits agent, its prompt, "
            "Scenario Store metadata, or agent-visible scenario text"
        ),
        "status": {
            "campaign_arms": 30,
            "ddeval_runs_completed": 0,
            "ddeval_candidates_after_event_and_telemetry_gates": 25,
            "primary_arms": 24,
            "supplementary_arms": 1,
            "excluded_arms": 5,
            "blocker": diagnosis["exact_failure"]["portal_log"],
            "writes_paused": True,
        },
        "packet_adaptation_contract": PACKET_ADAPTATION_CONTRACT,
        "hypothesis_boolean_definition": (
            "expected_to_improve is true when the pre-DDEval expectation is any positive change in Bits "
            "investigation quality; weak_improve therefore maps to true"
        ),
        "scenario_hypotheses": hypotheses,
        "scoring_criteria": SCORING_CRITERIA,
        "scoring_criteria_provenance": SCORING_CRITERIA_PROVENANCE,
        "cohort_definitions": COHORT_DEFINITIONS,
        "pair_analysis_after_ddeval": [
            "deepjudge_score_delta_1hz_minus_control",
            "match_probability_delta_1hz_minus_control",
            "immediate_cause_delta_1hz_minus_control",
            "inconclusive_delta_1hz_minus_control",
            "contradiction_delta_1hz_minus_control",
            "duration_delta_1hz_minus_control",
            "tool_error_delta_1hz_minus_control",
            "metric_use_classification_change",
        ],
        "historical_nonconfirmatory_smoke_reference": {
            "scenario": "2483 serialization-regression",
            "status": "passed_mandatory_1hz_capability",
            "metric_call_count": 6,
            "eligible_complete_1hz_call_count": 2,
            "all_requests_interval_1000": True,
            "all_requests_raw_data_true": True,
            "all_request_windows_at_most_15_minutes": True,
            "deepjudge_score": 100,
            "match_probability": 0.9600688,
            "interpretation": (
                "Proves mandatory 1 Hz request/retrieval plumbing only; it is a different capture and is not "
                "an observed result for any arm in this matrix."
            ),
        },
        "source_evidence": {
            "corpus_inventory": _source(args.inventory),
            "private_arm_map": _source(args.arm_map),
            "classification_baseline": _source(args.classification),
            "cohort_plan": _source(args.cohort_plan),
            "archive_gate": _source(args.archive_gate),
            "dry_run_previews": _source(args.dry_run_previews),
            "enrichment_size_audit": _source(args.size_audit),
            "first_ingest_diagnosis": _source(args.diagnosis),
            "smoke_reference": _source(args.smoke_reference),
        },
        "arms": arms,
    }


def render_markdown(matrix: dict[str, Any]) -> str:
    lines = [
        "# GenSim global 1 Hz scoring matrix",
        "",
        "> **Evaluator only.** Never expose `uses_1hz` or treatment labels to Bits.",
        "",
        "Status: **30 arms recorded; 0 current-arm DDEval runs.** Twenty-five are blocked on the Eval Data Portal projection fix; five are excluded.",
        "",
        "## Scoring key",
        "",
        "Report raw values and paired `1 Hz - control` deltas. Admission fields are gates, not points; missing evidence stays `null`.",
        "",
        "| Group | Measures |",
        "|---|---|",
        "| Admission | Execution, event identity, archive integrity, diagnostic telemetry, Scenario Store readiness |",
        "| RCA quality | Pass/inconclusive, judge scores, immediate/deeper cause, remediation |",
        "| Metric retrieval | Intended query/scope, requested and effective cadence, completeness, gaps, overrides |",
        "| Evidence use | Whether 1 Hz evidence changed reasoning, supported the chain, and tested alternatives |",
        "| Safety | False-resolution claims, contradictions, remediation safety |",
        "| Reliability / efficiency | Duration, iterations, calls, errors, tokens, cost |",
        "",
        "## Scenario expectations and packet adaptations",
        "",
        "`weak_improve` counts as an expected improvement. `—` means no scenario-specific packet exception beyond the shared bounded Agent adaptation.",
        "",
        "| Episode | Scenario | Class | Expected | Why | Packet exception |",
        "|---:|---|---|---|---|---|",
    ]
    for h in matrix["scenario_hypotheses"]:
        adaptation = h["packet_adaptation"]
        special = "—" if adaptation["special_change"] == "none" else adaptation["special_change"]
        if adaptation["agent_values_adapter"] == "empty_env_single":
            special = f"empty-env adapter; {special}" if special != "—" else "empty-env adapter"
        if adaptation["selected_packet"] != "v3":
            special = f"`{adaptation['selected_packet']}`: {special}"
        lines.append(
            f"| {h['episode_id']} | {h['scenario_name']} | `{h['admission_class']}` | "
            f"`{h['expected_direction']}` | {h['prediction_reason']} | {special} |"
        )

    lines += [
        "",
        "## Cohorts",
        "",
        "Cohorts describe DDEval readiness, not treatment or outcome quality.",
        "",
        "| Cohort | Size | Definition |",
        "|---|---:|---|",
    ]
    for name, definition in matrix["cohort_definitions"].items():
        arm_label = "arm" if definition["arm_count"] == 1 else "arms"
        pair_label = "pair" if definition["pair_count"] == 1 else "pairs"
        size = f"{definition['arm_count']} {arm_label} / {definition['pair_count']} complete {pair_label}"
        lines.append(f"| `{name}` | {size} | {definition['definition']} |")

    lines += [
        "",
        "## 30-arm results",
        "",
        "`blocked` means no DDEval launched. Excluded rows remain visible.",
        "",
        "| Episode | Scenario | Arm / run | 1 Hz? | Expected improve? | Execution / event | Archive | Cohort | EDP / DDEval | Bits |",
        "|---:|---|---|:---:|:---:|---|---|---|---|---|",
    ]
    for arm in matrix["arms"]:
        execution = arm["observed"]["execution"]
        archive = arm["observed"]["archive"]
        edp = arm["observed"]["eval_data_portal"]
        ddeval = arm["observed"]["ddeval"]
        size = edp["enrichment_wire_bytes"]
        if edp["production_ingestion"]["status"] == "failed_projection_too_large":
            edp_text = f"ingest failed; {_format_int(size)} B"
        elif arm["cohort"]["role"] == "excluded":
            edp_text = "excluded"
        else:
            edp_text = f"previewed; {_format_int(size)} B"
        lines.append(
            f"| {arm['episode_id']} | {arm['scenario_name']} | {arm['arm']}<br>`{arm['run_id']}` | "
            f"{'yes' if arm['uses_1hz'] else 'no'} | {'yes' if arm['expected_to_improve'] else 'no'} | "
            f"{execution['status']}; `{execution['observation_status']}` | "
            f"{_format_int(archive['total_rows'])} rows / {_format_int(archive['successful_archives'])} objects | "
            f"`{arm['cohort']['role']}` | {edp_text}; `{ddeval['status']}` | not run |"
        )

    lines += [
        "",
        "## Update rule and current blocker",
        "",
        "Populate Bits/judge fields only from exact iteration artifacts. The historical 2483 smoke proves plumbing only and is not a current-arm result.",
        "",
        f"**Blocker:** `{matrix['status']['blocker']}`",
        "",
        "No Scenario Store write or DDEval launch is authorized until the projection fix is deployed, exact previews pass production-equivalent validation, and a fresh write is approved.",
        "",
        "Machine-readable fields, provenance, and evidence hashes: `gensim-global-1hz-scoring-matrix.json`.",
        "",
    ]
    return "\n".join(lines)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--inventory", type=Path, required=True)
    parser.add_argument("--arm-map", type=Path, required=True)
    parser.add_argument("--classification", type=Path, required=True)
    parser.add_argument("--cohort-plan", type=Path, required=True)
    parser.add_argument("--archive-gate", type=Path, required=True)
    parser.add_argument("--dry-run-previews", type=Path, required=True)
    parser.add_argument("--size-audit", type=Path, required=True)
    parser.add_argument("--diagnosis", type=Path, required=True)
    parser.add_argument("--smoke-reference", type=Path, required=True)
    parser.add_argument("--output-json", type=Path, default=DEFAULT_JSON)
    parser.add_argument("--output-markdown", type=Path, default=DEFAULT_MARKDOWN)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    matrix = build(args)
    args.output_json.write_text(json.dumps(matrix, indent=2, sort_keys=True) + "\n")
    args.output_markdown.write_text(render_markdown(matrix))


if __name__ == "__main__":
    main()
