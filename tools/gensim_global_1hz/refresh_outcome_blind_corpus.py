#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.13"
# dependencies = ["pyyaml>=6"]
# ///
"""Refresh the global 1 Hz corpus from GS Admin with outcome-blind admission.

A bootstrap run binds the archived human source-screening taxonomy to the current
GS Admin bundle identities. Every later refresh requires that machine-readable
identity baseline and fails closed when a version or source hash changes.
Selection never reads an observed score, expected benefit, RCA outcome, or
resolution counterfactual.
"""

from __future__ import annotations

import argparse
import copy
import hashlib
import json
import re
import subprocess
import time
import urllib.error
import urllib.request
from collections import Counter
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path
from typing import Any, Iterable

import yaml

GS_ADMIN = "https://gs-admin.us1.staging.dog"
UI_ROOT = "https://gs-admin.static-app.us1.staging.dog/episodes"
SPECIAL_IRRELEVANT_IDS = {2586, 2657}
SECTION_CLASSES = {
    "High-confidence resolution-exposed candidates": "resolution_exposed",
    "High-confidence metric-neutral controls": "metric_neutral",
    "Source-eligible mixed secondary candidates": "mixed_secondary",
    "Log/trace-led future candidates": "resolution_irrelevant",
}
PANEL_ORDER = (
    "resolution_exposed",
    "metric_neutral",
    "mixed_secondary",
    "resolution_irrelevant",
)
CAPABILITY_REFERENCE = {
    "episode_id": 2483,
    "status": "passed_nonconfirmatory_mandatory_1hz_capability",
    "evidence": "/Users/ella.taira/episode-bundles/1hz-runs/episode-2483-1hz-20260816-164841-derivatives/bits-capability-e2e-v2-verification.json",
    "eligible_agent_owned_calls": 2,
    "points_per_eligible_call": 900,
    "spacing_ms": 1000,
    "structural_gaps": 0,
    "scope": "plumbing_and_retrieval_capability_not_treatment_effect",
}


