"""Compute the final-flush wait and validate/trim a fetched gensim recording."""

import argparse
import datetime as dt
import hashlib
import json
import math
import re
import shutil
import tarfile
import zipfile
from pathlib import Path, PurePosixPath


def wait_seconds(episode_file, flush):
    episode = json.loads(episode_file.read_text())
    end = dt.datetime.fromisoformat(episode["end_time"].replace("Z", "+00:00"))
    if end.tzinfo is None or end > dt.datetime.now(dt.timezone.utc):
        raise ValueError("episode end_time must be in the past and timezone-aware")
    return max(0, math.ceil(end.timestamp() + flush - dt.datetime.now(dt.timezone.utc).timestamp()))


def sha256_file(path):
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(65536), b""):
            digest.update(chunk)
    return digest.hexdigest()


def parse_duration_seconds(raw):
    match = re.fullmatch(r"([1-9][0-9]*)([smh])", raw.strip())
    if not match:
        raise ValueError(f"unsupported duration: {raw}")
    amount = int(match.group(1))
    unit = match.group(2)
    return amount * {"s": 1, "m": 60, "h": 3600}[unit]


def timestamp(value):
    result = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    if result.tzinfo is None:
        raise ValueError("episode timestamps must include a timezone")
    return result.timestamp()


def integer_divisor(value):
    for divisor in (1, 1000, 1000000, 1000000000):
        if 1e9 < value / divisor < 4e9:
            return divisor
    raise ValueError(f"unexpected Time value {value}")


def parse_pod_snapshot(output, filename, expected_single):
    raw = json.loads((output / filename).read_text())
    pods = raw.get("items", [])
    if not pods:
        raise ValueError(f"no recorder Agent pods in {filename}")
    if expected_single and len(pods) != 1:
        raise ValueError("expected one recorder Agent pod")
    identities = []
    pod_records = []
    for pod in sorted(pods, key=lambda item: item.get("metadata", {}).get("name", "")):
        meta = pod.get("metadata", {})
        spec = pod.get("spec", {})
        containers = pod.get("status", {}).get("containerStatuses", [])
        if not containers or any(not item.get("ready") or item.get("restartCount", 0) for item in containers):
            raise ValueError("Agent containers are unready or have restarted")
        container_map = {item["name"]: item.get("containerID", "") for item in containers}
        identities.append(
            (
                meta.get("name", ""),
                meta.get("uid", ""),
                spec.get("nodeName", ""),
                tuple(sorted(container_map.items())),
            )
        )
        agent_container = next((item for item in containers if item.get("name") == "agent"), containers[0])
        pod_records.append(
            {
                "name": meta.get("name", "unknown"),
                "uid": meta.get("uid", "unknown"),
                "node_name": spec.get("nodeName", "unknown"),
                "container_name": agent_container.get("name", "agent"),
                "container_id": agent_container.get("containerID", "unknown"),
                "image": agent_container.get("image", "unknown"),
                "image_id": agent_container.get("imageID", "unknown"),
                "ready": bool(agent_container.get("ready")),
                "restart_count": int(agent_container.get("restartCount", 0)),
            }
        )
    return tuple(identities), pod_records


def infer_source_pod(filename, pod_names, manifest_file_map):
    entry = manifest_file_map.get(filename)
    if entry and entry.get("source_pod"):
        return entry["source_pod"]
    stem = filename[:-8] if filename.endswith(".parquet") else filename
    for pod_name in sorted(pod_names, key=len, reverse=True):
        sanitized = re.sub(r"[^a-zA-Z0-9_.-]", "-", pod_name)
        if stem.endswith(f"-{sanitized}") or filename.startswith(f"{sanitized}-"):
            return pod_name
    if len(pod_names) == 1:
        return pod_names[0]
    return "unknown"


