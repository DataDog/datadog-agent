"""UTOF dataclasses — pure data, no logic beyond serialization."""

from __future__ import annotations

import json
from dataclasses import asdict, dataclass, field


@dataclass
class UTOFGitMetadata:
    branch: str = ""
    commit_sha: str = ""
    commit_author: str = ""  # noqa: F841
    base_branch: str = ""
    base_sha: str = ""  # noqa: F841


@dataclass
class UTOFCIMetadata:
    pipeline_id: str = ""
    job_id: str = ""
    job_name: str = ""
    job_url: str = ""


@dataclass
class UTOFEnvironmentMetadata:
    os: str = ""
    os_version: str = ""
    arch: str = ""
    kernel: str = ""
    go_version: str = ""
    agent_flavor: str = ""


@dataclass
class UTOFMetadata:
    test_system: str = "unit"
    timestamp: str = ""
    duration_seconds: float = 0.0
    git: UTOFGitMetadata = field(default_factory=UTOFGitMetadata)
    ci: UTOFCIMetadata = field(default_factory=UTOFCIMetadata)
    environment: UTOFEnvironmentMetadata = field(default_factory=UTOFEnvironmentMetadata)
    build_tags: list[str] = field(default_factory=list)


@dataclass
class UTOFSummary:
    total: int = 0
    passed: int = 0
    failed: int = 0
    skipped: int = 0
    flaky: int = 0
    retried: int = 0  # noqa: F841
    status: str = "pass"


@dataclass
class UTOFFailure:
    message: str = ""
    type: str = ""  # assertion, panic, timeout, build
    stacktrace: str = ""
    raw_output: str = ""  # noqa: F841


@dataclass
class UTOFFlaky:
    is_known_flaky: bool = False
    source: str = ""  # "marker", "washer", "log_pattern"
    pattern: str = ""


@dataclass
class UTOFAttempt:
    """A single execution attempt of a test. Present only when the test was retried."""

    attempt: int = 1
    status: str = "pass"  # pass, fail
    duration_seconds: float = 0.0
    failure: UTOFFailure | None = None


@dataclass
class UTOFTestResult:
    id: str = ""
    name: str = ""
    full_name: str = ""  # original test2json name, e.g. "TestSketch/useStore=true/empty_flush"
    package: str = ""
    suite: str = ""
    type: str = "unit"
    status: str = "pass"  # pass, fail, skip, flaky_pass, flaky_fail
    duration_seconds: float = 0.0
    retry_count: int = 0
    failure: UTOFFailure | None = None
    flaky: UTOFFlaky | None = None
    attempts: list[UTOFAttempt] | None = None
    subtests: list[UTOFTestResult] | None = None
    tags: list[str] = field(default_factory=list)


@dataclass
class UTOFLink:
    label: str = ""
    url: str = ""


@dataclass
class UTOFDocument:
    version: str = "1.0.0"
    metadata: UTOFMetadata = field(default_factory=UTOFMetadata)
    summary: UTOFSummary = field(default_factory=UTOFSummary)
    tests: list[UTOFTestResult] = field(default_factory=list)
    links: list[UTOFLink] = field(default_factory=list)

    def to_dict(self) -> dict:
        """Convert the document to a dictionary, stripping None values."""
        return _strip_none(asdict(self))

    def write_json(self, path: str) -> None:
        """Write the document as formatted JSON to the given path."""
        with open(path, "w") as f:
            json.dump(self.to_dict(), f, indent=2)


def _strip_none(obj):
    """Recursively remove keys with None values from dicts."""
    if isinstance(obj, dict):
        return {k: _strip_none(v) for k, v in obj.items() if v is not None}
    if isinstance(obj, list):
        return [_strip_none(item) for item in obj]
    return obj


@dataclass
class _AssertionBlock:
    """One assertion failure parsed from raw Go test output."""

    trace: str  # file:line location (may be full path)
    error_lines: list[str]  # error message parts