def sha256_bytes(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def canonical_sha256(value: Any) -> str:
    payload = json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode()
    return sha256_bytes(payload)


def class_counts(items: Iterable[dict[str, Any]]) -> dict[str, int]:
    counts = Counter(item["admission_class"] for item in items)
    return {name: counts[name] for name in PANEL_ORDER if counts[name]}


def classify_source(episode_id: int, catalog_section: str) -> dict[str, str]:
    if catalog_section in SECTION_CLASSES:
        return {
            "admission_class": SECTION_CLASSES[catalog_section],
            "admission_status": "admitted",
        }
    if catalog_section == "Excluded or repair-required" and episode_id in SPECIAL_IRRELEVANT_IDS:
        return {
            "admission_class": "resolution_irrelevant",
            "admission_status": "admitted",
        }
    return {"admission_class": "source_defect", "admission_status": "blocked"}


def admission_status(record: dict[str, Any]) -> str:
    required = (
        record.get("admission_status") == "admitted",
        record.get("causalgen_gates") == "passed",
        record.get("do_not_use") is False,
        record.get("canonical_ground_truth_available") is True,
        record.get("runnable_episode_yaml") is True,
    )
    return "admitted" if all(required) else "blocked"


def duration_seconds(path: str, value: Any) -> float | None:
    lower = path.lower()
    if "disruption" not in lower or not any(word in lower for word in ("duration", "second", "minute")):
        return None
    if isinstance(value, bool):
        return None
    if isinstance(value, (int, float)):
        return float(value) * (60 if "minute" in lower else 1)
    if not isinstance(value, str):
        return None
    match = re.fullmatch(
        r"\s*(\d+(?:\.\d+)?)\s*(s|sec|secs|second|seconds|m|min|mins|minute|minutes)?\s*",
        value,
        re.IGNORECASE,
    )
    if not match:
        return None
    amount = float(match.group(1))
    unit = (match.group(2) or ("minutes" if "minute" in lower else "seconds")).lower()
    return amount * (60 if unit.startswith("m") else 1)


def _walk_scalars(value: Any, path: str = "") -> Iterable[tuple[str, Any]]:
    if isinstance(value, dict):
        for key, child in value.items():
            child_path = f"{path}.{key}" if path else key
            if isinstance(child, (str, int, float, bool)):
                yield child_path, child
            yield from _walk_scalars(child, child_path)
    elif isinstance(value, list):
        for index, child in enumerate(value):
            yield from _walk_scalars(child, f"{path}[{index}]")


def disruption_duration(episode: Any) -> float | None:
    candidates = [
        parsed
        for path, value in _walk_scalars(episode)
        if (parsed := duration_seconds(path, value)) is not None
    ]
    return min(candidates) if candidates else None


def select_panel(
    candidates: Iterable[dict[str, Any]], snapshot_sha256: str, irrelevant_count: int = 3
) -> list[dict[str, Any]]:
    admitted = [item for item in candidates if item.get("admission_status") == "admitted"]
    selected: list[dict[str, Any]] = []
    for admission_class in PANEL_ORDER[:-1]:
        selected.extend(
            sorted(
                (item for item in admitted if item["admission_class"] == admission_class),
                key=lambda item: item["episode_id"],
            )
        )
    irrelevant = [item for item in admitted if item["admission_class"] == "resolution_irrelevant"]
    irrelevant.sort(
        key=lambda item: hashlib.sha256(
            f"{snapshot_sha256}:{item['episode_id']}".encode()
        ).hexdigest()
    )
    selected.extend(irrelevant[:irrelevant_count])
    return sorted(selected, key=lambda item: (PANEL_ORDER.index(item["admission_class"]), item["episode_id"]))


CLASSIFICATION_IDENTITY_FIELDS = (
    "app_bundle_version",
    "archive_key",
    "manifest_sha256",
    "canonical_episode_sha256",
)


def classification_identity_matches(
    current: dict[str, Any], baseline: dict[str, Any]
) -> bool:
    return all(current.get(field) == baseline.get(field) for field in CLASSIFICATION_IDENTITY_FIELDS)


def ensure_no_classification_identity_changes(
    candidates: Iterable[dict[str, Any]],
) -> None:
    changed = [
        item["episode_id"]
        for item in candidates
        if "classification_identity_changed_requires_static_review"
        in item.get("blocking_reasons", [])
    ]
    if changed:
        raise ValueError(
            "classification baseline identity changed for episode IDs "
            f"{changed}; perform a new static source review before selection"
        )


def load_classification_baseline(
    path: Path | None, bootstrap: bool
) -> dict[int, dict[str, Any]]:
    if path is None:
        if bootstrap:
            return {}
        raise ValueError(
            "a classification baseline is required unless --bootstrap-classifications is explicit"
        )
    payload = json.loads(path.read_text())
    if payload.get("schema_version") != 1 or not isinstance(payload.get("records"), list):
        raise ValueError("classification baseline must be a schema-version-1 record list")
    records = {record["episode_id"]: record for record in payload["records"]}
    if len(records) != len(payload["records"]):
        raise ValueError("classification baseline contains duplicate episode IDs")
    return records


def parse_catalog(path: Path) -> dict[int, dict[str, str]]:
    records: dict[int, dict[str, str]] = {}
    section = ""
    for line in path.read_text().splitlines():
        if line.startswith("## "):
            section = line[3:].strip()
            continue
        if not line.startswith("|") or line.lower().startswith("| episode") or re.match(r"^\|[- :|]+$", line):
            continue
        columns = [column.strip() for column in line.strip("|").split("|")]
        match = re.search(r"\[(\d+)\]", columns[0]) if columns else None
        if not match or len(columns) < 6:
            continue
        episode_id = int(match.group(1))
        records[episode_id] = {
            "catalog_section": section,
            "scenario_name": columns[1].strip("`"),
            "app_name": columns[2].strip("`"),
            "prior_classification": columns[3].strip("`"),
            "classification_evidence": columns[4].strip("`"),
            "ground_truth_summary": columns[5].strip("`"),
        }
    return records


class GSAdminClient:
    def __init__(self, header: str, base_url: str = GS_ADMIN) -> None:
        self.base_url = base_url.rstrip("/")
        self.header_name, self.header_value = header.split(": ", 1)

    def request(self, path: str, body: Any | None = None, attempts: int = 4) -> bytes:
        data = json.dumps(body).encode() if body is not None else None
        headers = {self.header_name: self.header_value}
        if data is not None:
            headers["Content-Type"] = "application/json"
        for attempt in range(attempts):
            try:
                request = urllib.request.Request(
                    self.base_url + path,
                    data=data,
                    headers=headers,
                    method="POST" if data is not None else "GET",
                )
                return urllib.request.urlopen(request, timeout=120).read()
            except Exception:
                if attempt + 1 == attempts:
                    raise
                time.sleep(0.4 * (attempt + 1))
        raise AssertionError("unreachable")

    def json(self, path: str) -> Any:
        return json.loads(self.request(path))

    def archive_file(self, episode_id: int, version: int, path: str) -> bytes:
        return self.request(
            f"/internal/api/v1/episode/{episode_id}/app-bundle/{version}/archive-file",
            {"data": {"attributes": {"file_name": path}}},
        )


def auth_header(ddtool: str, datacenter: str) -> str:
    return subprocess.check_output(
        [ddtool, "auth", "token", "rapid-sims", "--datacenter", datacenter, "--http-header"],
        text=True,
    ).strip()


def manifest_paths(raw_manifest: bytes) -> list[str]:
    paths = []
    for line in raw_manifest.decode(errors="replace").splitlines():
        parts = line.split(" ", 2)
        if len(parts) == 3:
            paths.append(parts[2])
    return paths


def latest_bundle(client: GSAdminClient, episode_id: int) -> dict[str, Any]:
    bundles = client.json(f"/internal/api/v1/episode/{episode_id}/app-bundles?limit=1000")["data"]
    version = max(item["attributes"]["app_bundle_version"] for item in bundles)
    attributes = client.json(f"/internal/api/v1/episode/{episode_id}/app-bundle/{version}")["data"]["attributes"]
    return {"episode_id": episode_id, "version": version, "attributes": attributes}


def inspect_passed_source(
    client: GSAdminClient,
    latest: dict[str, Any],
    catalog: dict[int, dict[str, str]],
    classification_baseline: dict[int, dict[str, Any]],
) -> dict[str, Any]:
    episode_id = latest["episode_id"]
    version = latest["version"]
    attributes = latest["attributes"]
    tags = attributes.get("tags") or []
    raw_manifest = client.request(f"/internal/api/v1/episode/{episode_id}/app-bundle/{version}/manifest")
    paths = manifest_paths(raw_manifest)
    episode_paths = sorted(
        path for path in paths if path.startswith("episodes/") and path.endswith((".yaml", ".yml"))
    )
    runnable = len(episode_paths) == 1
    episode: Any = None
    raw_episode = b""
    parse_error = None
    if runnable:
        try:
            raw_episode = client.archive_file(episode_id, version, episode_paths[0])
            episode = yaml.safe_load(raw_episode)
            runnable = isinstance(episode, dict)
        except Exception as error:
            parse_error = f"{type(error).__name__}: {error}"
            runnable = False
    prior = catalog.get(episode_id, {})
    baseline = classification_baseline.get(episode_id)
    if baseline is not None:
        source_class = {
            "admission_class": baseline["admission_class"],
            "admission_status": baseline["admission_status"],
        }
    else:
        source_class = classify_source(episode_id, prior.get("catalog_section", ""))
    summary = ""
    long_description = ""
    monitors: list[dict[str, Any]] = []
    if isinstance(episode, dict):
        summary = str(episode.get("description") or "").strip()
        long_description = str(episode.get("long_description") or "").strip()
        for monitor in episode.get("monitors") or []:
            if isinstance(monitor, dict):
                monitors.append(
                    {
                        "type": monitor.get("type"),
                        "query": monitor.get("query"),
                        "is_symptom": monitor.get("is_symptom"),
                    }
                )
    ground_truth_available = bool(summary and (long_description or prior.get("ground_truth_summary")))
    record: dict[str, Any] = {
        "episode_id": episode_id,
        "gs_admin_url": f"{UI_ROOT}/{episode_id}",
        "app_bundle_version": version,
        "archive_key": attributes.get("archive_key"),
        "bundle_tags": tags,
        "causalgen_gates": "passed" if "causalgen.gates=passed" in tags else "not-passed",
        "do_not_use": "causalgen.do-not-use=true" in tags,
        "manifest_sha256": sha256_bytes(raw_manifest),
        "canonical_episode_path": episode_paths[0] if len(episode_paths) == 1 else None,
        "canonical_episode_sha256": sha256_bytes(raw_episode) if raw_episode else None,
        "runnable_episode_yaml": runnable,
        "episode_parse_error": parse_error,
        "canonical_ground_truth_available": ground_truth_available,
        "scenario_name": (episode or {}).get("name") if isinstance(episode, dict) else prior.get("scenario_name"),
        "app_name": (episode or {}).get("app_name") if isinstance(episode, dict) else prior.get("app_name"),
        "ground_truth_summary": summary or prior.get("ground_truth_summary"),
        "disruption_duration_seconds": disruption_duration(episode) if isinstance(episode, dict) else None,
        "monitors": monitors,
        "catalog_section": prior.get("catalog_section"),
        "prior_classification": prior.get("prior_classification"),
        "classification_evidence": prior.get("classification_evidence"),
        **source_class,
        "capability_status": (
            CAPABILITY_REFERENCE["status"] if episode_id == CAPABILITY_REFERENCE["episode_id"] else "not_run"
        ),
        "campaign_archive_schema_compatibility": "not_an_admission_gate_requires_separate_preflight",
    }
    identity_changed = baseline is not None and not classification_identity_matches(
        record, baseline["source_identity"]
    )
    if identity_changed:
        record["admission_class"] = "source_defect"
        record["admission_status"] = "blocked"
    else:
        record["admission_status"] = admission_status(record)
    if record["admission_status"] == "blocked":
        reasons = []
        if identity_changed:
            reasons.append("classification_identity_changed_requires_static_review")
        elif source_class["admission_status"] == "blocked":
            reasons.append("source_screening_defect")
        if record["causalgen_gates"] != "passed":
            reasons.append("latest_bundle_did_not_pass_causalgen_gates")
        if record["do_not_use"]:
            reasons.append("latest_bundle_do_not_use")
        if not runnable:
            reasons.append("canonical_episode_yaml_not_runnable")
        if not ground_truth_available:
            reasons.append("canonical_ground_truth_unavailable")
        record["blocking_reasons"] = reasons
    return record


def source_pin(client: GSAdminClient, record: dict[str, Any]) -> dict[str, Any]:
    archive = client.request(
        f"/internal/api/v1/episode/{record['episode_id']}/app-bundle/{record['app_bundle_version']}/archive"
    )
    return {
        "episode_id": record["episode_id"],
        "app_bundle_version": record["app_bundle_version"],
        "archive_key": record["archive_key"],
        "archive_sha256": sha256_bytes(archive),
        "archive_bytes": len(archive),
        "manifest_sha256": record["manifest_sha256"],
        "canonical_episode_path": record["canonical_episode_path"],
        "canonical_episode_sha256": record["canonical_episode_sha256"],
    }


def freeze_hash(manifest: dict[str, Any]) -> str:
    value = copy.deepcopy(manifest)
    value["freeze"].pop("content_sha256", None)
    payload = (
        json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False) + "\n"
    ).encode()
    return sha256_bytes(payload)


