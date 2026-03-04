"""
GitLab CI YAML loader and resolver.

Handles:
- Multi-file includes (local paths with glob patterns)
- !reference [anchor, key] — GitLab-specific tag
- Standard YAML anchors/aliases
- extends: job inheritance with GitLab deep-merge semantics
- Normalization of job data into Job dataclasses
"""

from __future__ import annotations

import copy
import glob
import re
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

import yaml

# ---------------------------------------------------------------------------
# Custom YAML loader that handles !reference and collects anchors
# ---------------------------------------------------------------------------


class _ReferenceTag:
    """Placeholder for !reference [anchor, key] tags during first-pass load."""

    def __init__(self, args: list):
        self.args = args  # e.g. [".anchor_name", "script"]

    def __repr__(self) -> str:
        return f"!reference {self.args}"


def _reference_constructor(loader: yaml.Loader, node: yaml.SequenceNode) -> _ReferenceTag:
    args = loader.construct_sequence(node)
    return _ReferenceTag(args)


def _make_loader() -> type:
    """Create a custom YAML loader class with !reference support."""
    loader_class = type("GitLabLoader", (yaml.SafeLoader,), {})
    loader_class.add_constructor("!reference", _reference_constructor)
    return loader_class


# ---------------------------------------------------------------------------
# Job dataclass
# ---------------------------------------------------------------------------


@dataclass
class Job:
    name: str
    stage: str
    needs: list[str] = field(default_factory=list)
    script: list[str] = field(default_factory=list)
    artifacts: dict = field(default_factory=dict)
    rules: list[dict] = field(default_factory=list)
    trigger: dict | None = None
    tags: list[str] = field(default_factory=list)
    image: str | None = None
    # Raw data for anything not captured above
    raw: dict = field(default_factory=dict)

    @property
    def platform(self) -> str:
        """Infer platform from runner tags."""
        tag_str = " ".join(self.tags).lower()
        # Check mac/darwin before windows: "darwin" contains "win" as substring
        if "mac" in tag_str or "darwin" in tag_str or "osx" in tag_str:
            return "mac"
        if "windows" in tag_str or "win" in tag_str:
            return "windows"
        if "arm" in tag_str and "linux" not in tag_str:
            return "arm"
        return "linux"

    @property
    def job_type(self) -> str:
        """Classify job as build/test/deploy/trigger/other."""
        if self.trigger:
            return "trigger"
        stage = self.stage.lower()
        if any(s in stage for s in ("build", "compile", "binary", "package", "packaging")):
            return "build"
        if any(s in stage for s in ("test", "lint", "scan", "check", "benchmark", "functional")):
            return "test"
        if any(s in stage for s in ("deploy", "distribution", "publish", "upload", "install")):
            return "deploy"
        return "other"

    @property
    def s3_produces(self) -> list[str]:
        """S3 URI patterns found in script where this job writes."""
        return _extract_s3_writes(self.script)

    @property
    def s3_consumes(self) -> list[str]:
        """S3 URI patterns found in script where this job reads."""
        return _extract_s3_reads(self.script)


# ---------------------------------------------------------------------------
# S3 pattern extraction
# ---------------------------------------------------------------------------

# Matches s3:// URIs or $VAR-prefixed paths (e.g. $S3_ARTIFACTS_URI/...)
_S3_URI = r"(s3://\S+|\$\{?S3_\S+|\$\{?[A-Z_]+_URI\S*|\$\{?[A-Z_]+_BUCKET\S*)"

_S3_WRITE_RE = re.compile(
    r"\$S3_CP_CMD\s+\S+\s+" + _S3_URI + r"|"
    r"aws\s+s3\s+cp\s+\S+\s+" + _S3_URI + r"|"
    r"aws\s+s3\s+sync\s+\S+\s+" + _S3_URI
)
_S3_READ_RE = re.compile(
    r"\$S3_CP_CMD\s+" + _S3_URI + r"\s+\S+|"
    r"aws\s+s3\s+cp\s+" + _S3_URI + r"\s+\S+|"
    r"aws\s+s3\s+sync\s+" + _S3_URI + r"\s+\S+"
)


def _extract_s3_writes(lines: list[str]) -> list[str]:
    results = []
    for line in lines:
        for m in _S3_WRITE_RE.finditer(line):
            uri = next((g for g in m.groups() if g), None)
            if uri:
                results.append(uri)
    return results


def _extract_s3_reads(lines: list[str]) -> list[str]:
    results = []
    for line in lines:
        for m in _S3_READ_RE.finditer(line):
            uri = next((g for g in m.groups() if g), None)
            if uri:
                results.append(uri)
    return results


