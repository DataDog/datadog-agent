#!/usr/bin/env python3
"""Owner-run, append-only GS Admin publisher for a managed episode fork plan.

This tool defaults to a local-only dry run.  Remote writes occur only with
``--apply`` and are intentionally performed by the authenticated owner.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import urllib.request
from pathlib import Path
from typing import Any, Callable

SCHEMA_VERSION = 1


def sha256_bytes(data: bytes) -> str:
    return "sha256:" + hashlib.sha256(data).hexdigest()


def load_plan(path: Path) -> dict[str, Any]:
    try:
        plan = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise ValueError(f"read fork plan {path}: {error}") from error
    if not isinstance(plan, dict) or plan.get("schema_version") != SCHEMA_VERSION:
        raise ValueError("fork plan schema_version must be 1")
    if plan.get("mode") != "no_write_plan":
        raise ValueError("fork plan mode must be no_write_plan")
    if not isinstance(plan.get("output_group"), str) or not plan["output_group"]:
        raise ValueError("fork plan must provide output_group")
    arms = plan.get("arms")
    if not isinstance(arms, list) or {arm.get("arm") for arm in arms if isinstance(arm, dict)} != {"control", "treatment"}:
        raise ValueError("fork plan must contain exactly control and treatment arms")
    source = plan.get("source")
    if not isinstance(source, dict) or not isinstance(source.get("episode_id"), int) or source["episode_id"] <= 0:
        raise ValueError("fork plan source episode_id must be positive")
    checked_artifact(source.get("blueprint"), "source blueprint")
    for arm in arms:
        if not isinstance(arm, dict) or not isinstance(arm.get("episode_name"), str) or not arm["episode_name"]:
            raise ValueError("each arm must provide a non-empty episode_name")
        checked_artifact(arm.get("app_bundle"), f"{arm.get('arm')} app bundle")
    return plan


def checked_artifact(value: Any, label: str) -> dict[str, str]:
    if not isinstance(value, dict):
        raise ValueError(f"{label} artifact is missing")
    path = Path(str(value.get("path", ""))).resolve()
    if not path.is_file() or path.stat().st_size == 0:
        raise ValueError(f"{label} artifact must be a non-empty regular file: {path}")
    expected = value.get("sha256")
    actual = sha256_bytes(path.read_bytes())
    if expected != actual:
        raise ValueError(f"{label} SHA-256 does not match the approved plan")
    return {"path": str(path), "sha256": actual, "bytes": str(path.stat().st_size)}


def ordered_arms(plan: dict[str, Any]) -> list[dict[str, Any]]:
    by_name = {arm["arm"]: arm for arm in plan["arms"]}
    return [by_name["control"], by_name["treatment"]]


def dry_run_summary(plan: dict[str, Any]) -> dict[str, Any]:
    """Validate every local artifact without importing the GS Admin client."""
    blueprint = checked_artifact(plan["source"]["blueprint"], "source blueprint")
    arms = ordered_arms(plan)
    return {
        "schema_version": SCHEMA_VERSION,
        "mode": "dry_run",
        "output_group": plan["output_group"],
        "source_episode_id": plan["source"]["episode_id"],
        "blueprint_sha256": blueprint["sha256"],
        "arms": [arm["arm"] for arm in arms],
    }


def verify_destination_absent(plan: dict[str, Any], client: Any) -> None:
    """Reject existing identities before any Put operation is attempted."""
    group = plan["output_group"]
    source_id = plan["source"]["episode_id"]
    for arm in ordered_arms(plan):
        existing = client.find_episode_by_exact_name(group, arm["episode_name"])
        if existing is not None:
            raise ValueError(
                f"destination {group}/{arm['episode_name']} already exists as episode {existing.id}; "
                "refusing append-only publication"
            )
        if arm["episode_name"] == str(source_id):
            raise ValueError("destination episode name must not identify the source episode")


def fetch_app_bundle_from_presigned_url(url: str) -> bytes:
    with urllib.request.urlopen(url, timeout=90) as response:  # nosec B310: GS Admin supplied presigned URL
        if not 200 <= response.status < 300:
            raise ValueError(f"download published app-bundle archive: HTTP {response.status}")
        return response.read()


def published_arm_readback(
    plan: dict[str, Any], arm: dict[str, Any], client: Any, fetch_app_bundle: Callable[[str], bytes]
) -> dict[str, Any]:
    """Publish one new arm and prove the remote archives match local plan hashes."""
    group = plan["output_group"]
    episode_tags = list(arm["origin_tags"].items())
    tags = [f"{key}={value}" for key, value in episode_tags]
    episode = client.put_episode(group, arm["episode_name"], tags=tags)
    if episode.id == plan["source"]["episode_id"]:
        raise ValueError("GS Admin returned the immutable source episode as a destination")
    if client.latest_blueprint_version(episode.id) is not None or client.latest_app_bundle_version(episode.id) is not None:
        raise ValueError(f"destination episode {episode.id} already contains published artifacts")

    blueprint = checked_artifact(plan["source"]["blueprint"], "source blueprint")
    blueprint_ref = client.put_blueprint(episode.id, Path(blueprint["path"]).read_bytes(), tags=tags)
    remote_blueprint_hash = sha256_bytes(client.download_blueprint_archive(episode.id, blueprint_ref.version))
    if remote_blueprint_hash != blueprint["sha256"]:
        raise ValueError(f"{arm['arm']}: published blueprint archive hash does not match the approved plan")

    bundle = checked_artifact(arm["app_bundle"], f"{arm['arm']} app bundle")
    bundle_ref = client.put_app_bundle(
        episode.id, blueprint_ref.version, Path(bundle["path"]).read_bytes(), tags=tags
    )
    remote_bundle_hash = sha256_bytes(fetch_app_bundle(client.app_bundle_presigned_url(episode.id, bundle_ref.version)))
    if remote_bundle_hash != bundle["sha256"]:
        raise ValueError(f"{arm['arm']}: published app-bundle archive hash does not match the approved plan")

    return {
        "arm": arm["arm"],
        "episode_id": episode.id,
        "group": group,
        "episode_name": arm["episode_name"],
        "blueprint_version": blueprint_ref.version,
        "blueprint_sha256": remote_blueprint_hash,
        "app_bundle_version": bundle_ref.version,
        "app_bundle_sha256": remote_bundle_hash,
        "origin_tags": tags,
    }


def publish(plan: dict[str, Any], client: Any, fetch_app_bundle: Callable[[str], bytes]) -> dict[str, Any]:
    """Perform the owner-approved append-only publication and return verifier input."""
    dry_run_summary(plan)
    verify_destination_absent(plan, client)
    readback = [published_arm_readback(plan, arm, client, fetch_app_bundle) for arm in ordered_arms(plan)]
    if len({arm["episode_id"] for arm in readback}) != 2:
        raise ValueError("GS Admin returned a shared destination episode identity")
    return {"schema_version": SCHEMA_VERSION, "arms": readback}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--plan", required=True, type=Path)
    parser.add_argument("--readback", type=Path, help="new file written only after a successful --apply")
    parser.add_argument("--base-url", default="https://gs-admin.us1.staging.dog")
    parser.add_argument("--apply", action="store_true", help="perform remote GS Admin writes as the authenticated owner")
    args = parser.parse_args()

    plan = load_plan(args.plan)
    if not args.apply:
        print(json.dumps(dry_run_summary(plan), sort_keys=True))
        return 0
    if args.readback is None:
        raise ValueError("--readback is required with --apply")
    if args.readback.exists():
        raise ValueError(f"refusing to overwrite readback: {args.readback}")
    try:
        from gsadmin import Client
    except ImportError as error:
        raise RuntimeError("gsadmin package is required; run this under the approved GS Admin Python environment") from error
    client = Client(base_url=args.base_url, timeout=90)
    result = publish(plan, client, fetch_app_bundle_from_presigned_url)
    args.readback.parent.mkdir(parents=True, exist_ok=True)
    args.readback.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(json.dumps({"mode": "published", "readback": str(args.readback), "episode_ids": [arm["episode_id"] for arm in result["arms"]]}, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
