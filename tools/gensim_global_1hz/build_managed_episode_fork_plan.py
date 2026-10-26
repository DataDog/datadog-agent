#!/usr/bin/env python3
"""Build a no-write publication plan for a generated GenSim episode fork.

The tool only reads immutable local artifacts and writes a JSON plan. It never
constructs a gs-admin client, sends network traffic, or performs an external
write. The owner uses the plan to approve and execute append-only GS Admin
operations separately.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path
from typing import Any

SCHEMA_VERSION = 1
SAFE_NAME = re.compile(r"^[a-z0-9][a-z0-9-]{0,62}$")


def sha256_file(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load_json(path: Path, label: str) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise ValueError(f"read {label} {path}: {error}") from error
    if not isinstance(value, dict) or value.get("schema_version") != SCHEMA_VERSION:
        raise ValueError(f"{label} schema_version must be {SCHEMA_VERSION}")
    return value


def source_entry(pilot: dict[str, Any], source: dict[str, Any], episode_id: int) -> dict[str, Any]:
    pilot_entries = [entry for entry in pilot.get("scenarios", []) if entry.get("episode_id") == episode_id]
    source_entries = [entry for entry in source.get("selection", []) if entry.get("episode_id") == episode_id]
    if len(pilot_entries) != 1 or len(source_entries) != 1:
        raise ValueError(f"episode {episode_id}: expected exactly one pilot and source manifest entry")
    pilot_entry, source_entry = pilot_entries[0], source_entries[0]
    source_tar_sha256 = source_entry.get("tar_sha256")
    if pilot_entry.get("source_tar_sha256") != source_tar_sha256:
        raise ValueError(f"episode {episode_id}: source tar hash differs between manifests")
    source_dir = Path(str(pilot_entry.get("source_directory", ""))).resolve()
    canonical = source_dir / str(pilot_entry.get("canonical_episode_file", ""))
    if not canonical.is_file():
        raise ValueError(f"episode {episode_id}: canonical episode file is missing")
    canonical_hash = sha256_file(canonical)
    if canonical_hash != pilot_entry.get("canonical_episode_sha256"):
        raise ValueError(f"episode {episode_id}: canonical episode hash does not match pilot manifest")
    source_canonical = source_entry.get("canonical_episode_file")
    if not isinstance(source_canonical, dict) or source_canonical.get("sha256") != canonical_hash:
        raise ValueError(f"episode {episode_id}: canonical episode hash differs between manifests")
    return {
        "pilot": pilot_entry,
        "source": source_entry,
        "source_tar_sha256": source_tar_sha256,
        "canonical_hash": canonical_hash,
    }


def checked_artifact(path: Path, label: str) -> dict[str, str]:
    path = path.resolve()
    if not path.is_file() or path.stat().st_size == 0:
        raise ValueError(f"{label} must be a non-empty regular file: {path}")
    return {"path": str(path), "sha256": "sha256:" + sha256_file(path), "bytes": str(path.stat().st_size)}


def checked_name(value: str, label: str) -> str:
    if not SAFE_NAME.fullmatch(value):
        raise ValueError(f"{label} must be a DNS-safe name of at most 63 characters")
    return value


def arm(arm_name: str, episode_name: str, bundle: dict[str, str], source: dict[str, Any]) -> dict[str, Any]:
    return {
        "arm": arm_name,
        "episode_name": episode_name,
        "origin_tags": {
            "global-1hz.arm": arm_name,
            "global-1hz.origin_episode_id": str(source["episode_id"]),
            "global-1hz.origin_app_bundle_version": str(source["app_bundle_version"]),
            "global-1hz.origin_source_tar_sha256": source["source_tar_sha256"],
            "global-1hz.origin_canonical_episode_sha256": source["canonical_episode_sha256"],
        },
        "app_bundle": bundle,
    }


def build_plan(
    pilot_manifest_path: Path,
    source_manifest_path: Path,
    episode_id: int,
    output_group: str,
    control_name: str,
    treatment_name: str,
    blueprint_path: Path,
    control_bundle_path: Path,
    treatment_bundle_path: Path,
) -> dict[str, Any]:
    """Return an append-only GS Admin fork plan without doing external I/O."""
    if episode_id <= 0:
        raise ValueError("episode_id must be positive")
    output_group = checked_name(output_group, "output_group")
    control_name = checked_name(control_name, "control_name")
    treatment_name = checked_name(treatment_name, "treatment_name")
    if control_name == treatment_name:
        raise ValueError("control_name and treatment_name must be distinct")

    pilot_path = pilot_manifest_path.resolve()
    source_path = source_manifest_path.resolve()
    pilot = load_json(pilot_path, "pilot manifest")
    source_manifest = load_json(source_path, "source manifest")
    selected = source_entry(pilot, source_manifest, episode_id)
    pilot_source, source = selected["pilot"], selected["source"]
    source_summary = {
        "episode_id": episode_id,
        "episode_group": source.get("episode_group"),
        "episode_name": source.get("episode_name"),
        "app_bundle_version": source.get("app_bundle_version"),
        "source_tar_sha256": selected["source_tar_sha256"],
        "canonical_episode_file": pilot_source.get("canonical_episode_file"),
        "canonical_episode_sha256": selected["canonical_hash"],
        "blueprint": checked_artifact(blueprint_path, "blueprint artifact"),
    }
    control = arm("control", control_name, checked_artifact(control_bundle_path, "control bundle artifact"), source_summary)
    treatment = arm("treatment", treatment_name, checked_artifact(treatment_bundle_path, "treatment bundle artifact"), source_summary)
    operations: list[dict[str, str | bool]] = []
    for item in (control, treatment):
        name = str(item["episode_name"])
        for operation in ("PutEpisode", "PutBlueprint", "PutAppBundle"):
            operations.append({
                "operation": operation,
                "arm": str(item["arm"]),
                "group": output_group,
                "episode_name": name,
                "external_write": True,
                "requires_owner_approval": True,
            })
    return {
        "schema_version": SCHEMA_VERSION,
        "mode": "no_write_plan",
        "pilot_manifest": {"path": str(pilot_path), "sha256": "sha256:" + sha256_file(pilot_path)},
        "source_manifest": {"path": str(source_path), "sha256": "sha256:" + sha256_file(source_path)},
        "output_group": output_group,
        "source": source_summary,
        "arms": [control, treatment],
        "planned_operations": operations,
        "required_readback": [
            "derived episode ID and exact group/name",
            "derived blueprint version and SHA-256",
            "derived app-bundle version and SHA-256",
            "origin and arm tags",
            "control/treatment identity distinctness",
        ],
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--pilot-manifest", required=True, type=Path)
    parser.add_argument("--source-manifest", required=True, type=Path)
    parser.add_argument("--episode-id", required=True, type=int)
    parser.add_argument("--output-group", required=True)
    parser.add_argument("--control-name", required=True)
    parser.add_argument("--treatment-name", required=True)
    parser.add_argument("--blueprint", required=True, type=Path)
    parser.add_argument("--control-bundle", required=True, type=Path)
    parser.add_argument("--treatment-bundle", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path, help="new local JSON plan; must not exist")
    args = parser.parse_args()
    if args.output.exists():
        raise ValueError(f"refusing to overwrite output plan: {args.output}")
    plan = build_plan(
        args.pilot_manifest, args.source_manifest, args.episode_id, args.output_group,
        args.control_name, args.treatment_name, args.blueprint, args.control_bundle, args.treatment_bundle,
    )
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(plan, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(json.dumps({"output": str(args.output.resolve()), "mode": plan["mode"]}, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