# ---------------------------------------------------------------------------
# Deep-merge helpers (GitLab extends: semantics)
# ---------------------------------------------------------------------------

_SCRIPT_KEYS = {"script", "before_script", "after_script"}
_LIST_OVERRIDE_KEYS = {"rules", "needs", "tags", "only", "except"}


def _deep_merge(base: dict, override: dict) -> dict:
    """
    Merge `override` into `base` following GitLab extends: semantics.
    - Scalars: override wins
    - script/before_script/after_script: override wins (no concatenation)
    - rules/needs: override wins
    - variables (dict): deep merge, override wins on conflict
    - Other dicts: recursive merge
    """
    result = copy.deepcopy(base)
    for key, val in override.items():
        if key in _SCRIPT_KEYS or key in _LIST_OVERRIDE_KEYS:
            result[key] = copy.deepcopy(val)
        elif key == "variables" and isinstance(val, dict) and isinstance(result.get(key), dict):
            merged_vars = copy.deepcopy(result[key])
            merged_vars.update(val)
            result[key] = merged_vars
        elif isinstance(val, dict) and isinstance(result.get(key), dict):
            result[key] = _deep_merge(result[key], val)
        else:
            result[key] = copy.deepcopy(val)
    return result


# ---------------------------------------------------------------------------
# Main parser class
# ---------------------------------------------------------------------------


