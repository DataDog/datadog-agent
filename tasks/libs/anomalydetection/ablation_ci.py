"""Restore ablation checkpoints from an explicitly selected GitLab job."""

import os
import tempfile
import zipfile
from pathlib import Path, PurePosixPath

import requests


def restore_checkpoint(job_id: str, output: Path) -> None:
    if not job_id.isdigit():
        raise ValueError("OBSERVER_ABLATION_RESUME_JOB_ID must be a numeric job ID")
    if output.exists():
        raise ValueError(f"Refusing to replace existing checkpoint directory: {output}")
    base = os.environ["CI_API_V4_URL"]
    project = os.environ["CI_PROJECT_ID"]
    headers = {"JOB-TOKEN": os.environ["CI_JOB_TOKEN"]}
    with requests.get(
        f"{base}/projects/{project}/jobs/{job_id}/artifacts", headers=headers, stream=True, timeout=60
    ) as response:
        response.raise_for_status()
        with tempfile.TemporaryFile() as download:
            for chunk in response.iter_content(1024 * 1024):
                download.write(chunk)
            download.seek(0)
            with zipfile.ZipFile(download) as archive:
                for item in archive.infolist():
                    path = PurePosixPath(item.filename)
                    if path.is_absolute() or ".." in path.parts or "\\" in item.filename:
                        raise ValueError("Invalid path in job artifacts")
                    if not path.parts or path.parts[0] != output.name or item.is_dir():
                        continue
                    # The JSON checkpoint is sufficient to reconstruct the optimizer and pending workflows.
                    # Do not restore the SQLite lock or unpickle any artifact.
                    if path.suffix not in {".json", ".md"}:
                        continue
                    destination = output.joinpath(*path.parts[1:])
                    destination.parent.mkdir(parents=True, exist_ok=True)
                    destination.write_bytes(archive.read(item))
    if not (output / "study.json").is_file():
        raise ValueError(f"Job {job_id} has no ablation checkpoint")