def validate(
    output,
    flush_arg="",
    backend_arg="",
    run_id_arg="",
    scenario_dir_arg="",
    episode_arg="",
    cluster_name_arg="",
    namespace_arg="",
):
    import pyarrow as pa
    import pyarrow.compute as pc
    import pyarrow.parquet as pq

    parquet_dir = output / "parquet"
    parquet_dir.mkdir(exist_ok=True)

    now_iso = dt.datetime.now(dt.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    archive_to_remove = None

    if (output / "artifact.zip").exists():
        archive_to_remove = output / "artifact.zip"
        with zipfile.ZipFile(archive_to_remove) as zf:
            infos = zf.infolist()
            seen_names = set()
            result_members = []
            parquet_members = []
            metadata_members = []
            manifest_member = None
            for info in infos:
                raw_name = info.filename
                path = PurePosixPath(raw_name)
                mode_bits = (info.external_attr >> 16) & 0o170000
                if mode_bits == 0o120000:
                    raise ValueError(f"unsafe symlink entry in zip archive: {raw_name}")
                if path.is_absolute() or ".." in path.parts:
                    raise ValueError(f"unsafe archive entry: {raw_name}")
                if raw_name.endswith("/"):
                    continue
                norm = path.as_posix()
                if norm in seen_names:
                    raise ValueError(f"duplicate archive entry: {raw_name}")
                seen_names.add(norm)
                if norm == "tmp/gensim-archive/recording-manifest.json":
                    manifest_member = info
                elif norm.startswith("tmp/gensim-archive/results/") and len(path.parts) == 4 and norm.endswith(".json"):
                    result_members.append(info)
                elif (
                    norm.startswith("tmp/gensim-archive/metadata/") and len(path.parts) == 4 and norm.endswith(".json")
                ):
                    metadata_members.append(info)
                elif norm.startswith("tmp/gensim-archive/parquet/") and len(path.parts) == 4:
                    parquet_members.append(info)
                else:
                    raise ValueError(f"unexpected archive entry path: {raw_name}")

            if not result_members:
                raise ValueError("archive is missing tmp/gensim-archive/results/<scenario>.json")
            if len(result_members) > 1:
                raise ValueError("archive contains multiple episode result JSON files")

            with zf.open(result_members[0]) as src, (output / "episode.json").open("xb") as dst:
                shutil.copyfileobj(src, dst)
            if manifest_member is not None:
                with zf.open(manifest_member) as src, (output / "recording-manifest.json").open("xb") as dst:
                    shutil.copyfileobj(src, dst)
            for info in metadata_members:
                target_name = PurePosixPath(info.filename).name
                with zf.open(info) as src, (output / target_name).open("xb") as dst:
                    shutil.copyfileobj(src, dst)
            for info in parquet_members:
                target_name = PurePosixPath(info.filename).name
                with zf.open(info) as src, (parquet_dir / target_name).open("xb") as dst:
                    shutil.copyfileobj(src, dst)
    elif (output / "recording.tar").exists():
        archive_to_remove = output / "recording.tar"
        with tarfile.open(archive_to_remove) as archive:
            members = archive.getmembers()
            names = set()
            for member in members:
                path = PurePosixPath(member.name)
                if member.isdir() and member.name in (".", "./"):
                    continue
                if (
                    not member.isfile()
                    or path.is_absolute()
                    or len(path.parts) != 1
                    or path.name == ".."
                    or path.name in names
                ):
                    raise ValueError(f"unsafe or duplicate archive entry: {member.name}")
                names.add(path.name)
            for member in members:
                if member.isfile():
                    destination = parquet_dir / PurePosixPath(member.name).name
                    with archive.extractfile(member) as source, destination.open("xb") as target:
                        shutil.copyfileobj(source, target)

    existing_manifest = {}
    if (output / "recording-manifest.json").exists():
        existing_manifest = json.loads((output / "recording-manifest.json").read_text())

    backend = (
        backend_arg
        or existing_manifest.get("backend")
        or ("eks" if archive_to_remove and archive_to_remove.name == "artifact.zip" else "vm")
    )
    if backend not in ("vm", "eks"):
        raise ValueError(f"unsupported backend: {backend}")

    recorder_settings = dict(existing_manifest.get("recorder_settings") or {})
    if flush_arg not in ("", None):
        flush = int(flush_arg)
    elif recorder_settings.get("flush_seconds"):
        flush = int(recorder_settings["flush_seconds"])
    elif recorder_settings.get("flush_interval"):
        flush = parse_duration_seconds(recorder_settings["flush_interval"])
    else:
        flush = 60
    if flush <= 0:
        raise ValueError("flush interval must be positive seconds")

    errors = list(existing_manifest.get("errors") or [])
    pod_records = list(existing_manifest.get("pods") or [])

    if backend == "vm":
        current_identity, pod_records = parse_pod_snapshot(output, "agent-fetch.json", expected_single=True)
        history_verified = (output / "agent-before.json").exists() and (output / "agent-after.json").exists()
        if history_verified:
            before_identity, _ = parse_pod_snapshot(output, "agent-before.json", expected_single=True)
            after_identity, _ = parse_pod_snapshot(output, "agent-after.json", expected_single=True)
            if not before_identity == after_identity == current_identity:
                raise ValueError("Agent changed between the episode and fetching")
    else:
        history_verified = (output / "agent-before.json").exists() and (output / "agent-after.json").exists()
        if not history_verified:
            errors.append("missing agent-before.json or agent-after.json snapshot in EKS artifact")
        else:
            try:
                before_identity, before_records = parse_pod_snapshot(output, "agent-before.json", expected_single=False)
                after_identity, after_records = parse_pod_snapshot(output, "agent-after.json", expected_single=False)
                if before_identity != after_identity:
                    errors.append("Agent pod or node topology changed during the episode")
                if (output / "agent-fetch.json").exists():
                    fetch_identity, _ = parse_pod_snapshot(output, "agent-fetch.json", expected_single=False)
                    if fetch_identity != after_identity:
                        errors.append("Agent pod or node topology changed before collection completed")
                before_restarts = {item["name"]: item["restart_count"] for item in before_records}
                pod_records = []
                for item in after_records:
                    rec = dict(item)
                    rec["restart_count_before"] = before_restarts.get(item["name"], 0)
                    rec["restart_count_after"] = item["restart_count"]
                    pod_records.append(rec)
            except ValueError as exc:
                errors.append(str(exc))

    manifest_states = dict(existing_manifest.get("states") or {})
    if manifest_states.get("collection") == "failed":
        errors.append("remote collection state is failed")
    if manifest_states.get("upload") == "failed":
        errors.append("remote upload state is failed")

    manifest_files = list(existing_manifest.get("files") or [])
    manifest_file_map = {}
    for item in manifest_files:
        fname = item.get("filename")
        if not fname:
            continue
        manifest_file_map[fname] = item
        fpath = parquet_dir / fname
        if not fpath.exists():
            errors.append(f"missing file declared in manifest: {fname}")
            continue
        actual_size = fpath.stat().st_size
        expected_size = item.get("size_bytes")
        if expected_size is not None and int(expected_size) != actual_size:
            errors.append(f"size mismatch for {fname}: expected {expected_size}, got {actual_size}")
        expected_sha = item.get("sha256")
        if expected_sha and sha256_file(fpath) != expected_sha:
            errors.append(f"sha256 mismatch for {fname}")

    episode = json.loads((output / "episode.json").read_text())
    start, end = timestamp(episode["start_time"]), timestamp(episode["end_time"])
    if start >= end:
        raise ValueError("episode window must have positive duration")

    families = {family: {"files": 0, "rows": 0, "times": set()} for family in ("metrics", "logs")}
    dropped = []
    per_pod_families = {}
    known_pod_names = [item.get("name", "") for item in pod_records if item.get("name")]
    for pod_name in known_pod_names:
        per_pod_families[pod_name] = {"metrics": 0, "logs": 0}

    for filename in sorted(parquet_dir.glob("*.parquet")):
        family = next((key for key in families if filename.name.startswith(f"observer-{key}-")), None)
        if family is None:
            raise ValueError(f"unexpected parquet family: {filename.name}")
        try:
            table = pq.read_table(filename)
        except Exception as exc:
            if backend == "vm":
                raise
            errors.append(f"corrupt parquet file {filename.name}: {exc}")
            continue
        if "Time" not in table.column_names:
            if backend == "vm":
                raise ValueError(f"missing Time column in {filename.name}")
            errors.append(f"missing Time column in {filename.name}")
            continue
        column = table.column("Time")
        limits = pc.min_max(column).as_py() if len(column) else None
        if not limits or limits["min"] is None:
            dropped.append(filename)
            continue
        if pa.types.is_timestamp(column.type):
            divisor = {"s": 1, "ms": 1000, "us": 1000000, "ns": 1000000000}[column.type.unit]
        elif pa.types.is_integer(column.type):
            divisor = integer_divisor(limits["min"])
        else:
            raise ValueError(f"unsupported Time column: {column.type}")
        times = [value / divisor for value in pc.unique(pc.cast(column, pa.int64())).to_pylist() if value is not None]
        inside = [value for value in times if start <= value <= end]
        if max(times) < start or min(times) > end:
            dropped.append(filename)
            continue
        raw_time = pc.cast(column, pa.int64())
        selected = pc.and_(
            pc.greater_equal(raw_time, int(start * divisor)), pc.less_equal(raw_time, int(end * divisor))
        )
        in_window_rows = pc.sum(pc.cast(selected, pa.int64())).as_py() or 0
        families[family]["files"] += 1
        families[family]["rows"] += in_window_rows
        families[family]["times"].update(inside)
        source_pod = infer_source_pod(filename.name, known_pod_names, manifest_file_map)
        if source_pod in per_pod_families and in_window_rows > 0:
            per_pod_families[source_pod][family] += 1

    warnings = [] if history_verified else ["no episode Agent snapshots; restart/replacement history is unknown"]
    for family, stats in families.items():
        times = sorted(stats.pop("times"))
        if not times:
            errors.append(f"no nonempty {family} files overlap the episode")
            stats.update(observed_window=None, max_gap_seconds=None)
            continue
        gaps = [times[0] - start, end - times[-1]] + [later - times[index] for index, later in enumerate(times[1:])]
        stats.update(observed_window=[times[0], times[-1]], max_gap_seconds=max(gaps))
        if gaps[0] > flush or gaps[1] > flush or any(gap > 2 * flush for gap in gaps[2:]):
            warnings.append(
                f"{family} has sparse timestamps or gaps; inspect coverage before claiming a complete recording"
            )

    if backend == "eks" and known_pod_names:
        for pod_name in known_pod_names:
            counts = per_pod_families.get(pod_name, {"metrics": 0, "logs": 0})
            if counts["metrics"] == 0 or counts["logs"] == 0:
                errors.append(
                    f"recorder pod {pod_name} is missing in-window data (metrics={counts['metrics']}, logs={counts['logs']})"
                )

    dropped_names = {path.name for path in dropped}
    file_inventory = []
    for path in sorted(parquet_dir.iterdir()):
        if not path.is_file():
            continue
        prior = manifest_file_map.get(path.name, {})
        family = next((key for key in ("metrics", "logs") if path.name.startswith(f"observer-{key}-")), "sidecar")
        source_pod = infer_source_pod(path.name, known_pod_names, manifest_file_map)
        retained = (path.name not in dropped_names) if not errors else True
        file_inventory.append(
            {
                "filename": path.name,
                "original_filename": prior.get("original_filename", path.name),
                "source_pod": source_pod,
                "family": family,
                "size_bytes": path.stat().st_size,
                "sha256": prior.get("sha256") or sha256_file(path),
                "retained": retained,
            }
        )

    if "success" in episode:
        episode_outcome = "success" if episode.get("success") else "failure"
    else:
        episode_outcome = manifest_states.get("episode_outcome") or "unknown"

    observed_images = sorted(
        {
            item.get("image_id", "unknown")
            for item in pod_records
            if item.get("image_id") and item.get("image_id") != "unknown"
        }
    )

    manifest = {
        "schema_version": 1,
        "run_id": run_id_arg or existing_manifest.get("run_id") or episode.get("run_id") or "unknown",
        "backend": backend,
        "scenario_dir": scenario_dir_arg
        or existing_manifest.get("scenario_dir")
        or episode.get("episode")
        or "unknown",
        "episode": episode_arg or existing_manifest.get("episode") or episode.get("scenario") or "unknown",
        "cycle": int(existing_manifest.get("cycle") or episode.get("cycle") or 1),
        "gensim_sha": existing_manifest.get("gensim_sha") or "unknown",
        "episode_sha": existing_manifest.get("episode_sha") or "unknown",
        "requested_agent_image": (
            existing_manifest.get("requested_agent_image") or (pod_records[0]["image"] if pod_records else "unknown")
        ),
        "observed_agent_image_ids": observed_images or existing_manifest.get("observed_agent_image_ids") or [],
        "recording_format": existing_manifest.get("recording_format") or "parquet-v1",
        "recorder_settings": {
            "enabled": bool(recorder_settings.get("enabled", True)),
            "output_dir": recorder_settings.get("output_dir") or "/tmp/observer-metrics",
            "flush_interval": recorder_settings.get("flush_interval") or f"{flush}s",
            "flush_seconds": flush,
            "retention": recorder_settings.get("retention") or "unknown",
            "recording_only": recorder_settings.get("recording_only", True),
            "mode": recorder_settings.get("mode") or "record-parquet",
        },
        "cluster_name": (
            cluster_name_arg
            or existing_manifest.get("cluster_name")
            or ("kind-gadget-dev" if backend == "vm" else "gensim")
        ),
        "namespace": namespace_arg or existing_manifest.get("namespace") or "default",
        "window": {
            "start_time": episode["start_time"],
            "end_time": episode["end_time"],
        },
        "pods": pod_records,
        "agent_history_verified": history_verified,
        "collection_started_at": existing_manifest.get("collection_started_at") or now_iso,
        "collection_completed_at": existing_manifest.get("collection_completed_at") or now_iso,
        "validated_at": now_iso,
        "files": file_inventory,
        "states": {
            "episode_outcome": episode_outcome,
            "collection": manifest_states.get("collection") or "succeeded",
            "upload": manifest_states.get("upload") or ("not_applicable" if backend == "vm" else "succeeded"),
            "validation": "passed" if not errors else "failed",
        },
        "errors": errors,
        "coverage_warnings": warnings,
    }
    (output / "recording-manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")

    report = {
        "usable": not errors,
        "backend": backend,
        "run_id": manifest["run_id"],
        "window": [episode["start_time"], episode["end_time"]],
        "flush_seconds": flush,
        "families": families,
        "recorder_pods": len(pod_records),
        "removed": len(dropped) if not errors else 0,
        "errors": errors,
        "coverage_warnings": warnings,
        "agent_history_verified": history_verified,
    }
    (output / "validation.json").write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(report, indent=2))
    if errors:
        raise SystemExit("recording validation failed; keep partial output for diagnosis, do not replay")
    for filename in dropped:
        filename.unlink()
    if archive_to_remove is not None and archive_to_remove.exists():
        archive_to_remove.unlink()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("wait", "validate"))
    parser.add_argument("path", type=Path)
    parser.add_argument("flush", nargs="?", default="")
    parser.add_argument("--backend", default="")
    parser.add_argument("--run-id", default="")
    parser.add_argument("--scenario-dir", default="")
    parser.add_argument("--episode", default="")
    parser.add_argument("--cluster-name", default="")
    parser.add_argument("--namespace", default="")
    arguments = parser.parse_args()
    if arguments.command == "wait":
        if arguments.flush == "" or int(arguments.flush) <= 0:
            parser.error("flush interval must be positive seconds")
        print(wait_seconds(arguments.path, int(arguments.flush)))
    else:
        if arguments.flush != "" and int(arguments.flush) <= 0:
            parser.error("flush interval must be positive seconds")
        validate(
            arguments.path,
            arguments.flush,
            backend_arg=arguments.backend,
            run_id_arg=arguments.run_id,
            scenario_dir_arg=arguments.scenario_dir,
            episode_arg=arguments.episode,
            cluster_name_arg=arguments.cluster_name,
            namespace_arg=arguments.namespace,
        )


if __name__ == "__main__":
    main()