class GitLabCIParser:
    """
    Loads and resolves a GitLab CI pipeline from a root YAML file.

    Usage::

        parser = GitLabCIParser("/path/to/repo")
        jobs = parser.parse()  # -> list[Job]
        stages = parser.stages
        variables = parser.variables
    """

    # Keys that are NOT job definitions
    _RESERVED = {
        "stages",
        "variables",
        "default",
        "include",
        "workflow",
        "image",
        "services",
        "before_script",
        "after_script",
        "cache",
        "pages",
    }

    def __init__(self, repo_root: str | Path):
        self.repo_root = Path(repo_root)
        self._raw: dict[str, Any] = {}  # merged YAML from all files
        self._jobs_raw: dict[str, dict] = {}  # resolved job dicts (after extends)
        self.stages: list[str] = []
        self.variables: dict[str, str] = {}
        self._parsed = False

    # ------------------------------------------------------------------
    # Public API
    # ------------------------------------------------------------------

    def parse(self) -> list[Job]:
        """Full parse: load, include, resolve references, resolve extends, build Jobs."""
        if not self._parsed:
            self._load_all()
            self._resolve_extends()
            self._parsed = True
        return self._build_jobs()

    # ------------------------------------------------------------------
    # Step 1: Load all YAML files
    # ------------------------------------------------------------------

    def _load_all(self) -> None:
        root_path = self.repo_root / ".gitlab-ci.yml"
        merged = self._load_file(root_path)
        self._raw = merged

        # Extract top-level metadata
        self.stages = merged.get("stages", [])
        self.variables = {k: str(v) for k, v in merged.get("variables", {}).items()}

    def _load_file(self, path: Path) -> dict:
        """Load a single YAML file and recursively resolve its includes."""
        loader_class = _make_loader()
        with open(path, encoding="utf-8") as f:
            data = yaml.load(f, Loader=loader_class)  # noqa: S506

        if not isinstance(data, dict):
            return {}

        # Process includes first so anchors from included files are available
        includes = data.pop("include", None) or []
        if isinstance(includes, dict):
            includes = [includes]

        included_data: dict = {}
        for inc in includes:
            for inc_data in self._resolve_include(inc, path):
                included_data = _deep_merge(included_data, inc_data)

        # Merge: included data forms the base, current file overrides
        return _deep_merge(included_data, data)

    def _resolve_include(self, inc: Any, current_file: Path) -> list[dict]:
        """Resolve a single include entry to a list of parsed dicts."""
        results = []

        if isinstance(inc, str):
            # shorthand: include: path
            inc = {"local": inc}

        if not isinstance(inc, dict):
            return results

        if "local" in inc:
            pattern = inc["local"].lstrip("/")
            # GitLab uses **.yml to mean "any depth"; Python glob needs **/*.yml
            # Replace ** not already followed by / with **/
            pattern = re.sub(r"\*\*(?!/)", "**/*", pattern)
            base = self.repo_root
            matched = sorted(glob.glob(str(base / pattern), recursive=True))
            for fpath in matched:
                try:
                    results.append(self._load_file(Path(fpath)))
                except Exception as e:
                    # Non-fatal: some files may not exist in all contexts
                    print(f"Warning: could not load {fpath}: {e}")

        # Remote includes (project:, file:, template:) are ignored —
        # they reference external repos or GitLab server templates.

        return results

    # ------------------------------------------------------------------
    # Step 2: Resolve !reference tags
    # ------------------------------------------------------------------

    def _resolve_references(self, data: Any, root: dict) -> Any:
        """Recursively resolve !reference [anchor, key] tags."""
        if isinstance(data, _ReferenceTag):
            return self._lookup_reference(data.args, root)
        if isinstance(data, dict):
            return {k: self._resolve_references(v, root) for k, v in data.items()}
        if isinstance(data, list):
            result = []
            for item in data:
                resolved = self._resolve_references(item, root)
                # If a !reference resolves to a list, flatten it in
                if isinstance(item, _ReferenceTag) and isinstance(resolved, list):
                    result.extend(resolved)
                else:
                    result.append(resolved)
            return result
        return data

    def _lookup_reference(self, args: list, root: dict) -> Any:
        """Look up !reference [job_or_anchor, key, ...] in the merged data."""
        if not args:
            return None
        node = root
        for key in args:
            if isinstance(node, dict) and key in node:
                node = node[key]
            else:
                # Reference not found — return empty list as fallback
                return []
        return node

    # ------------------------------------------------------------------
    # Step 3: Resolve extends:
    # ------------------------------------------------------------------

    def _resolve_extends(self) -> None:
        """
        Collect all job-like entries, resolve !references, then resolve extends:
        topologically so that child jobs merge correctly.
        """
        # First pass: collect raw job dicts and resolve !reference tags
        raw_jobs: dict[str, dict] = {}
        for name, val in self._raw.items():
            if name.startswith(".") or name not in self._RESERVED:
                if isinstance(val, dict):
                    resolved = self._resolve_references(val, self._raw)
                    raw_jobs[name] = resolved

        # Second pass: topological resolution of extends:
        resolved: dict[str, dict] = {}

        def resolve_one(name: str, seen: set[str]) -> dict:
            if name in resolved:
                return resolved[name]
            if name in seen:
                # Circular — just return as-is
                return raw_jobs.get(name, {})
            seen = seen | {name}
            job_data = copy.deepcopy(raw_jobs.get(name, {}))
            extends = job_data.pop("extends", None)
            if extends is None:
                resolved[name] = job_data
                return job_data
            if isinstance(extends, str):
                extends = [extends]
            # Merge base classes left-to-right, then apply job_data on top
            merged_base: dict = {}
            for parent_name in extends:
                parent = resolve_one(parent_name, seen)
                merged_base = _deep_merge(merged_base, parent)
            result = _deep_merge(merged_base, job_data)
            resolved[name] = result
            return result

        for name in list(raw_jobs.keys()):
            resolve_one(name, set())

        self._jobs_raw = resolved

    # ------------------------------------------------------------------
    # Step 4: Build Job objects
    # ------------------------------------------------------------------

    def _build_jobs(self) -> list[Job]:
        jobs = []
        for name, data in self._jobs_raw.items():
            # Skip hidden (template) jobs — they start with "."
            if name.startswith("."):
                continue
            job = self._build_job(name, data)
            jobs.append(job)
        return jobs

    def _build_job(self, name: str, data: dict) -> Job:
        stage = data.get("stage", "test")  # GitLab default stage

        # needs: can be list of strings or list of {job: str, ...} dicts
        needs_raw = data.get("needs", [])
        needs: list[str] = []
        if isinstance(needs_raw, list):
            for item in needs_raw:
                if isinstance(item, str):
                    needs.append(item)
                elif isinstance(item, dict) and "job" in item:
                    needs.append(item["job"])
                elif isinstance(item, dict) and "pipeline" in item:
                    pass  # cross-pipeline needs — skip

        # script: normalize to list of strings
        script = _normalize_str_list(data.get("script", []))

        # artifacts
        artifacts = data.get("artifacts", {}) or {}

        # rules
        rules = data.get("rules", []) or []

        # trigger
        trigger = data.get("trigger", None)

        # tags
        tags = _normalize_str_list(data.get("tags", []))

        # image
        image_raw = data.get("image", None)
        image = None
        if isinstance(image_raw, str):
            image = image_raw
        elif isinstance(image_raw, dict):
            image = image_raw.get("name")

        return Job(
            name=name,
            stage=str(stage),
            needs=needs,
            script=script,
            artifacts=artifacts,
            rules=rules,
            trigger=trigger,
            tags=tags,
            image=image,
            raw=data,
        )


def _normalize_str_list(val: Any) -> list[str]:
    """Convert scalar, list of scalars, or None to a list of strings."""
    if val is None:
        return []
    if isinstance(val, str):
        return [val]
    if isinstance(val, list):
        result = []
        for item in val:
            if isinstance(item, str):
                result.append(item)
            elif item is not None:
                result.append(str(item))
        return result
    return []
