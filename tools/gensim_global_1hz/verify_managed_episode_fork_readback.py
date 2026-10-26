#!/usr/bin/env python3
"""Verify owner-supplied GS Admin readback against a no-write fork plan."""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any


def load(path: Path, label: str) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise ValueError(f"read {label} {path}: {error}") from error
    if not isinstance(value, dict) or value.get("schema_version") != 1:
        raise ValueError(f"{label} schema_version must be 1")
    return value


def verify_readback(plan: dict[str, Any], readback: dict[str, Any]) -> dict[str, Any]:
    planned = {arm.get("arm"): arm for arm in plan.get("arms", [])}
    observed = {arm.get("arm"): arm for arm in readback.get("arms", [])}
    if set(planned) != {"control", "treatment"} or set(observed) != set(planned):
        raise ValueError("readback must contain exactly control and treatment arms")
    blueprint_hash = plan.get("source", {}).get("blueprint", {}).get("sha256")
    episode_ids: list[int] = []
    for arm_name in ("control", "treatment"):
        expected, actual = planned[arm_name], observed[arm_name]
        for key, expected_value in {
            "group": plan.get("output_group"),
            "episode_name": expected.get("episode_name"),
            "blueprint_sha256": blueprint_hash,
            "app_bundle_sha256": expected.get("app_bundle", {}).get("sha256"),
        }.items():
            if actual.get(key) != expected_value:
                raise ValueError(f"{arm_name}: {key} does not match approved plan")
        for version_key in ("blueprint_version", "app_bundle_version"):
            if not isinstance(actual.get(version_key), int) or actual[version_key] <= 0:
                raise ValueError(f"{arm_name}: {version_key} must be a positive integer")
        episode_id = actual.get("episode_id")
        if not isinstance(episode_id, int) or episode_id <= 0:
            raise ValueError(f"{arm_name}: episode_id must be a positive integer")
        episode_ids.append(episode_id)
    if len(set(episode_ids)) != len(episode_ids):
        raise ValueError("control and treatment must have distinct GS Admin episode IDs")
    return {"schema_version": 1, "verified": True, "episode_ids": episode_ids}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--plan", required=True, type=Path)
    parser.add_argument("--readback", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    if args.output.exists():
        raise ValueError(f"refusing to overwrite verification output: {args.output}")
    result = verify_readback(load(args.plan, "plan"), load(args.readback, "readback"))
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(json.dumps(result, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