def render_catalog(
    inventory: dict[str, Any], candidates: list[dict[str, Any]], panel_ids: set[int]
) -> str:
    admitted = [item for item in candidates if item["admission_status"] == "admitted"]
    blocked = [item for item in candidates if item["admission_status"] == "blocked"]
    lines = [
        "# GenSim global 1 Hz outcome-blind candidate catalog",
        "",
        "> **Active source-screening record.** Admission validates provenance, CausalGen quality, runnable ground truth, telemetry class, and capability-smoke readiness. It does not require or predict a 1 Hz benefit.",
        "",
        f"Snapshot SHA-256: `{inventory['snapshot_sha256']}`. GS Admin returned {inventory['archived_episode_count']} archived episodes; {inventory['passed_latest_bundle_count']} latest bundles passed CausalGen gates.",
        "",
        "## Admission policy",
        "",
        "Admitted scenarios may help, be neutral, or regress under 1 Hz. Existing RCA improvement, a 15-second counterfactual, a short incident, and `scenario.yaml.rca_causal_chain` are not source-admission gates. They are capability, execution, analysis, or separate archive-compatibility concerns.",
        "",
        "The score panel includes every admitted resolution-exposed, metric-neutral, and mixed-secondary source, plus a deterministic SHA-256-ranked sample of resolution-irrelevant sources. Selection does not inspect capability outcomes or observed scores.",
        "",
        "## Deterministic score panel",
        "",
        "| Episode | Scenario | App | Admission class | Capability | Static evidence |",
        "|---|---|---|---|---|---|",
    ]
    for item in candidates:
        if item["episode_id"] not in panel_ids:
            continue
        lines.append(
            f"| [{item['episode_id']}]({item['gs_admin_url']}) | {item.get('scenario_name') or '—'} | {item.get('app_name') or '—'} | `{item['admission_class']}` | `{item['capability_status']}` | {item.get('classification_evidence') or 'source-screened telemetry path'} |"
        )
    lines.extend(
        [
            "",
            "## Admitted population",
            "",
            "| Episode | Scenario | Admission class | Panel |",
            "|---|---|---|---|",
        ]
    )
    for item in admitted:
        lines.append(
            f"| [{item['episode_id']}]({item['gs_admin_url']}) | {item.get('scenario_name') or '—'} | `{item['admission_class']}` | {'yes' if item['episode_id'] in panel_ids else 'no'} |"
        )
    lines.extend(
        [
            "",
            "## Blocked source defects",
            "",
            "| Episode | Scenario | Reasons |",
            "|---|---|---|",
        ]
    )
    for item in blocked:
        lines.append(
            f"| [{item['episode_id']}]({item['gs_admin_url']}) | {item.get('scenario_name') or '—'} | {', '.join(item.get('blocking_reasons') or ['source defect'])} |"
        )
    lines.extend(
        [
            "",
            "## Interpretation",
            "",
            "Report results by stratum and for the complete frozen panel. Keep every semantically valid completed result, including null and negative effects. Replace only preregistered infrastructure-invalid captures, without observing arm outcomes.",
            "",
        ]
    )
    return "\n".join(lines)


