"""Helpers for submitting local Observer builds to the remote DDEval worker."""

import hashlib
import re
from copy import deepcopy
from pathlib import Path
from typing import TextIO
from urllib.parse import parse_qs, unquote, urlsplit, urlunsplit

ARTIFACT_BUCKET = "qbranch-gensim-recordings"
ARTIFACT_PREFIX = "observer-log-ad-binaries/test-binaries"
ARTIFACT_REGION = "us-east-1"
TESTBENCH_FILENAME = "anomalydetection-testbench"

_ALLOWED_ARTIFACT_HOSTS = {
    f"{ARTIFACT_BUCKET}.s3.amazonaws.com",
    f"{ARTIFACT_BUCKET}.s3.{ARTIFACT_REGION}.amazonaws.com",
}
_SHA256_PATTERN = re.compile(r"[0-9a-f]{64}")


def sha256_file(path: str | Path) -> str:
    """Return the lowercase SHA-256 digest of a file without loading it into memory."""
    digest = hashlib.sha256()
    with Path(path).open("rb") as source:
        while chunk := source.read(1024 * 1024):
            digest.update(chunk)
    return digest.hexdigest()


def validate_linux_amd64_executable(path: str | Path) -> None:
    """Require a 64-bit little-endian x86 ELF binary for the DDBuild worker."""
    with Path(path).open("rb") as binary:
        header = binary.read(20)
    is_elf64_little_endian_x86 = (
        len(header) == 20
        and header[:4] == b"\x7fELF"
        and header[4] == 2
        and header[5] == 1
        and int.from_bytes(header[18:20], "little") == 62
    )
    if not is_elf64_little_endian_x86:
        raise ValueError("testbench must be a Linux/amd64 executable; rebuild it with --build")


def local_testbench_key(digest: str) -> str:
    """Build a content-addressed S3 key for a local testbench binary."""
    digest = digest.strip().lower()
    if not _SHA256_PATTERN.fullmatch(digest):
        raise ValueError("testbench digest must be a 64-character hexadecimal SHA-256")

    return f"{ARTIFACT_PREFIX}/sha256/{digest}/{TESTBENCH_FILENAME}"


def artifact_metadata_matches(head_object: dict, digest: str, size: int) -> bool:
    """Return whether S3 metadata identifies the expected content-addressed binary."""
    if not isinstance(head_object, dict):
        return False
    metadata = head_object.get("Metadata")
    if not isinstance(metadata, dict):
        return False
    try:
        content_length = int(head_object.get("ContentLength", -1))
    except (TypeError, ValueError):
        return False
    return content_length == size and str(metadata.get("sha256", "")).lower() == digest.lower()


def validate_presigned_artifact_url(url: str) -> None:
    """Reject URLs that do not point to the expected private local-artifact prefix."""
    parsed = urlsplit(url)
    try:
        port = parsed.port
    except ValueError as error:
        raise ValueError("presigned artifact URL contains an invalid port") from error

    if parsed.scheme != "https":
        raise ValueError("presigned artifact URL must use HTTPS")
    if parsed.username or parsed.password or port is not None:
        raise ValueError("presigned artifact URL must not contain credentials or a port")
    if parsed.hostname not in _ALLOWED_ARTIFACT_HOSTS:
        raise ValueError(f"presigned artifact URL must target {ARTIFACT_BUCKET}")
    if parsed.fragment:
        raise ValueError("presigned artifact URL must not contain a fragment")
    if not unquote(parsed.path).lstrip("/").startswith(f"{ARTIFACT_PREFIX}/"):
        raise ValueError(f"presigned artifact URL must target {ARTIFACT_PREFIX}")

    query = parse_qs(parsed.query)
    if not query.get("X-Amz-Signature"):
        raise ValueError("presigned artifact URL is missing its AWS signature")


def redacted_presigned_url(url: str) -> str:
    """Return a useful URL identity without its temporary AWS credentials."""
    parsed = urlsplit(url)
    return urlunsplit((parsed.scheme, parsed.netloc, parsed.path, "<redacted>", ""))


def build_experiment_config(
    base_config: dict,
    *,
    testbench_url: str,
    testbench_sha256: str,
    testbench_config: dict | None = None,
) -> dict:
    """Add the local testbench artifact and optional Observer config to a DDEval config."""
    if not isinstance(base_config, dict):
        raise ValueError("experiment config must be a JSON object")
    if not _SHA256_PATTERN.fullmatch(testbench_sha256):
        raise ValueError("testbench digest must be a 64-character hexadecimal SHA-256")

    config = deepcopy(base_config)
    executor_config = config.get("executor_config") or {}
    if not isinstance(executor_config, dict):
        raise ValueError("experiment config executor_config must be a JSON object")
    executor_config["binary_artifacts"] = {
        "testbench": {
            "uri": testbench_url,
            "sha256": testbench_sha256,
            "filename": TESTBENCH_FILENAME,
        }
    }
    config["executor_config"] = executor_config

    if testbench_config is not None:
        if not isinstance(testbench_config, dict):
            raise ValueError("testbench config must be a JSON object")
        input_parameters = config.get("input_parameters") or {}
        if not isinstance(input_parameters, dict):
            raise ValueError("experiment config input_parameters must be a JSON object")
        input_parameters["testbench_config"] = deepcopy(testbench_config)
        config["input_parameters"] = input_parameters

    return config


class RedactingWriter:
    """Stream text while replacing a secret even when writes split it across chunks."""

    def __init__(self, stream: TextIO, secret: str, replacement: str):
        if not secret:
            raise ValueError("secret must not be empty")
        self._stream = stream
        self._secret = secret
        self._replacement = replacement
        self._buffer = ""

    def write(self, value: str) -> int:
        self._buffer += value
        self._drain(final=False)
        return len(value)

    def flush(self) -> None:
        self._stream.flush()

    def finish(self) -> None:
        self._drain(final=True)
        self._stream.flush()

    def _drain(self, *, final: bool) -> None:
        while self._buffer:
            if self._buffer.startswith(self._secret):
                self._stream.write(self._replacement)
                self._buffer = self._buffer[len(self._secret) :]
                continue

            if final and self._secret.startswith(self._buffer):
                self._stream.write(self._replacement)
                self._buffer = ""
                continue

            if not final and self._secret.startswith(self._buffer):
                return

            next_candidate = self._buffer.find(self._secret[0])
            if next_candidate < 0:
                self._stream.write(self._buffer)
                self._buffer = ""
            elif next_candidate > 0:
                self._stream.write(self._buffer[:next_candidate])
                self._buffer = self._buffer[next_candidate:]
            else:
                self._stream.write(self._buffer[0])
                self._buffer = self._buffer[1:]


def workflow_run_args(
    command_prefix: list[str],
    *,
    service: str,
    project: str,
    dataset: str,
    dataset_version: int,
    config_path: str,
    jobs: int,
    max_attempts: int,
    data_env: str,
    limit: int = 0,
    where_in: str = "",
) -> list[str]:
    """Build the DDEval CLI arguments for the production DDBuild worker."""
    args = [
        *command_prefix,
        "workflow",
        "run",
        "-s",
        service,
        "-p",
        project,
        "-d",
        dataset,
        "--env",
        "ddbuild",
        "--data-env",
        data_env,
        "-f",
        config_path,
        "-j",
        str(jobs),
        "--max-attempts",
        str(max_attempts),
    ]
    if dataset_version:
        args.extend(["--dataset-version", str(dataset_version)])
    if limit:
        args.extend(["--limit", str(limit)])
    if where_in:
        args.extend(["--where-in", where_in])
    return args
