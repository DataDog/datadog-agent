#!/usr/bin/env python3
"""Derive deterministic, no-write first managed-smoke A/B artifacts.

This intentionally preserves the frozen source topology. It is limited to the
first episode-2483 DaemonSet smoke and is not a general nested-k3s adapter or a
GS Admin publisher.
"""

from __future__ import annotations

import argparse
import hashlib
import importlib.util
import json
import os
import shutil
import tarfile
import tempfile
from pathlib import Path
from typing import Any

LOCAL_BUILDER = Path(__file__).with_name("build_pilot_bundles.py")
SPEC = importlib.util.spec_from_file_location("build_pilot_bundles", LOCAL_BUILDER)
assert SPEC and SPEC.loader
local_builder = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(local_builder)

AGENT_TEMPLATE_PATH = local_builder.AGENT_TEMPLATE_PATH


def tar_sha256(root: Path, output: Path) -> str:
    """Write deterministic uncompressed tar from a directory and return its hash."""
    with tarfile.open(output, "w", format=tarfile.USTAR_FORMAT) as archive:
        for path in sorted(item for item in root.rglob("*") if item.is_file()):
            info = archive.gettarinfo(str(path), arcname=path.relative_to(root).as_posix())
            info.uid = info.gid = 0
            info.uname = info.gname = ""
            info.mtime = 0
            with path.open("rb") as source:
                archive.addfile(info, source)
    return hashlib.sha256(output.read_bytes()).hexdigest()


def ensure_first_smoke_topology(agent_template: Path) -> None:
    content = agent_template.read_text(encoding="utf-8")
    if "kind: DaemonSet" not in content:
        raise ValueError(f"{agent_template}: first managed smoke requires the frozen DaemonSet topology")
    for required in ("DD_CRI_SOCKET_PATH", "/var/log/pods"):
        if required not in content:
            raise ValueError(f"{agent_template}: missing frozen managed-smoke prerequisite {required}")


def build_smoke_bundles(manifest_path: Path, episode_id: int, output_root: Path) -> dict[str, Any]:
    manifest_path, output_root = manifest_path.resolve(), output_root.resolve()
    if output_root.exists():
        raise ValueError(f"refusing to overwrite existing output directory: {output_root}")
    manifest = local_builder.load_manifest(manifest_path)
    scenarios = [scenario for scenario in manifest["scenarios"] if scenario.get("episode_id") == episode_id]
    if len(scenarios) != 1:
        raise ValueError(f"expected exactly one episode {episode_id} in pilot manifest")
    scenario = scenarios[0]
    source = local_builder.verify_source(scenario)
    ensure_first_smoke_topology(source / AGENT_TEMPLATE_PATH)

    output_root.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=f".{output_root.name}.", dir=output_root.parent) as temporary:
        staging = Path(temporary) / output_root.name
        control, treatment = staging / "control", staging / "treatment"
        shutil.copytree(source, control)
        shutil.copytree(source, treatment)
        local_builder.patch_agent_template(control / AGENT_TEMPLATE_PATH, manifest["agent_image"], enabled=False)
        local_builder.patch_agent_template(treatment / AGENT_TEMPLATE_PATH, manifest["agent_image"], enabled=True)
        changed = local_builder.compare_arms(control, treatment)
        control_tar, treatment_tar = staging / "control.tar", staging / "treatment.tar"
        result = {
            "schema_version": 1,
            "mode": "prepublication_managed_smoke_artifacts",
            "episode_id": episode_id,
            "source_directory": str(source),
            "source_tar_sha256": scenario["source_tar_sha256"],
            "canonical_episode_file": scenario["canonical_episode_file"],
            "canonical_episode_sha256": scenario["canonical_episode_sha256"],
            "agent_image": manifest["agent_image"],
            "allowed_arm_diff_paths": changed,
            "arms": {
                "control": {"directory": "control", "tar_path": "control.tar", "tree_sha256": local_builder.tree_sha256(control), "tar_sha256": "sha256:" + tar_sha256(control, control_tar), "resolution_experiment_enabled": False},
                "treatment": {"directory": "treatment", "tar_path": "treatment.tar", "tree_sha256": local_builder.tree_sha256(treatment), "tar_sha256": "sha256:" + tar_sha256(treatment, treatment_tar), "resolution_experiment_enabled": True},
            },
            "publication_prohibited": True,
        }
        (staging / "managed-smoke-manifest.json").write_text(json.dumps(result, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        os.replace(staging, output_root)
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", required=True, type=Path)
    parser.add_argument("--episode-id", required=True, type=int)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    result = build_smoke_bundles(args.manifest, args.episode_id, args.output)
    print(json.dumps({"output": str(args.output.resolve()), "publication_prohibited": result["publication_prohibited"]}, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