def render_matrix(manifest: dict[str, Any]) -> str:
    lines = [
        "# GenSim global 1 Hz outcome-blind corpus and gate matrix",
        "",
        "> **Active admission and readiness contract.** Scenario admission is outcome-blind. Episode 2483 is a plumbing calibration reference, not proof of a treatment effect and not a template for selecting favorable incidents.",
        "",
        f"Status: `{manifest['status']}`. Corpus hash: `{manifest['freeze']['content_sha256']}`.",
        "",
        "## Gate separation",
        "",
        "| Stage | Gate | Pass condition | Not required |",
        "|---|---|---|---|",
        "| Source admission | S0 provenance | Archived GS Admin identity and immutable latest passed bundle are pinned. | Observed treatment outcome |",
        "| Source admission | S1 quality | Latest bundle has `causalgen.gates=passed`, is not do-not-use, and has runnable episode YAML plus objective ground truth. | New campaign archive-schema compatibility |",
        "| Source admission | S2 telemetry class | Source evidence classifies the decisive path as resolution-exposed, neutral, mixed, or irrelevant. | Expected positive 1 Hz effect |",
        "| Capability | C0 deployment | Immutable custom Agent identity and effective control/treatment configuration are read back. | Better RCA score |",
        "| Capability | C1 retrieval | Eligible sources can return raw one-second points without unexplained gaps, truncation, or coarse override. | Same behavior from every telemetry class |",
        "| Capability | C2 plumbing RCA | Bits can use available evidence and the judge can evaluate the canonical cause. | Confirmatory treatment effect |",
        "| Experiment | E0 matched execution | Control and treatment use equivalent source, resources, workload, and windows. | Favorable arm result |",
        "| Experiment | E1 counterfactual | Exact 15-second data is derived from the same native raw event. | Pre-existing counterfactual in source bundle |",
        "| Experiment | E2 scoring | RCA, latency, false confidence, reliability, resource, and data-volume outcomes are retained for every valid result. | Positive or statistically significant delta |",
        "| Publication | P0 evidence | Cleanup, archive/readback where required, Scenario Store, and evaluation identities are complete. | Deleting null or negative outcomes |",
        "",
        "## Frozen score panel",
        "",
        "| Episode | Scenario | Class | Capability status | Duration metadata |",
        "|---|---|---|---|---|",
    ]
    for item in manifest["score_panel"]:
        duration = item.get("disruption_duration_seconds")
        lines.append(
            f"| {item['episode_id']} | {item.get('scenario_name') or '—'} | `{item['admission_class']}` | `{item['capability_status']}` | {f'{duration:g}s' if isinstance(duration, (int, float)) else 'not declared'} |"
        )
    lines.extend(
        [
            "",
            "## Score policy",
            "",
            "- Freeze membership before treatment outcomes are observed.",
            "- Report resolution-exposed effectiveness, complete-panel operational utility, and neutral/irrelevant safety separately.",
            "- Treat null and negative effects as valid results.",
            "- Do not replace completed semantic results. Replace only preregistered infrastructure-invalid captures using an arm-blind decision.",
            "- Classify fleeting, boundary, and sustained behavior from observed metric trajectories after capture; do not select sources because a short signal is expected to favor 1 Hz.",
            "",
            "## Capability reference",
            "",
            "Episode 2483 passed a nonconfirmatory mandatory 1 Hz retrieval smoke with two eligible Agent-owned calls, 900 one-second points per call, and no structural gaps. This establishes the plumbing floor only. It does not establish natural tool selection, repeatability, or a treatment effect.",
            "",
        ]
    )
    return "\n".join(lines)


