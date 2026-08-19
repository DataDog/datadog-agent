"""UTOF dataclasses — pure data, no logic beyond serialization."""

from __future__ import annotations

import json
from collections.abc import Iterator
from dataclasses import asdict, dataclass, field


@dataclass
class UTOFGitMetadata:
    branch: str = ""
    commit_sha: str = ""
    commit_author: str = ""  # noqa: F841
    base_branch: str = ""
    base_sha: str = ""  # noqa: F841

    @classmethod
    def from_dict(cls, data: dict) -> UTOFGitMetadata:
        return cls(**{k: v for k, v in data.items() if k in _field_names(cls)})


@dataclass
class UTOFCIMetadata:
    pipeline_id: str = ""
    job_id: str = ""
    job_name: str = ""
    job_url: str = ""

    @classmethod
    def from_dict(cls, data: dict) -> UTOFCIMetadata:
        return cls(**{k: v for k, v in data.items() if k in _field_names(cls)})


@dataclass
class UTOFEnvironmentMetadata:
    os: str = ""
    os_version: str = ""
    arch: str = ""
    kernel: str = ""
    runtime_version: str = ""  # noqa: F841
    agent_flavor: str = ""
    runner_cpu_request: str = ""  # noqa: F841
    runner_memory_request: str = ""  # noqa: F841

    @classmethod
    def from_dict(cls, data: dict) -> UTOFEnvironmentMetadata:
        return cls(**{k: v for k, v in data.items() if k in _field_names(cls)})


@dataclass
class UTOFMetadata:
    test_system: str = "unit"
    timestamp: str = ""
    duration_seconds: float = 0.0
    git: UTOFGitMetadata = field(default_factory=UTOFGitMetadata)
    ci: UTOFCIMetadata = field(default_factory=UTOFCIMetadata)
    environment: UTOFEnvironmentMetadata = field(default_factory=UTOFEnvironmentMetadata)
    build_tags: list[str] = field(default_factory=list)

    @classmethod
    def from_dict(cls, data: dict) -> UTOFMetadata:
        return cls(
            test_system=data.get("test_system", "unit"),
            timestamp=data.get("timestamp", ""),
            duration_seconds=data.get("duration_seconds", 0.0),
            git=UTOFGitMetadata.from_dict(data.get("git") or {}),
            ci=UTOFCIMetadata.from_dict(data.get("ci") or {}),
            environment=UTOFEnvironmentMetadata.from_dict(data.get("environment") or {}),
            build_tags=list(data.get("build_tags") or []),
        )


@dataclass
class UTOFSummary:
    total: int = 0
    passed: int = 0
    failed: int = 0
    skipped: int = 0
    flaky: int = 0
    retried: int = 0  # noqa: F841
    status: str = "pass"

    @classmethod
    def from_dict(cls, data: dict) -> UTOFSummary:
        return cls(**{k: v for k, v in data.items() if k in _field_names(cls)})


@dataclass
class UTOFFailure:
    message: str = ""
    type: str = ""  # assertion, panic, timeout, build, infrastructure
    stacktrace: str = ""
    raw_output: str = ""  # noqa: F841
    # True when the failure originates from a direct assertion on this test.
    # False when it is inferred/propagated from child test failures.
    # Consumers can use this to avoid rendering redundant failure details on
    # parent nodes that are already explained by their children.
    direct: bool = False

    @classmethod
    def from_dict(cls, data: dict) -> UTOFFailure:
        return cls(**{k: v for k, v in data.items() if k in _field_names(cls)})


@dataclass
class UTOFFlaky:
    is_known_flaky: bool = False
    source: str = ""  # "marker", "washer", "log_pattern"
    pattern: str = ""

    @classmethod
    def from_dict(cls, data: dict) -> UTOFFlaky:
        return cls(**{k: v for k, v in data.items() if k in _field_names(cls)})


