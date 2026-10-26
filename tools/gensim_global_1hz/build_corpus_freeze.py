#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.13"
# dependencies = ["pyyaml>=6"]
# ///
"""Build immutable corpus-freeze and candidate-decision artifacts from pinned sources."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path
from typing import Any

import yaml

import freeze_corpus

CORE = {
    2483: ("resolution_exposed", "ordinary_dogstatsd_count", "resolution_exposed_count"),
    2481: ("resolution_exposed", "agent_owned_recurring_check", "resolution_exposed_gauge_or_recurring"),
    2498: ("resolution_exposed", "ordinary_dogstatsd_gauge", None),
    2467: ("resolution_exposed", "ordinary_dogstatsd_gauge", None),
    2590: ("resolution_exposed", "ordinary_dogstatsd_count", None),
    2592: ("resolution_exposed", "ordinary_dogstatsd_count", None),
    2563: ("resolution_exposed", "ordinary_dogstatsd_gauge", None),
    2502: ("metric_neutral", "durable_absence_signal", None),
    2643: ("metric_neutral", "producer_cadence_limited_10s", "metric_neutral"),
    2528: ("log_trace_led", "logs", None),
    2589: ("log_trace_led", "logs", "log_trace_led"),
    2674: ("log_trace_led", "logs", None),
}
RESERVES = {
    2597: ("resolution_exposed", "ordinary_dogstatsd_count_mixed_secondary", "mixed_secondary"),
    2666: ("metric_neutral", "producer_cadence_limited_30s", "high_confidence"),
    2684: ("log_trace_led", "logs", "high_confidence"),
}
METRIC_EVIDENCE = {
    2483: ["event_transformer.payload.encode.exceptions", "notification_router.job.enqueue.hits"],
    2481: ["container.cpu.user"],
    2498: ["kafka.consumer.lag_seconds", "mork.intake.unresolved_contexts"],
    2467: ["kafka.estimated_consumer_lag"],
    2590: ["dd.crawler.vendor_insights.batch_api.metrics.empty"],
    2592: ["portal audit queue-failure counters", "security notification counters"],
    2563: ["kafka.request.handler.avg.idle.pct.rate"],
    2502: ["mirror_destination.pipeline.trigger.count"],
    2643: ["kafka.consumer_lag"],
    2597: ["authentication failure DogStatsD count"],
    2666: ["compactor gauge on fixed 30-second producer ticker"],
}
WRONG_CONCLUSIONS = {
    2483: {
        "symptom_only": "The notification-router enqueue rate dropped.",
        "wrong_mechanism": "Notification-router queue saturation prevented workers from accepting otherwise valid jobs.",
        "explicit_distractor": "The delivery sink was independently dropping successfully transformed events.",
    },
    2481: {
        "symptom_only": "Recent mobile analytics results became stale because consumer lag increased.",
        "wrong_mechanism": "The event store could not persist normally processed events.",
        "explicit_distractor": "Audit-indexer backlog exhausted shared Kafka capacity and stalled the parser.",
    },
    2498: {
        "symptom_only": "Kafka consumers repeatedly rebalanced and monitor evaluations were suppressed.",
        "wrong_mechanism": "A Kafka broker outage directly disconnected otherwise healthy consumers.",
        "explicit_distractor": "Monitor evaluation infrastructure stopped evaluating healthy context-rewriter metrics.",
    },
    2467: {
        "symptom_only": "Entity resolution latency and stream consumer lag increased.",
        "wrong_mechanism": "Waypoint-router generated the lookup surge that overloaded entity-resolver.",
        "explicit_distractor": "A broad entity-resolver error-rate regression, rather than cache-miss load, caused the incident.",
    },
    2590: {
        "symptom_only": "Customer cloud metrics contained persistent zero-valued dips.",
        "wrong_mechanism": "The vendor API returned hard errors that stopped collector crawls.",
        "explicit_distractor": "Metrics-query-store aggregation converted valid nonzero points to zero.",
    },
    2592: {
        "symptom_only": "Audit events and security notifications disappeared in several regions.",
        "wrong_mechanism": "The flag-config-registry service was unavailable in all regions.",
        "explicit_distractor": "Portal request failures prevented successful requests from generating audit events.",
    },
    2563: {
        "symptom_only": "Kafka handler idle capacity fell and span consumer lag increased.",
        "wrong_mechanism": "A downstream span consumer code regression reduced consumption despite healthy brokers.",
        "explicit_distractor": "The traffic source increased span volume enough to overload an otherwise unchanged broker fleet.",
    },
    2502: {
        "symptom_only": "Mirror pipeline triggers stopped after worker reassignment.",
        "wrong_mechanism": "VCS intake stopped forwarding new push signals to the workflow.",
        "explicit_distractor": "The mirror destination rejected correctly scheduled synchronization activities.",
    },
    2643: {
        "symptom_only": "Stream consumers restarted and accumulated multi-minute lag.",
        "wrong_mechanism": "JVM heap growth exhausted the stream-processor heap.",
        "explicit_distractor": "The stream-shard broker failed and disconnected healthy scanner workers.",
    },
    2528: {
        "symptom_only": "Persistence workers logged Cassandra connectivity failures.",
        "wrong_mechanism": "The Cassandra ring was unavailable because all database nodes crashed.",
        "explicit_distractor": "Consumer lag alone caused healthy Cassandra client pools to discard connections.",
    },
    2589: {
        "symptom_only": "Anonymous trial registrations failed CSRF validation.",
        "wrong_mechanism": "Registration clients omitted the valid CSRF token from the request body.",
        "explicit_distractor": "Identity-token-broker session refresh timeouts caused the signup failures.",
    },
    2674: {
        "symptom_only": "Batch executors exited and dependent analytics pipelines failed.",
        "wrong_mechanism": "The table metadata service became unavailable to healthy workers.",
        "explicit_distractor": "A general cluster capacity shortage killed both table-format and non-table-format jobs.",
    },
    2597: {
        "symptom_only": "Authentication failures increased across the login endpoint.",
        "wrong_mechanism": "An identity database outage rejected otherwise valid credentials.",
        "explicit_distractor": "Infrastructure saturation caused healthy authentication workers to time out.",
    },
    2666: {
        "symptom_only": "Compliance API requests returned HTTP 500 errors while compaction fell behind.",
        "wrong_mechanism": "A compactor deployment regression made healthy fragments impossible to merge.",
        "explicit_distractor": "The compliance query database became unavailable to otherwise healthy services.",
    },
    2684: {
        "symptom_only": "Archive sink writes began failing during a long-running process.",
        "wrong_mechanism": "The archive destination ran out of storage capacity.",
        "explicit_distractor": "A network partition prevented valid refreshed credentials from reaching the archive service.",
    },
}


def file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        while chunk := source.read(1024 * 1024):
            digest.update(chunk)
    return digest.hexdigest()


def text_sha256(value: str) -> str:
    return hashlib.sha256(value.encode()).hexdigest()


def load_pins(path: Path) -> dict[int, dict[str, Any]]:
    payload = json.loads(path.read_text())
    return {entry["episode_id"]: entry for entry in payload["selection"]}


def scenario_entry(pin: dict[str, Any], metadata: tuple[str, str, Any], reserve: bool = False) -> dict[str, Any]:
    episode_path = Path(pin["directory"]) / pin["canonical_episode_file"]["path"]
    episode = yaml.safe_load(episode_path.read_text())
    summary = str(episode.get("description") or "").strip()
    long_description = str(episode.get("long_description") or "").strip()
    if not summary or not long_description:
        raise ValueError(f"episode {pin['episode_id']} lacks canonical ground truth text")
    stratum, source_class, extra = metadata
    entry = {
        "episode_id": pin["episode_id"],
        "episode_name": pin["episode_name"],
        "scenario_name": episode["name"],
        "app_name": episode["app_name"],
        "stratum": stratum,
        "source_class": source_class,
        "metric_evidence": METRIC_EVIDENCE.get(pin["episode_id"], []),
        "source_pin": {
            "gs_admin_datacenter": "us1.staging.dog",
            "app_bundle_version": pin["app_bundle_version"],
            "tar_sha256": pin["tar_sha256"],
            "tar_bytes": pin["tar_bytes"],
            "canonical_episode_path": pin["canonical_episode_file"]["path"],
            "canonical_episode_sha256": pin["canonical_episode_file"]["sha256"],
            "source_file_count": pin["source_file_count"],
        },
        "ground_truth": {
            "summary": summary,
            "long_description": long_description,
            "long_description_sha256": text_sha256(long_description),
            "monitor_title": str((episode.get("monitors") or [{}])[0].get("message") or (episode.get("monitors") or [{}])[0].get("name") or summary),
        },
        "qualification": {
            "g0_source_pin": "passed",
            "g1a_static_source_eligibility": "not_applicable" if stratum == "log_trace_led" else "passed",
            "g1b_judge_qualification": "pending_calibration",
        },
    }
    if reserve:
        entry["reserve_confidence"] = extra
        entry["replacement_scope"] = stratum
    elif extra:
        entry["pilot_role"] = extra
    if pin["episode_id"] == 2563:
        entry["classification_note"] = "High-confidence resolution-exposed: the ordinary DogStatsD gauge is directly treatment-eligible; consumer-lag logs are corroborating, not decisive."
    if pin["episode_id"] == 2597:
        entry["classification_note"] = "Mixed-secondary fallback only: authentication logs may remain more decisive than the eligible DogStatsD count; report separately if substituted."
    return entry


def parse_catalog(path: Path, core_ids: set[int], reserve_ids: set[int]) -> list[dict[str, Any]]:
    entries = []
    section = ""
    for line in path.read_text().splitlines():
        if line.startswith("## "):
            section = line[3:]
            continue
        if not line.startswith("|") or re.match(r"^\|[- :|]+$", line) or line.lower().startswith("| episode"):
            continue
        columns = [column.strip() for column in line.strip("|").split("|")]
        match = re.search(r"\[(\d+)\]", columns[0])
        if not match:
            continue
        episode_id = int(match.group(1))
        decision = "selected_core" if episode_id in core_ids else "selected_reserve" if episode_id in reserve_ids else "not_selected_initial_corpus"
        entries.append(
            {
                "episode_id": episode_id,
                "scenario_name": columns[1],
                "catalog_section": section,
                "decision": decision,
                "reason": "selected by frozen initial corpus" if decision != "not_selected_initial_corpus" else f"retained in source-screened catalog; not selected from {section.lower()}",
            }
        )
    return entries


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--core-source-manifest", type=Path, required=True)
    parser.add_argument("--reserve-source-manifest", type=Path, required=True)
    parser.add_argument("--candidate-catalog", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--rejection-ledger-output", type=Path, required=True)
    parser.add_argument("--judge-calibration-output", type=Path, required=True)
    args = parser.parse_args()

    core_pins = load_pins(args.core_source_manifest)
    reserve_pins = load_pins(args.reserve_source_manifest)
    if set(core_pins) != set(CORE):
        raise ValueError(f"core pin IDs differ: expected {sorted(CORE)}, got {sorted(core_pins)}")
    if set(reserve_pins) != set(RESERVES):
        raise ValueError(f"reserve pin IDs differ: expected {sorted(RESERVES)}, got {sorted(reserve_pins)}")

    core = [scenario_entry(core_pins[episode_id], CORE[episode_id]) for episode_id in CORE]
    reserves = [scenario_entry(reserve_pins[episode_id], RESERVES[episode_id], reserve=True) for episode_id in RESERVES]
    manifest = {
        "schema_version": 1,
        "experiment_version": "global_1hz_eval_v3",
        "status": "source_frozen_pending_judge_calibration",
        "composition": {"core": {"resolution_exposed": 7, "metric_neutral": 2, "log_trace_led": 3}, "reserves": {"resolution_exposed": 1, "metric_neutral": 1, "log_trace_led": 1}},
        "source_artifacts": {
            "core_source_manifest": {"path": str(args.core_source_manifest), "sha256": file_sha256(args.core_source_manifest)},
            "reserve_source_manifest": {"path": str(args.reserve_source_manifest), "sha256": file_sha256(args.reserve_source_manifest)},
            "candidate_catalog": {"path": str(args.candidate_catalog), "sha256": file_sha256(args.candidate_catalog)},
        },
        "core_scenarios": core,
        "reserves": reserves,
        "replacement_policy": {
            "completed_semantic_results_are_replaceable": False,
            "allowed_failure_classes": ["pre_disruption_qualification_failed", "capture_qualification_failed_due_to_preregistered_infrastructure_failure"],
            "maximum_replacements_per_failed_core_scenario": 1,
            "arm_blind_decision_required": True,
            "mixed_secondary_reporting": "A substituted mixed-secondary exposed reserve remains separately labeled in all analyses.",
        },
        "pilot_design": {
            "scenario_ids": [2483, 2481, 2643, 2589],
            "matched_capture_pairs_per_scenario": 1,
            "ddeval_iterations_per_group": 10,
            "initial_investigation_workers": 1,
            "capture_sets_are_confirmatory": False,
            "schedule_status": "not_generated_until_phases_0_through_4_pass",
        },
        "freeze": {
            "owner_approval_status": "pending",
            "judge_owner_approval_status": "pending",
            "content_sha256": "",
        },
        "blockers": [
            "Repeated judge calibration has not yet accepted canonical conclusions and rejected symptom-only, wrong-mechanism, and explicit-distractor conclusions.",
            "Phase 0 prompt/image/routing readback is incomplete.",
            "Phases 2-4 managed execution, telemetry qualification, and canonical smoke are incomplete.",
        ],
    }
    manifest["freeze"]["content_sha256"] = freeze_corpus.content_sha256(manifest)
    args.output.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n")

    decisions = parse_catalog(args.candidate_catalog, set(CORE), set(RESERVES))
    ledger = {
        "schema_version": 1,
        "candidate_catalog_sha256": file_sha256(args.candidate_catalog),
        "decision_count": len(decisions),
        "decisions": decisions,
    }
    args.rejection_ledger_output.write_text(json.dumps(ledger, indent=2, sort_keys=True) + "\n")

    calibration = {
        "schema_version": 1,
        "status": "prepared_not_run",
        "required_repetitions": "judge_owner_to_freeze_before_execution",
        "acceptance": "Every scenario must repeatedly accept canonical and reject symptom-only, wrong-mechanism, and explicit-distractor conclusions.",
        "cases": [
            {
                "scenario_uuid": f"gs-admin-episode-{item['episode_id']}",
                "episode_id": item["episode_id"],
                "scenario_name": item["scenario_name"],
                "app_name": item["app_name"],
                "monitor_title": item["ground_truth"]["monitor_title"],
                "ground_truth": item["ground_truth"]["long_description"],
                "variants": {
                    "canonical_correct": item["ground_truth"]["summary"],
                    "symptom_only": WRONG_CONCLUSIONS[item["episode_id"]]["symptom_only"],
                    "wrong_mechanism": WRONG_CONCLUSIONS[item["episode_id"]]["wrong_mechanism"],
                    "distractor": WRONG_CONCLUSIONS[item["episode_id"]]["explicit_distractor"],
                },
            }
            for item in core + reserves
        ],
    }
    args.judge_calibration_output.write_text(json.dumps(calibration, indent=2, sort_keys=True) + "\n")
    print(json.dumps({"manifest": str(args.output), "content_sha256": manifest["freeze"]["content_sha256"], "decision_count": len(decisions), "calibration_cases": len(calibration["cases"])}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