def refresh(args: argparse.Namespace) -> dict[str, Any]:
    catalog = parse_catalog(args.archived_catalog)
    classification_baseline = load_classification_baseline(
        args.classification_baseline, args.bootstrap_classifications
    )
    header = auth_header(args.ddtool, args.datacenter)
    client = GSAdminClient(header, args.gs_admin)
    listing = client.json("/internal/api/v1/episodes?limit=1000&tags=causalgen.state%3Darchived")
    episode_ids = sorted(int(item["id"]) for item in listing["data"])

    latest_rows: list[dict[str, Any]] = []
    with ThreadPoolExecutor(max_workers=args.workers) as executor:
        futures = {executor.submit(latest_bundle, client, episode_id): episode_id for episode_id in episode_ids}
        for future in as_completed(futures):
            latest_rows.append(future.result())
    latest_rows.sort(key=lambda item: item["episode_id"])
    gate_counts = Counter()
    passed_rows = []
    for row in latest_rows:
        tags = row["attributes"].get("tags") or []
        if "causalgen.do-not-use=true" in tags:
            gate_counts["do_not_use"] += 1
        elif "causalgen.gates=passed" in tags:
            gate_counts["passed"] += 1
            passed_rows.append(row)
        elif "causalgen.gates=soft-pass" in tags:
            gate_counts["soft_pass"] += 1
        else:
            gate_counts["other"] += 1

    candidates: list[dict[str, Any]] = []
    with ThreadPoolExecutor(max_workers=args.workers) as executor:
        futures = {
            executor.submit(
                inspect_passed_source, client, row, catalog, classification_baseline
            ): row["episode_id"]
            for row in passed_rows
        }
        for future in as_completed(futures):
            candidates.append(future.result())
    candidates.sort(key=lambda item: item["episode_id"])
    current_ids = {item["episode_id"] for item in candidates}
    if set(catalog) != current_ids:
        raise ValueError(
            "archived source-screening catalog IDs do not match current passed latest-bundle IDs; "
            "perform a new static source review before selection"
        )
    if classification_baseline and set(classification_baseline) != current_ids:
        raise ValueError(
            "classification baseline IDs do not match current passed latest-bundle IDs; "
            "perform a new static source review before selection"
        )
    ensure_no_classification_identity_changes(candidates)

    snapshot_basis = [
        {
            "episode_id": item["episode_id"],
            "app_bundle_version": item["app_bundle_version"],
            "archive_key": item["archive_key"],
            "manifest_sha256": item["manifest_sha256"],
            "canonical_episode_sha256": item["canonical_episode_sha256"],
            "admission_class": item["admission_class"],
            "admission_status": item["admission_status"],
        }
        for item in candidates
    ]
    snapshot_sha = canonical_sha256(snapshot_basis)
    panel = select_panel(candidates, snapshot_sha, args.irrelevant_count)
    panel_ids = {item["episode_id"] for item in panel}
    admitted = [item for item in candidates if item["admission_status"] == "admitted"]
    irrelevant_ranked = [
        item
        for item in select_panel(
            [item for item in admitted if item["admission_class"] == "resolution_irrelevant"],
            snapshot_sha,
            irrelevant_count=len(admitted),
        )
        if item["admission_class"] == "resolution_irrelevant"
    ]
    reserve_ids = {
        item["episode_id"] for item in irrelevant_ranked[args.irrelevant_count : args.irrelevant_count + 3]
    }

    pins: dict[int, dict[str, Any]] = {}
    with ThreadPoolExecutor(max_workers=min(args.workers, 8)) as executor:
        futures = {executor.submit(source_pin, client, item): item["episode_id"] for item in panel}
        for future in as_completed(futures):
            pins[futures[future]] = future.result()

    baseline_payload = {
        "schema_version": 1,
        "documentation": {
            "role": "current_bundle_identity_guard_for_human_source_classification",
            "bootstrap_source": str(args.archived_catalog),
            "selection_uses_observed_outcomes": False,
            "refresh_rule": "any identity change requires a new static source review",
        },
        "records": [
            {
                "episode_id": item["episode_id"],
                "scenario_name": item["scenario_name"],
                "catalog_section": item["catalog_section"],
                "admission_class": (
                    classification_baseline[item["episode_id"]]["admission_class"]
                    if classification_baseline
                    else classify_source(item["episode_id"], item["catalog_section"])["admission_class"]
                ),
                "admission_status": (
                    classification_baseline[item["episode_id"]]["admission_status"]
                    if classification_baseline
                    else classify_source(item["episode_id"], item["catalog_section"])["admission_status"]
                ),
                "classification_evidence": item["classification_evidence"],
                "source_identity": {
                    field: item[field] for field in CLASSIFICATION_IDENTITY_FIELDS
                },
            }
            for item in candidates
        ],
    }
    baseline_payload["content_sha256"] = canonical_sha256(baseline_payload["records"])

    inventory = {
        "gs_admin": args.gs_admin,
        "query": "causalgen.state=archived",
        "archived_episode_count": len(episode_ids),
        "latest_bundle_count": len(latest_rows),
        "passed_latest_bundle_count": gate_counts["passed"],
        "soft_pass_latest_bundle_count": gate_counts["soft_pass"],
        "other_latest_bundle_count": gate_counts["other"],
        "do_not_use_latest_bundle_count": gate_counts["do_not_use"],
        "source_screened_count": len(candidates),
        "admitted_count": len(admitted),
        "blocked_count": len(candidates) - len(admitted),
        "admitted_class_counts": class_counts(admitted),
        "snapshot_sha256": snapshot_sha,
    }

    panel_records = []
    for item in panel:
        value = {key: item[key] for key in (
            "episode_id",
            "scenario_name",
            "app_name",
            "admission_class",
            "capability_status",
            "ground_truth_summary",
            "classification_evidence",
            "disruption_duration_seconds",
            "monitors",
        )}
        value["source_pin"] = pins[item["episode_id"]]
        panel_records.append(value)

    manifest: dict[str, Any] = {
        "schema_version": 2,
        "experiment_version": "global_1hz_outcome_blind_v1",
        "status": "source_selected_pending_capability_smokes",
        "documentation": {
            "role": "active_outcome_blind_readiness_input",
            "governing_design": "docs/dev/gensim-global-1hz-scaled-experiment-plan.md",
            "governing_campaign_contract": "docs/dev/gensim-global-1hz-episode-ctl-campaign-plan.md",
            "archived_predecessor": "docs/dev/archive/gensim-global-1hz/2026-08-25/gensim-global-1hz-corpus-freeze.json",
            "classification_baseline": "docs/dev/gensim-global-1hz-classification-baseline.json",
            "classification_baseline_sha256": baseline_payload["content_sha256"],
        },
        "inventory": inventory,
        "admission_policy": {
            "required": [
                "archived GS Admin episode with latest causalgen.gates=passed bundle",
                "not tagged causalgen.do-not-use=true",
                "runnable canonical episode YAML and objective ground truth",
                "source-screened telemetry class",
            ],
            "explicit_non_gates": [
                "observed positive 1 Hz effect",
                "pre-existing one-second versus fifteen-second counterfactual",
                "pre-existing RCA score improvement",
                "short or fleeting incident duration",
                "scenario.yaml.rca_causal_chain compatibility with the separate campaign archiver",
            ],
        },
        "composition": class_counts(panel),
        "score_panel": panel_records,
        "reserves": [
            {
                "episode_id": item["episode_id"],
                "scenario_name": item["scenario_name"],
                "admission_class": item["admission_class"],
                "selection": "next_sha256_ranked_resolution_irrelevant_source",
            }
            for item in candidates
            if item["episode_id"] in reserve_ids
        ],
        "capability_reference": CAPABILITY_REFERENCE,
        "judge_calibration": {
            "artifact": "docs/dev/gensim-global-1hz-judge-calibration-cases.json",
            "status": "pending_current_panel_cases",
            "cases_complete": False,
            "missing_episode_ids": [2574, 2497, 2500, 2601],
            "evidence_sha256": None,
            "admission_impact": "none_source_admission_is_outcome_blind",
            "execution_readiness_impact": "blocking",
        },
        "replacement_policy": {
            "completed_semantic_results_are_replaceable": False,
            "arm_blind_decision_required": True,
            "allowed_failure_classes": [
                "pre_disruption_infrastructure_failure",
                "capture_invalid_due_to_preregistered_platform_or_telemetry_failure",
            ],
            "null_negative_or_regressive_results_are_retained": True,
        },
        "score_policy": {
            "report_separately": [
                "resolution_exposed_effectiveness",
                "complete_panel_operational_utility",
                "neutral_mixed_and_irrelevant_safety",
            ],
            "same_event_15_second_counterfactual": "post_capture_analysis_not_admission",
            "duration_cohort": "assigned_from_observed_decisive_metric_trajectory_not_selection_expectation",
            "membership_frozen_before_treatment_outcomes": True,
        },
        "freeze": {"owner_approval_status": "pending", "content_sha256": ""},
    }
    manifest["freeze"]["content_sha256"] = freeze_hash(manifest)

    decisions = []
    for item in candidates:
        decision = (
            "selected_score_panel"
            if item["episode_id"] in panel_ids
            else "selected_reserve"
            if item["episode_id"] in reserve_ids
            else "admitted_not_selected"
            if item["admission_status"] == "admitted"
            else "blocked_source_defect"
        )
        decisions.append(
            {
                "episode_id": item["episode_id"],
                "scenario_name": item["scenario_name"],
                "app_bundle_version": item["app_bundle_version"],
                "admission_class": item["admission_class"],
                "admission_status": item["admission_status"],
                "decision": decision,
                "reason": (
                    "outcome-blind deterministic panel membership"
                    if decision == "selected_score_panel"
                    else "next deterministic resolution-irrelevant reserve"
                    if decision == "selected_reserve"
                    else "source-admitted; not required by preregistered panel composition"
                    if decision == "admitted_not_selected"
                    else ", ".join(item.get("blocking_reasons") or ["source defect"])
                ),
                "source_identity": {
                    "archive_key": item["archive_key"],
                    "manifest_sha256": item["manifest_sha256"],
                    "canonical_episode_sha256": item["canonical_episode_sha256"],
                },
            }
        )
    ledger = {
        "schema_version": 2,
        "documentation": {
            "role": "active_outcome_blind_selection_provenance",
            "archived_predecessor": "docs/dev/archive/gensim-global-1hz/2026-08-25/gensim-global-1hz-corpus-rejection-ledger.json",
        },
        "snapshot_sha256": snapshot_sha,
        "decision_count": len(decisions),
        "decision_counts": dict(Counter(item["decision"] for item in decisions)),
        "decisions": decisions,
    }
    dataset = {
        "schema_version": 2,
        "documentation": {
            "role": "active_outcome_blind_dataset_plan",
            "archived_predecessor": "docs/dev/archive/gensim-global-1hz/2026-08-25/gensim-global-1hz-dataset-manifest.json",
        },
        "dataset": {
            "name": "global_1hz_outcome_blind_v1",
            "status": "membership_selected_not_executed",
            "scenario_store_dataset_id": None,
            "membership_hash": manifest["freeze"]["content_sha256"],
        },
        "selection": {
            "source_population": len(candidates),
            "admitted_population": len(admitted),
            "score_panel_count": len(panel),
            "composition": class_counts(panel),
            "selection_uses_observed_outcomes": False,
        },
        "score_policy": manifest["score_policy"],
        "scenarios": [
            {
                "episode_id": item["episode_id"],
                "scenario_name": item["scenario_name"],
                "admission_class": item["admission_class"],
                "capability_status": item["capability_status"],
                "source_pin": pins[item["episode_id"]],
                "execution_status": "not_run",
                "observed_outcome": None,
            }
            for item in panel
        ],
    }

    args.output_dir.mkdir(parents=True, exist_ok=True)
    (args.output_dir / "gensim-global-1hz-classification-baseline.json").write_text(
        json.dumps(baseline_payload, indent=2, sort_keys=True) + "\n"
    )
    (args.output_dir / "gensim-global-1hz-corpus-freeze.json").write_text(
        json.dumps(manifest, indent=2, sort_keys=True) + "\n"
    )
    (args.output_dir / "gensim-global-1hz-corpus-rejection-ledger.json").write_text(
        json.dumps(ledger, indent=2, sort_keys=True) + "\n"
    )
    (args.output_dir / "gensim-global-1hz-dataset-manifest.json").write_text(
        json.dumps(dataset, indent=2, sort_keys=True) + "\n"
    )
    (args.output_dir / "gensim-global-1hz-future-candidate-catalog.md").write_text(
        render_catalog(inventory, candidates, panel_ids)
    )
    (args.output_dir / "gensim-global-1hz-corpus-matrix.md").write_text(render_matrix(manifest))
    return {
        "inventory": inventory,
        "panel_ids": [item["episode_id"] for item in panel],
        "panel_composition": class_counts(panel),
        "reserve_ids": sorted(reserve_ids),
        "corpus_sha256": manifest["freeze"]["content_sha256"],
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--archived-catalog", type=Path, required=True)
    parser.add_argument("--classification-baseline", type=Path)
    parser.add_argument("--bootstrap-classifications", action="store_true")
    parser.add_argument("--output-dir", type=Path, required=True)
    parser.add_argument("--ddtool", default="/opt/homebrew/bin/ddtool")
    parser.add_argument("--datacenter", default="us1.staging.dog")
    parser.add_argument("--gs-admin", default=GS_ADMIN)
    parser.add_argument("--workers", type=int, default=16)
    parser.add_argument("--irrelevant-count", type=int, default=3)
    args = parser.parse_args()
    result = refresh(args)
    print(json.dumps(result, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
