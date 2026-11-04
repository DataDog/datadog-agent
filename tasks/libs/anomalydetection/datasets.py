"""Read current LLMObs datasets and stage their original recordings for local replay."""

import errno
import hashlib
import json
import os
import re
import shutil
import stat
import tempfile
import urllib.parse
import urllib.request
import zipfile
from pathlib import Path, PurePosixPath

from tasks.libs.anomalydetection.ddeval import sha256_file

DEFAULT_DATASET = "observer-metric-types-2026-10-26"
DEFAULT_PROJECT = "observer-log-ad"
DEFAULT_SITE = "datadoghq.com"


def _api_pages(path, site):
    api_key = os.environ.get("DD_API_KEY") or os.environ.get("DATADOG_API_KEY")
    app_key = os.environ.get("DD_APP_KEY") or os.environ.get("DATADOG_APP_KEY")
    if not api_key or not app_key:
        raise ValueError("Set DD_API_KEY and DD_APP_KEY, or run this command through dd-auth, to read LLMObs datasets")
    seen = set()
    while True:
        request = urllib.request.Request(
            f"https://api.{site}{path}", headers={"DD-API-KEY": api_key, "DD-APPLICATION-KEY": app_key}
        )
        with urllib.request.urlopen(request, timeout=60) as response:
            page = json.load(response)
        yield from page.get("data", [])
        meta = page.get("meta") or {}
        cursor = meta.get("after") or (meta.get("page") or {}).get("after")
        if not cursor:
            return
        if cursor in seen:
            raise ValueError("LLMObs returned a repeated pagination cursor")
        seen.add(cursor)
        query = urllib.parse.urlsplit(path)
        params = urllib.parse.parse_qs(query.query)
        params["page[cursor]"] = [cursor]
        path = query.path + "?" + urllib.parse.urlencode(params, doseq=True)


def read_dataset(dataset="", project=DEFAULT_PROJECT, site=DEFAULT_SITE):
    """Read the latest dataset using Datadog API/application keys from the environment."""
    if site not in {
        "datadoghq.com",
        "datad0g.com",
        "datadoghq.eu",
        "us3.datadoghq.com",
        "us5.datadoghq.com",
        "ap1.datadoghq.com",
        "ap2.datadoghq.com",
        "uk1.datadoghq.com",
    }:
        raise ValueError(f"Unsupported Datadog site: {site}")
    projects = list(
        _api_pages("/api/v2/llm-obs/v1/projects?" + urllib.parse.urlencode({"filter[name]": project}), site)
    )
    matches = [p for p in projects if p.get("attributes", {}).get("name") == project]
    if len(matches) != 1:
        raise ValueError(f"Expected one LLMObs project named {project!r}, found {len(matches)}")
    project_id = matches[0]["id"]
    datasets = list(_api_pages(f"/api/v2/llm-obs/v1/{project_id}/datasets", site))
    if not dataset:
        return [{"id": d["id"], "name": d.get("attributes", {}).get("name")} for d in datasets]
    matches = [d for d in datasets if d["id"] == dataset or d.get("attributes", {}).get("name") == dataset]
    if len(matches) != 1:
        raise ValueError(f"Expected one dataset named or identified by {dataset!r}, found {len(matches)}")
    selected = matches[0]
    # Resolve latest once so all pages describe one coherent current dataset.
    # There is deliberately no historical-version input or selector.
    params = {"page[limit]": 100}
    latest = selected.get("attributes", {}).get("current_version")
    if latest is not None:
        params["filter[version]"] = latest
    records = list(
        _api_pages(
            f"/api/v2/llm-obs/v1/{project_id}/datasets/{selected['id']}/records?" + urllib.parse.urlencode(params), site
        )
    )
    return {"id": selected["id"], "name": selected["attributes"]["name"], "records": records}


def _download(uri, destination, aws_profile):
    parsed = urllib.parse.urlsplit(uri)
    if parsed.scheme == "file":
        shutil.copyfile(urllib.request.url2pathname(parsed.path), destination)
    elif parsed.scheme == "https":
        with urllib.request.urlopen(uri, timeout=120) as source, destination.open("wb") as target:
            shutil.copyfileobj(source, target)
    elif parsed.scheme == "s3":
        import boto3

        try:
            client = boto3.Session(profile_name=aws_profile).client("s3") if aws_profile else boto3.client("s3")
            client.download_file(parsed.netloc, parsed.path.lstrip("/"), str(destination))
        except Exception as error:
            raise ValueError(
                f"Cannot download the S3 recording with AWS profile {aws_profile or 'default'}: {error}"
            ) from error
    else:
        raise ValueError("Artifact URI must use s3://, https://, or file://")


