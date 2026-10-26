#!/usr/bin/env python3
"""Derive immutable local-GKE A/B GenSim pilot bundles from pinned sources.

This tool preserves the source chart's Agent topology, so its output is for the
local-GKE ``episode-ctl`` pilot. A gs-flow/Terrapin launch needs a separate,
reviewed nested-k3s topology adapter before publication. The tool never contacts
Kubernetes, gs-admin, a registry, or a Datadog API.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import shutil
import tempfile
from pathlib import Path
from typing import Any

SCHEMA_VERSION = 1
AGENT_TEMPLATE_PATH = Path("chart/templates/datadog-agent.yaml")
# The explicit pattern avoids rewriting unrelated application images.
AGENT_IMAGE_PATTERN = re.compile(
    r"(?m)^(?P<name_indent>[ \t]*)- name: datadog-agent[ \t]*\n(?P<image_indent>[ \t]*)image: (?P<image>[^\s#]+)[ \t]*$"
)
KUBERNETES_ENV_PATTERN = re.compile(
    r'(?m)^(?P<indent>[ \t]*)- name: KUBERNETES[ \t]*\n(?P<value_indent>[ \t]*)value: "yes"[ \t]*$'
)


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def tree_sha256(root: Path) -> str:
    """Hash a directory as sorted ``path\0sha256\n`` records."""
    digest = hashlib.sha256()
    for path in sorted(item for item in root.rglob("*") if item.is_file()):
        relative = path.relative_to(root).as_posix().encode("utf-8")
        digest.update(relative)
        digest.update(b"\0")
        digest.update(sha256_bytes(path.read_bytes()).encode("ascii"))
        digest.update(b"\n")
    return digest.hexdigest()


def load_manifest(manifest_path: Path) -> dict[str, Any]:
    try:
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise ValueError(f"read pilot manifest {manifest_path}: {error}") from error
    if manifest.get("schema_version") != SCHEMA_VERSION:
        raise ValueError(f"pilot manifest schema_version must be {SCHEMA_VERSION}")
    if not isinstance(manifest.get("agent_image"), str) or "@sha256:" not in manifest["agent_image"]:
        raise ValueError("pilot manifest agent_image must be a digest-pinned image")
    scenarios = manifest.get("scenarios")
    if not isinstance(scenarios, list) or not scenarios:
        raise ValueError("pilot manifest scenarios must be a non-empty list")
    return manifest


def verify_source(scenario: dict[str, Any]) -> Path:
    source = Path(scenario["source_directory"]).resolve()
    if not source.is_dir():
        raise ValueError(f"episode {scenario.get('episode_id')}: source directory does not exist: {source}")
    episode_file = source / scenario["canonical_episode_file"]
    expected_hash = scenario["canonical_episode_sha256"]
    if not episode_file.is_file():
        raise ValueError(f"episode {scenario.get('episode_id')}: canonical episode file is missing")
    if sha256_bytes(episode_file.read_bytes()) != expected_hash:
        raise ValueError(f"episode {scenario.get('episode_id')}: canonical episode hash does not match manifest")
    if not (source / AGENT_TEMPLATE_PATH).is_file():
        raise ValueError(f"episode {scenario.get('episode_id')}: missing {AGENT_TEMPLATE_PATH}")
    return source


def patch_agent_template(path: Path, image: str, enabled: bool) -> None:
    content = path.read_text(encoding="utf-8")
    matches = list(AGENT_IMAGE_PATTERN.finditer(content))
    if len(matches) != 1:
        raise ValueError(f"{path}: expected exactly one agent image, found {len(matches)}")
    content = content[: matches[0].start("image")] + image + content[matches[0].end("image") :]
    env_matches = list(KUBERNETES_ENV_PATTERN.finditer(content))
    if len(env_matches) != 1:
        raise ValueError(f"{path}: expected exactly one KUBERNETES=yes environment entry, found {len(env_matches)}")
    match = env_matches[0]
    value = "true" if enabled else "false"
    insertion = f'{match.group("indent")}- name: DD_METRIC_RESOLUTION_EXPERIMENT_ENABLED\n{match.group("value_indent")}value: "{value}"\n'
    content = content[: match.start()] + insertion + content[match.start() :]
    path.write_text(content, encoding="utf-8")


def compare_arms(control: Path, treatment: Path) -> list[str]:
    control_files = {path.relative_to(control).as_posix(): path for path in control.rglob("*") if path.is_file()}
    treatment_files = {path.relative_to(treatment).as_posix(): path for path in treatment.rglob("*") if path.is_file()}
    if control_files.keys() != treatment_files.keys():
        raise ValueError("control and treatment bundle file sets differ")
    changed = sorted(name for name in control_files if control_files[name].read_bytes() != treatment_files[name].read_bytes())
    if changed != [AGENT_TEMPLATE_PATH.as_posix()]:
        raise ValueError(f"control and treatment differ outside the Agent template: {changed}")
    return changed


def scenario_result(scenario: dict[str, Any], source: Path, output_root: Path, image: str) -> dict[str, Any]:
    episode_id = scenario["episode_id"]
    base = output_root / f"episode-{episode_id}-v{scenario['app_bundle_version']}"
    control = base / "A"
    treatment = base / "B"
    shutil.copytree(source, control)
    shutil.copytree(source, treatment)
    patch_agent_template(control / AGENT_TEMPLATE_PATH, image, enabled=False)
    patch_agent_template(treatment / AGENT_TEMPLATE_PATH, image, enabled=True)
    changed_paths = compare_arms(control, treatment)
    return {
        "episode_id": episode_id,
        "role": scenario["role"],
        "source_directory": str(source),
        "source_tree_sha256": tree_sha256(source),
        "canonical_episode_file": scenario["canonical_episode_file"],
        "canonical_episode_sha256": scenario["canonical_episode_sha256"],
        "allowed_arm_diff_paths": changed_paths,
        "arms": {
            "A": {"directory": str(control.relative_to(output_root)), "tree_sha256": tree_sha256(control), "resolution_experiment_enabled": False},
            "B": {"directory": str(treatment.relative_to(output_root)), "tree_sha256": tree_sha256(treatment), "resolution_experiment_enabled": True},
        },
    }


def build_bundles(manifest_path: Path, output_root: Path) -> dict[str, Any]:
    """Build all requested A/B bundles atomically and return their manifest."""
    manifest_path = manifest_path.resolve()
    output_root = output_root.resolve()
    if output_root.exists():
        raise ValueError(f"refusing to overwrite existing output directory: {output_root}")
    manifest = load_manifest(manifest_path)
    seen: set[int] = set()
    scenarios: list[tuple[dict[str, Any], Path]] = []
    for scenario in manifest["scenarios"]:
        episode_id = scenario.get("episode_id")
        if not isinstance(episode_id, int) or episode_id <= 0 or episode_id in seen:
            raise ValueError("pilot manifest episode_id values must be unique positive integers")
        seen.add(episode_id)
        for field in ("app_bundle_version", "role", "source_directory", "canonical_episode_file", "canonical_episode_sha256"):
            if not scenario.get(field):
                raise ValueError(f"episode {episode_id}: missing {field}")
        scenarios.append((scenario, verify_source(scenario)))

    output_root.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=f".{output_root.name}.", dir=output_root.parent) as temporary:
        staging = Path(temporary) / output_root.name
        staging.mkdir()
        results = [scenario_result(scenario, source, staging, manifest["agent_image"]) for scenario, source in sorted(scenarios, key=lambda item: item[0]["episode_id"])]
        result = {
            "schema_version": SCHEMA_VERSION,
            "pilot_manifest": str(manifest_path),
            "pilot_manifest_sha256": sha256_bytes(manifest_path.read_bytes()),
            "agent_image": manifest["agent_image"],
            "scenarios": results,
        }
        (staging / "pilot-bundle-manifest.json").write_text(json.dumps(result, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        os.replace(staging, output_root)
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", required=True, type=Path, help="pilot scenario manifest")
    parser.add_argument("--output", required=True, type=Path, help="new output directory; must not exist")
    args = parser.parse_args()
    result = build_bundles(args.manifest, args.output)
    print(json.dumps({"output": str(args.output.resolve()), "scenarios": len(result["scenarios"])}, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