@dataclass
class UTOFAttempt:
    """A single execution attempt of a test."""

    attempt: int = 1
    status: str = "pass"  # pass, fail
    duration_seconds: float = 0.0
    failure: UTOFFailure | None = None

    @classmethod
    def from_dict(cls, data: dict) -> UTOFAttempt:
        failure = data.get("failure")
        return cls(
            attempt=data.get("attempt", 1),
            status=data.get("status", "pass"),
            duration_seconds=data.get("duration_seconds", 0.0),
            failure=UTOFFailure.from_dict(failure) if failure else None,
        )


@dataclass
class UTOFTestResult:
    id: str = ""
    name: str = ""
    full_name: str = ""  # original fully-qualified test name, e.g. "TestSketch/useStore=true/empty_flush"
    package: str = ""
    suite: str = ""
    type: str = "unit"
    status: str = "pass"  # pass, fail, skip, flaky_pass, flaky_fail
    duration_seconds: float = 0.0
    retry_count: int = 0
    flaky: UTOFFlaky | None = None
    attempts: list[UTOFAttempt] = field(default_factory=list)
    subtests: list[UTOFTestResult] | None = None
    tags: list[str] = field(default_factory=list)

    @classmethod
    def from_dict(cls, data: dict) -> UTOFTestResult:
        flaky = data.get("flaky")
        subtests = data.get("subtests")
        return cls(
            id=data.get("id", ""),
            name=data.get("name", ""),
            full_name=data.get("full_name", ""),
            package=data.get("package", ""),
            suite=data.get("suite", ""),
            type=data.get("type", "unit"),
            status=data.get("status", "pass"),
            duration_seconds=data.get("duration_seconds", 0.0),
            retry_count=data.get("retry_count", 0),
            flaky=UTOFFlaky.from_dict(flaky) if flaky else None,
            attempts=[UTOFAttempt.from_dict(a) for a in data.get("attempts") or []],
            subtests=[UTOFTestResult.from_dict(t) for t in subtests] if subtests else None,
            tags=list(data.get("tags") or []),
        )


@dataclass
class UTOFLink:
    label: str = ""
    url: str = ""

    @classmethod
    def from_dict(cls, data: dict) -> UTOFLink:
        return cls(**{k: v for k, v in data.items() if k in _field_names(cls)})


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

    @classmethod
    def from_dict(cls, data: dict) -> UTOFDocument:
        """Reconstruct a UTOFDocument from a dict produced by to_dict()/write_json()."""
        return cls(
            version=data.get("version", "1.0.0"),
            metadata=UTOFMetadata.from_dict(data.get("metadata") or {}),
            summary=UTOFSummary.from_dict(data.get("summary") or {}),
            tests=[UTOFTestResult.from_dict(t) for t in data.get("tests") or []],
            links=[UTOFLink.from_dict(link) for link in data.get("links") or []],
        )

    @classmethod
    def from_json(cls, raw: str | bytes) -> UTOFDocument:
        """Reconstruct a UTOFDocument from a JSON string/bytes produced by write_json()."""
        return cls.from_dict(json.loads(raw))


def walk_tests(tests: list[UTOFTestResult]) -> Iterator[UTOFTestResult]:
    """Recursively yield every leaf UTOFTestResult in a tree of tests.

    A "leaf" is a test with no subtests. Parent nodes that only exist to
    group subtests are not yielded, matching how UTOFSummary counts are
    computed (count_leaves in go/parser/run_parser.py) and avoiding
    double-counting a test and its subtests as separate failures.
    """
    for t in tests:
        if t.subtests:
            yield from walk_tests(t.subtests)
        else:
            yield t


def _field_names(cls) -> frozenset[str]:
    return frozenset(f.name for f in cls.__dataclass_fields__.values())


def _strip_none(obj):
    """Recursively remove keys with None or empty-list values from dicts."""
    if isinstance(obj, dict):
        return {k: _strip_none(v) for k, v in obj.items() if v is not None and v != []}
    if isinstance(obj, list):
        return [_strip_none(item) for item in obj]
    return obj