def _extract(archive_path, destination, episode_member):
    with zipfile.ZipFile(archive_path) as archive:
        members = archive.infolist()
        names = [m.filename for m in members]
        if len(names) != len(set(names)):
            raise ValueError("Recording ZIP contains duplicate members")
        for member in members:
            path = PurePosixPath(member.filename)
            mode = member.external_attr >> 16
            if (
                path.is_absolute()
                or ".." in path.parts
                or "\\" in member.filename
                or stat.S_ISLNK(mode)
                or (stat.S_IFMT(mode) and not (stat.S_ISREG(mode) or stat.S_ISDIR(mode)))
            ):
                raise ValueError(f"Unsafe recording ZIP member: {member.filename!r}")
        prefix = "tmp/gensim-archive/" if any(n.startswith("tmp/gensim-archive/") for n in names) else ""
        candidates = [
            n
            for n in names
            if n == prefix + "episode.json" or (prefix and n.startswith(prefix + "results/") and n.endswith(".json"))
        ]
        if episode_member:
            candidates = [n for n in candidates if n == episode_member]
        episodes = []
        for name in candidates:
            try:
                episode = json.loads(archive.read(name))
                if not isinstance(episode, dict):
                    continue
            except (ValueError, TypeError, AttributeError):
                continue
            episodes.append(name)
        if len(episodes) != 1:
            raise ValueError(
                "Recording must contain one episode metadata object; use recording.episode_member to disambiguate"
            )
        selected = {"episode.json": episodes[0]}
        for name in names:
            if name == prefix + "ground_truth.json":
                selected["ground_truth.json"] = name
            elif name.startswith(prefix + "parquet/") and name.endswith(".parquet"):
                relative = name[len(prefix) :]
                if len(PurePosixPath(relative).parts) != 2:
                    raise ValueError("Recording Parquets must be directly inside parquet/")
                selected[relative] = name
        if not any(n.startswith("parquet/") for n in selected):
            raise ValueError("Recording contains no Parquet files")
        for relative, member in selected.items():
            path = destination / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            with archive.open(member) as source, path.open("wb") as target:
                shutil.copyfileobj(source, target)


def _cached_recording(target, identity):
    try:
        return (
            json.loads((target / "recording.json").read_text()) == identity
            and (target / "episode.json").is_file()
            and any((target / "parquet").glob("*.parquet"))
        )
    except (OSError, ValueError):
        return False


def _publish_directory(staged, target):
    """Publish a complete directory, or reuse one another invocation published."""
    try:
        staged.rename(target)
    except OSError as error:
        if error.errno not in {errno.EEXIST, errno.ENOTEMPTY} or not target.is_dir():
            raise


def prepare_dataset(dataset, scenarios_dir, *, scenario="", aws_profile=""):
    """Stage selected dataset rows without changing recording bytes or episode labels."""
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.-]*", str(dataset["id"])):
        raise ValueError("Invalid dataset ID")
    root = Path(scenarios_dir) / ".llmobs" / dataset["id"]
    cache = root / ".recordings"
    cache.mkdir(parents=True, exist_ok=True)
    selected = []
    recordings = {}
    requested = {name.strip() for name in scenario.split(",") if name.strip()}
    for record in dataset["records"]:
        attrs = record.get("attributes", record)
        inputs = attrs.get("input") or {}
        if isinstance(inputs, str):
            inputs = json.loads(inputs)
        if not isinstance(inputs, dict):
            raise ValueError("Dataset input must be an object")
        name = inputs.get("scenario")
        if requested and name not in requested:
            continue
        if not isinstance(name, str) or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.-]*", name):
            raise ValueError(f"Invalid scenario name: {name!r}")
        if name in selected:
            raise ValueError(f"Dataset contains duplicate scenario name: {name}")
        if "recording" not in inputs:
            raise ValueError(f"{name}: dataset replay requires a recording ZIP reference")
        spec = inputs.get("recording") or {}
        digest = str(spec.get("sha256") or "").lower()
        if not re.fullmatch(r"[0-9a-f]{64}", digest):
            raise ValueError(f"{name}: recording.sha256 is required")
        identity = {"sha256": digest, "episode_member": spec.get("episode_member")}
        key = hashlib.sha256(json.dumps(identity, sort_keys=True).encode()).hexdigest()
        target = cache / key
        if not _cached_recording(target, identity):
            print(f"Downloading {name}…", flush=True)
            with tempfile.TemporaryDirectory(dir=cache, prefix=".download-") as temporary:
                staged = Path(temporary)
                archive = staged / "recording.zip"
                _download(spec.get("uri", ""), archive, aws_profile)
                if sha256_file(archive) != digest:
                    raise ValueError(f"{name}: recording SHA-256 mismatch")
                extracted = staged / "scenario"
                extracted.mkdir()
                _extract(archive, extracted, spec.get("episode_member"))
                (extracted / "recording.json").write_text(json.dumps(identity))
                _publish_directory(extracted, target)
                if not _cached_recording(target, identity):
                    raise ValueError(f"Incomplete recording cache at {target}; remove it and retry")
        selected.append(name)
        recordings[name] = target
    if requested - set(selected):
        raise ValueError(f"Scenarios not found in dataset: {', '.join(sorted(requested - set(selected)))}")
    if not selected:
        raise ValueError(f"No matching scenarios in dataset {dataset['name']!r}")
    # Link only the requested recordings, whose checksum-specific paths never
    # change beneath another invocation. This key also bypasses older cache views
    # built from replaceable scenario-name directories.
    selection = hashlib.sha256(
        json.dumps([(name, path.name) for name, path in sorted(recordings.items())]).encode()
    ).hexdigest()
    views = root / ".selections"
    views.mkdir(exist_ok=True)
    replay_root = views / selection
    if not replay_root.exists():
        with tempfile.TemporaryDirectory(dir=views) as temporary:
            staged = Path(temporary) / "scenarios"
            staged.mkdir()
            for name in selected:
                shutil.copytree(recordings[name], staged / name, copy_function=os.link)
            _publish_directory(staged, replay_root)
    return replay_root, selected
