#!/usr/bin/env python3
"""Audit source-only GenSim readiness without launching an episode.

The audit consumes frozen local metadata and exact local source archives. It
never contacts Kubernetes, Datadog, GS Admin, a registry, or a model. Missing
evidence remains unknown rather than being inferred from another bundle.
"""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import tarfile
from pathlib import Path, PurePosixPath
from typing import Any, Iterable

import yaml

MAX_ARCHIVE_BYTES = 128 * 1024 * 1024
MAX_EXPANDED_BYTES = 128 * 1024 * 1024
MAX_MEMBERS = 10_000
HEX_FIELDS = (
    "manifest_sha256",
    "canonical_episode_sha256",
)


def sha256_bytes(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def rca_chain_status(scenario: Any) -> str:
    if not isinstance(scenario, dict):
        return "invalid_scenario"
    chain = scenario.get("rca_causal_chain")
    if not isinstance(chain, dict):
        return "missing"
    required = {"immediate_cause", "findings_chain", "deeper_findings_total"}
    allowed = required | {"contributing_factors"}
    if not required <= set(chain) or set(chain) - allowed:
        return "invalid_fields"
    cause = chain["immediate_cause"]
    findings = chain["findings_chain"]
    factors = chain.get("contributing_factors", [])
    total = chain["deeper_findings_total"]
    if not isinstance(cause, str) or not cause.strip():
        return "invalid_immediate_cause"
    if (
        not isinstance(findings, list)
        or not findings
        or any(not isinstance(item, str) or not item.strip() for item in findings)
    ):
        return "invalid_findings_chain"
    if not isinstance(factors, list) or any(
        not isinstance(item, str) or not item.strip() for item in factors
    ):
        return "invalid_contributing_factors"
    if isinstance(total, bool) or not isinstance(total, int):
        return "invalid_deeper_findings_total"
    if total != len(findings) - 1 + len(factors):
        return "invalid_count"
    return "valid"


def read_safe_tar(raw: bytes) -> dict[str, bytes]:
    if len(raw) > MAX_ARCHIVE_BYTES:
        raise ValueError("archive exceeds size limit")
    files: dict[str, bytes] = {}
    total = 0
    try:
        with tarfile.open(fileobj=io.BytesIO(raw), mode="r:*") as archive:
            for index, member in enumerate(archive):
                if index >= MAX_MEMBERS:
                    raise ValueError("archive has too many members")
                path = PurePosixPath(member.name)
                if not member.name or path.is_absolute() or ".." in path.parts:
                    raise ValueError("archive has unsafe member")
                name = path.as_posix()
                if name in files:
                    raise ValueError("archive has duplicate member")
                if not (member.isfile() or member.isdir()):
                    raise ValueError("archive has unsupported member type")
                if not member.isfile():
                    continue
                total += member.size
                if total > MAX_EXPANDED_BYTES:
                    raise ValueError("archive expands beyond size limit")
                source = archive.extractfile(member)
                if source is None:
                    raise ValueError("archive file cannot be read")
                files[name] = source.read()
    except tarfile.TarError as error:
        raise ValueError(f"invalid tar archive: {error}") from error
    return files


def agent_adapter_status(files: dict[str, bytes]) -> str:
    try:
        values = files["chart/values.yaml"].decode("utf-8")
        template = files["chart/templates/datadog-agent.yaml"].decode("utf-8")
    except KeyError:
        return "missing_agent_inputs"
    except UnicodeDecodeError:
        return "invalid_agent_input_encoding"
    if values.count('  env: ""\n') == 1:
        env_status = "compatible"
    elif values.count("  env: ''\n") == 1:
        env_status = "requires_empty_string_adapter_extension"
    else:
        return "unsupported_datadog_env_shape"
    required = (
        "            - name: DD_ENV\n"
        "              value: {{ .Values.datadog.env | quote }}\n",
        "          volumeMounts:\n",
        "      volumes:\n",
        "---\napiVersion: v1\nkind: Service\nmetadata:\n  name: datadog-agent",
    )
    if template.count("gcr.io/datadoghq/agent:7") != 1:
        return "unsupported_agent_image_shape"
    if any(template.count(fragment) != 1 for fragment in required):
        return "unsupported_agent_template_shape"
    return env_status


def index_judge_fixtures(judge: dict[str, Any]) -> dict[int, dict[str, Any]]:
    bindings = judge.get("source_identity_bindings") or {}
    indexed: dict[int, dict[str, Any]] = {}
    for item in judge["cases"]:
        episode_id = item["episode_id"]
        value = dict(item)
        binding = bindings.get(str(episode_id))
        if isinstance(binding, dict):
            value["source_identity"] = binding
        indexed[episode_id] = value
    return indexed


def judge_fixture_status(
    fixture: dict[str, Any] | None, identity: dict[str, Any]
) -> str:
    if fixture is None:
        return "missing"
    bound = fixture.get("source_identity")
    if not isinstance(bound, dict):
        return "present_unbound"
    expected = {field: identity.get(field) for field in ("app_bundle_version", "archive_key", *HEX_FIELDS)}
    actual = {field: bound.get(field) for field in expected}
    return "bound_match" if actual == expected else "bound_mismatch"


def _load_yaml(files: dict[str, bytes], name: str) -> Any:
    raw = files.get(name)
    if raw is None:
        return None
    try:
        return yaml.safe_load(raw)
    except yaml.YAMLError:
        return None


def audit_candidate(
    *,
    candidate: dict[str, Any],
    identity: dict[str, Any],
    judge_fixture: dict[str, Any] | None,
    archive_path: Path | None,
    expected_archive_sha256: str | None,
    canonical_episode_path: str | None,
) -> dict[str, Any]:
    result: dict[str, Any] = {
        "episode_id": candidate["episode_id"],
        "admission_class": candidate["admission_class"],
        "app_bundle_version": identity["app_bundle_version"],
        "archive_key": identity["archive_key"],
        "archive_path": str(archive_path) if archive_path else None,
        "archive_status": "missing",
        "canonical_episode_status": "unknown",
        "authoritative_ground_truth_status": "unknown",
        "judge_fixture_status": judge_fixture_status(judge_fixture, identity),
        "archive_rca_chain_status": "unknown",
        "agent_adapter_status": "unknown",
        "required_bundle_inputs_status": "unknown",
        "static_execution_ready": False,
    }
    if archive_path is None:
        return result
    try:
        raw = archive_path.read_bytes()
        files = read_safe_tar(raw)
    except (OSError, ValueError) as error:
        result["archive_status"] = f"invalid:{type(error).__name__}:{error}"
        return result
    actual_archive_sha = sha256_bytes(raw)
    result["archive_sha256"] = actual_archive_sha
    if expected_archive_sha256 and actual_archive_sha != expected_archive_sha256:
        result["archive_status"] = "sha256_mismatch"
        return result
    result["archive_status"] = "verified" if expected_archive_sha256 else "locally_hashed_unpinned"
    episode_paths = sorted(
        name for name in files if name.startswith("episodes/") and name.endswith((".yaml", ".yml"))
    )
    episode_path = canonical_episode_path or (episode_paths[0] if len(episode_paths) == 1 else None)
    required = ["play-episode.sh", "chart/values.yaml", "chart/templates/datadog-agent.yaml"]
    if episode_path:
        required.append(episode_path)
    result["required_bundle_inputs_status"] = (
        "present" if episode_path and all(name in files for name in required) else "missing"
    )
    episode_raw = files.get(episode_path or "")
    if episode_raw is not None and sha256_bytes(episode_raw) == identity["canonical_episode_sha256"]:
        result["canonical_episode_status"] = "verified"
    elif episode_raw is not None:
        result["canonical_episode_status"] = "sha256_mismatch"
    else:
        result["canonical_episode_status"] = "missing"
    episode = _load_yaml(files, episode_path or "")
    if isinstance(episode, dict):
        description = str(episode.get("description") or "").strip()
        long_description = str(episode.get("long_description") or "").strip()
        result["description_bytes"] = len(description.encode())
        result["long_description_bytes"] = len(long_description.encode())
        result["authoritative_ground_truth_status"] = (
            "source_detailed" if description and long_description else "source_summary_only"
        )
    else:
        result["authoritative_ground_truth_status"] = "invalid_episode_yaml"
    scenario = _load_yaml(files, "scenario.yaml")
    result["archive_rca_chain_status"] = rca_chain_status(scenario)
    result["agent_adapter_status"] = agent_adapter_status(files)
    result["static_execution_ready"] = all(
        (
            result["archive_status"] == "verified",
            result["canonical_episode_status"] == "verified",
            result["authoritative_ground_truth_status"] == "source_detailed",
            result["required_bundle_inputs_status"] == "present",
            result["agent_adapter_status"] == "compatible",
        )
    )
    return result


def _load_object(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text())
    if not isinstance(value, dict):
        raise ValueError(f"{path} must contain an object")
    return value


def _resolve_archive(directories: Iterable[Path], episode_id: int, version: int) -> Path | None:
    name = f"episode-{episode_id}-v{version}.tar"
    matches = [directory / name for directory in directories if (directory / name).is_file()]
    if len(matches) > 1:
        digests = {sha256_bytes(path.read_bytes()) for path in matches}
        if len(digests) != 1:
            raise ValueError(f"conflicting local archives for episode {episode_id} v{version}")
    return matches[0] if matches else None


def build_report(
    freeze: dict[str, Any],
    baseline: dict[str, Any],
    judge: dict[str, Any],
    archive_directories: list[Path],
) -> dict[str, Any]:
    identities = {item["episode_id"]: item for item in baseline["records"]}
    fixtures = index_judge_fixtures(judge)
    panel = {item["episode_id"]: item for item in freeze["score_panel"]}
    candidates: list[dict[str, Any]] = list(freeze["score_panel"])
    candidates.extend(
        {
            "episode_id": item["episode_id"],
            "scenario_name": item["scenario_name"],
            "admission_class": item["admission_class"],
            "selection_role": "reserve",
        }
        for item in freeze.get("reserves", [])
    )
    selected = {item["episode_id"] for item in candidates}
    candidates.extend(
        {
            "episode_id": episode_id,
            "scenario_name": identities[episode_id].get("scenario_name"),
            "admission_class": identities[episode_id]["admission_class"],
            "selection_role": "admitted_fixture_alternative",
        }
        for episode_id in sorted(fixtures)
        if episode_id not in selected
        and episode_id in identities
        and identities[episode_id]["admission_status"] == "admitted"
    )
    results = []
    for candidate in candidates:
        episode_id = candidate["episode_id"]
        identity = identities[episode_id]["source_identity"]
        panel_item = panel.get(episode_id)
        source_pin = panel_item.get("source_pin", {}) if panel_item else {}
        results.append(
            audit_candidate(
                candidate=candidate,
                identity=identity,
                judge_fixture=fixtures.get(episode_id),
                archive_path=_resolve_archive(
                    archive_directories, episode_id, identity["app_bundle_version"]
                ),
                expected_archive_sha256=source_pin.get("archive_sha256"),
                canonical_episode_path=source_pin.get("canonical_episode_path"),
            )
        )
    return {
        "schema_version": 1,
        "role": "source_only_no_run_readiness_audit",
        "does_not_use_treatment_outcomes": True,
        "records": results,
        "summary": {
            "record_count": len(results),
            "static_execution_ready_count": sum(
                item["static_execution_ready"] for item in results
            ),
            "missing_archive_episode_ids": [
                item["episode_id"] for item in results if item["archive_status"] == "missing"
            ],
            "missing_judge_fixture_episode_ids": [
                item["episode_id"] for item in results if item["judge_fixture_status"] == "missing"
            ],
            "unbound_judge_fixture_episode_ids": [
                item["episode_id"]
                for item in results
                if item["judge_fixture_status"] == "present_unbound"
            ],
        },
    }


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--freeze", type=Path, required=True)
    parser.add_argument("--classification-baseline", type=Path, required=True)
    parser.add_argument("--judge-cases", type=Path, required=True)
    parser.add_argument("--archive-dir", type=Path, action="append", required=True)
    parser.add_argument("--output", type=Path)
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    report = build_report(
        _load_object(args.freeze),
        _load_object(args.classification_baseline),
        _load_object(args.judge_cases),
        [path.expanduser().resolve() for path in args.archive_dir],
    )
    rendered = json.dumps(report, indent=2, sort_keys=True) + "\n"
    if args.output:
        output = args.output.expanduser().resolve()
        if output.exists():
            raise FileExistsError(f"refusing to overwrite {output}")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(rendered)
    else:
        print(rendered, end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
