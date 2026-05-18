"""M3 transitive bundler for the DDOT Collector configuration JSON Schema.

Walks every Collector component schema (manifest + local), follows their
`$ref`s into shared upstream types, and builds a single JSON Schema
2020-12 document with a flat `$defs` registry.

Naming scheme inside `$defs`:

  - `component__<class>__<type>` — the root of a registered Collector
    component schema (e.g. `component__processor__infraattributes`).
  - `component__<class>__<type>__<inner>` — a nested `$defs` entry from
    that component's own file.
  - `shared__<short-path>__<type>` — a shared (non-component) upstream
    type (e.g. `shared__pkg_datadog_config__api_config`,
    `shared__core_config_confighttp__client_config`). Short-paths strip
    known repo prefixes for readability.

`$ref`s are rewritten to JSON Pointers into the bundle's `$defs`. Refs
that cannot be resolved (missing schema upstream, dangling sub-package,
etc.) are handled by the chosen `--missing` strategy.

Run as:

    python -m tasks.libs.otelcol_schema.bundle \\
        [--out <path>] [--report <path>] \\
        [--missing permissive|strict]

Default `--missing` is `permissive`: a placeholder `{type: object,
additionalProperties: true}` is inserted in `$defs` for every
unresolved target so the bundle stays internally consistent.
"""

from __future__ import annotations

import argparse
import functools
import hashlib
import json
import re
import sys
from collections import defaultdict, deque
from collections.abc import Mapping
from dataclasses import dataclass, field
from pathlib import Path
from types import MappingProxyType
from typing import Any

import yaml

from tasks.libs.otelcol_schema._refs import (
    KNOWN_NAMESPACES,
    classify_ref,
    find_module_schema,
    follow_relative,
    is_component_mode,
    module_cache_dir,
    parse_go_mod_versions,
    parse_go_sum_versions,
    parse_namespace_relative,
    repo_namespace_of,
    resolve_relative_go_path,
    walk_refs,
)
from tasks.libs.otelcol_schema.convert import JSON_SCHEMA_DRAFT, validate_meta
from tasks.libs.otelcol_schema.inventory import (
    LOCAL_SCHEMAS,
    MANIFEST_PATH,
    REPO_ROOT,
    ensure_downloaded,
    gomodcache,
    parse_manifest,
)

# ---------------------------------------------------------------------------
# Bundle identity + versioning
# ---------------------------------------------------------------------------

# Stable URI identifying this bundle. Per JSON Schema 2020-12, `$id` is the
# base URI against which other refs in the doc resolve. Consumers (Fleet
# validator, IDE schema stores, doc generators) key off this.
BUNDLE_ID = "https://github.com/DataDog/datadog-agent/comp/otelcol/collector-config.schema.json"

BUNDLE_TITLE = "Datadog Distribution of OpenTelemetry Collector configuration"

# Semver of the BUNDLE FORMAT itself — independent of the Collector version
# the bundle was built from. Bump when consumers of the schema need to react:
#   MAJOR: the envelope shape, $defs naming scheme, or $ref form changes in
#          ways that break existing validators (e.g. renaming
#          `component__<class>__<type>` to a different scheme).
#   MINOR: new validator-visible affordances added (e.g. new `x-*` extension
#          fields, new permissive-fallback semantics, new placeholder shapes).
#   PATCH: schema content updates that don't change the format (most common —
#          a new component appears in the manifest, an upstream schema is
#          refreshed, a shim is added or retired).
#
# This is hand-curated rather than derived from the inputs because the
# version's *meaning* (validator behaviour) doesn't map one-to-one onto input
# changes. Update this constant when changing the bundle format.
BUNDLE_SCHEMA_VERSION = "1.0.0"


def _manifest_collector_version() -> str:
    """Read `dist.version` from the OCB manifest. This is the upstream
    Collector version the bundle's component schemas correspond to —
    written into the bundle output so consumers (Fleet, IDE schema stores)
    can pin to compatible versions."""
    raw = yaml.safe_load(MANIFEST_PATH.read_text())
    version = (raw.get("dist") or {}).get("version")
    return f"v{version}" if version and not str(version).startswith("v") else (version or "unknown")


# ---------------------------------------------------------------------------
# ID generation
# ---------------------------------------------------------------------------

# Manifest section names map to the singular component class used in
# metadata.yaml (`status.class`) and our ID scheme.
SECTION_TO_CLASS = {
    "receivers": "receiver",
    "processors": "processor",
    "exporters": "exporter",
    "connectors": "connector",
    "extensions": "extension",
}

# Short-label prefixes for known repo namespaces, applied in `short_label`
# to compress IDs. Falls back to the full encoded path for anything else.
_NAMESPACE_LABEL_PREFIX = {
    "github.com/open-telemetry/opentelemetry-collector-contrib": "",
    "go.opentelemetry.io/collector": "core_",
    "github.com/DataDog/datadog-agent": "local_",
}

# Upstream shims: schemas that we ship in-repo ahead of an upstream PR merge.
# Each entry maps an upstream Go import path to a local directory (relative to
# the repo root) that contains a `config.schema.yaml` and (for full components)
# a `metadata.yaml`. The bundler treats the shim as if it lived in the cached
# upstream module at that go_path, so refs into / out of the shim resolve
# through the same namespace-relative + package-type machinery used for real
# upstream schemas. Removed once upstream merges the corresponding schema.
#
# Sources of these shims:
#   https://github.com/open-telemetry/opentelemetry-collector/pull/15300
# See tasks/libs/otelcol_schema/upstream_shims/README.md for details.
_UPSTREAM_SHIM_ROOT = "tasks/libs/otelcol_schema/upstream_shims"
UPSTREAM_SHIMS: dict[str, str] = {
    "go.opentelemetry.io/collector/exporter/debugexporter": f"{_UPSTREAM_SHIM_ROOT}/exporter_debugexporter",
    "go.opentelemetry.io/collector/exporter/otlpexporter": f"{_UPSTREAM_SHIM_ROOT}/exporter_otlpexporter",
    "go.opentelemetry.io/collector/exporter/otlphttpexporter": f"{_UPSTREAM_SHIM_ROOT}/exporter_otlphttpexporter",
    "go.opentelemetry.io/collector/receiver/otlpreceiver": f"{_UPSTREAM_SHIM_ROOT}/receiver_otlpreceiver",
    "go.opentelemetry.io/collector/processor/batchprocessor": f"{_UPSTREAM_SHIM_ROOT}/processor_batchprocessor",
    "go.opentelemetry.io/collector/processor/memorylimiterprocessor": f"{_UPSTREAM_SHIM_ROOT}/processor_memorylimiterprocessor",
    "go.opentelemetry.io/collector/extension/zpagesextension": f"{_UPSTREAM_SHIM_ROOT}/extension_zpagesextension",
    "go.opentelemetry.io/collector/internal/memorylimiter": f"{_UPSTREAM_SHIM_ROOT}/internal_memorylimiter",
}


def _shim_schema_path(go_path: str) -> Path | None:
    """Return the local shim's `config.schema.yaml` for an upstream go_path,
    or None if no shim is registered (or the registered shim is missing
    its schema file)."""
    rel = UPSTREAM_SHIMS.get(go_path)
    if rel is None:
        return None
    candidate = REPO_ROOT / rel / "config.schema.yaml"
    return candidate if candidate.is_file() else None


def _flatten(s: str) -> str:
    """Encode an arbitrary string as a snake_case identifier."""
    return s.replace("/", "_").replace(".", "_").replace("-", "_")


def _missing_id(ref: str) -> str:
    """Stable, collision-free placeholder ID for an unresolved ref.

    `_flatten` lossily collapses `/`, `.`, `-` to `_`, so two distinct refs
    can produce the same flattened stem. Append the first eight hex chars
    of SHA-1(ref) to keep the human-readable prefix while guaranteeing
    one-to-one mapping with the original ref string.
    """
    digest = hashlib.sha1(ref.encode("utf-8"), usedforsecurity=False).hexdigest()[:8]
    return f"__missing__{_flatten(ref)}__{digest}"


def short_label(go_path: str) -> str:
    """Compress a Go package path by stripping known repo prefixes."""
    for ns in KNOWN_NAMESPACES:
        if go_path == ns or go_path.startswith(ns + "/"):
            tail = go_path[len(ns) :].lstrip("/")
            return _NAMESPACE_LABEL_PREFIX[ns] + _flatten(tail)
    return _flatten(go_path)


def component_id(class_name: str, type_name: str, *, inner: str | None = None) -> str:
    base = f"component__{class_name}__{type_name}"
    if inner:
        return f"{base}__{inner}"
    return base


# ---------------------------------------------------------------------------
# Schema source records
# ---------------------------------------------------------------------------


@dataclass
class SchemaSource:
    """Everything we know about one schema file we've decided to bundle."""

    path: Path  # absolute path to the .yaml file
    go_path: str  # Go import path of the package the schema lives in
    doc: dict[str, Any]  # parsed YAML (will be mutated by the rewriter)
    component_class: str | None = None  # singular form when this is a registered component
    component_type: str | None = None  # metadata.yaml `type` for components
    refs_used: list[str] = field(default_factory=list)

    @property
    def is_component(self) -> bool:
        return self.component_class is not None and self.component_type is not None

    @property
    def prefix(self) -> str:
        """Canonical ID prefix shared by every type defined in this file."""
        if self.is_component:
            assert self.component_class and self.component_type
            return component_id(self.component_class, self.component_type)
        return f"shared__{short_label(self.go_path)}"


# ---------------------------------------------------------------------------
# Discovery
# ---------------------------------------------------------------------------


def _read_metadata(directory: Path) -> tuple[str | None, str | None]:
    """Return (type, class) from the directory's metadata.yaml, if present."""
    md = directory / "metadata.yaml"
    if not md.is_file():
        return None, None
    try:
        data = yaml.safe_load(md.read_text()) or {}
    except yaml.YAMLError:
        return None, None
    return data.get("type"), (data.get("status") or {}).get("class")


def _go_path_of_local_dir(directory: Path) -> str:
    """Approximate Go-import-path style identifier for a local repo dir.

    Used purely as an ID-generation seed when a local component has no
    metadata.yaml; component IDs prefer `component__<class>__<type>`.
    """
    rel = directory.relative_to(REPO_ROOT)
    return "github.com/DataDog/datadog-agent/" + str(rel)


@functools.cache
def _module_versions() -> Mapping[str, str]:
    """Module-version lookup combining manifest entries, the impl's go.mod,
    and go.sum (covers transitive deps not in `require`).

    Cached for the process lifetime — neither file changes mid-build, and
    re-parsing thousands of go.sum lines on every call is wasteful. Returned
    as a `MappingProxyType` so the cached value is read-only and accidental
    mutation by a caller can't corrupt subsequent builds in the same
    interpreter.
    """
    impl = MANIFEST_PATH.parent
    versions: dict[str, str] = {}
    for entries in parse_manifest(MANIFEST_PATH).values():
        for gomod, version in entries:
            if gomod and version:
                versions[gomod] = version
    for gomod, version in parse_go_mod_versions(impl / "go.mod").items():
        versions.setdefault(gomod, version)
    for gomod, version in parse_go_sum_versions(impl / "go.sum").items():
        versions.setdefault(gomod, version)
    return MappingProxyType(versions)


def _lookup_module_schema(
    go_path: str,
    *,
    cache: dict[str, Path | None],
    cache_root: Path,
    versions: Mapping[str, str],
) -> Path | None:
    """Memoised wrapper around `find_module_schema`. Consults UPSTREAM_SHIMS
    first so locally-shipped schemas override the cached upstream module
    (used to leverage in-flight upstream PRs ahead of merge). Returns just
    the schema path (callers needing the matched-module-dir distinction
    must call `find_module_schema` directly)."""
    if go_path in cache:
        return cache[go_path]
    shim = _shim_schema_path(go_path)
    if shim is not None:
        cache[go_path] = shim
        return shim
    schema, _ = find_module_schema(go_path, cache_root=cache_root, versions=versions)
    cache[go_path] = schema
    return schema


def _enqueue_refs(
    *,
    source_path: Path,
    source_go_path: str,
    refs: list[str],
    cache_root: Path,
    versions: Mapping[str, str],
    schema_cache: dict[str, Path | None],
    queue: deque[tuple[Path, str, str | None, str | None]],
    seen: set[Path],
) -> None:
    """Look at every ref a source emits and enqueue the target schema files
    we'd need to load to follow them. Module-schema lookups go through
    `schema_cache` so the same go_path is resolved once across discovery
    and rewriting."""
    for ref in refs:
        status = classify_ref(ref)
        if status.kind == "package_type" and status.target_module:
            target_path = _lookup_module_schema(
                status.target_module, cache=schema_cache, cache_root=cache_root, versions=versions
            )
            if target_path and target_path not in seen:
                queue.append((target_path, status.target_module, None, None))
        elif status.kind == "relative":
            target_path = follow_relative(source_path.parent, status.target_module)
            if target_path and target_path not in seen:
                sub_go_path = resolve_relative_go_path(source_go_path, status.target_module or "")
                queue.append((target_path, sub_go_path, None, None))
        elif status.kind == "namespace_relative":
            ns = repo_namespace_of(source_go_path)
            if ns is None:
                continue
            parsed = parse_namespace_relative(ref, ns)
            if parsed is None:
                continue
            target_module, _type_name = parsed
            target_path = _lookup_module_schema(
                target_module, cache=schema_cache, cache_root=cache_root, versions=versions
            )
            if target_path and target_path not in seen:
                queue.append((target_path, target_module, None, None))
        # `bare` refs are intra-file (handled by ID assignment); `uri` refs
        # are not followed.


def collect_schemas(
    *, schema_cache: dict[str, Path | None] | None = None
) -> tuple[list[SchemaSource], list[tuple[str, str, str]]]:
    """Discover and load every schema we need.

    Returns (sources, missing_components). Sources are in BFS order from
    component roots through transitive `$ref` deps.

    `schema_cache` is mutated in place: callers may pass a dict to share the
    module-schema lookups with the subsequent rewriting phase. When None, a
    transient dict is used and discarded.
    """
    if schema_cache is None:
        schema_cache = {}
    cache_root = gomodcache()
    manifest = parse_manifest(MANIFEST_PATH)
    versions = _module_versions()

    queue: deque[tuple[Path, str, str | None, str | None]] = deque()
    seen: set[Path] = set()
    sources: list[SchemaSource] = []
    missing: list[tuple[str, str, str]] = []

    # Manifest components. Shims (in-repo schemas that pre-empt an in-flight
    # upstream PR) take precedence over the cached module's schema, if any.
    for section, entries in manifest.items():
        cls = SECTION_TO_CLASS.get(section)
        for gomod, version in entries:
            if not (gomod and version):
                continue
            shim = _shim_schema_path(gomod)
            if shim is not None:
                schema_path = shim
                metadata_dir = shim.parent
            else:
                module_dir = module_cache_dir(cache_root, gomod, version)
                schema_path = module_dir / "config.schema.yaml"
                if not schema_path.is_file():
                    missing.append((cls or section, gomod, "no config.schema.yaml in module"))
                    continue
                metadata_dir = module_dir
            type_name, mclass = _read_metadata(metadata_dir)
            queue.append((schema_path, gomod, mclass or cls or "unknown", type_name))

    # Local components.
    for section, directory in LOCAL_SCHEMAS:
        schema_path = directory / "config.schema.yaml"
        if not schema_path.is_file():
            missing.append((SECTION_TO_CLASS.get(section, section), str(directory), "no config.schema.yaml in dir"))
            continue
        type_name, mclass = _read_metadata(directory)
        cls = mclass or SECTION_TO_CLASS.get(section, section)
        queue.append((schema_path, _go_path_of_local_dir(directory), cls, type_name))

    while queue:
        schema_path, go_path, cls, type_name = queue.popleft()
        if schema_path in seen:
            continue
        seen.add(schema_path)

        try:
            doc = yaml.safe_load(schema_path.read_text()) or {}
        except yaml.YAMLError:
            continue

        is_component = bool(cls and type_name)
        source = SchemaSource(
            path=schema_path,
            go_path=go_path,
            doc=doc,  # yaml.safe_load already returns a fresh tree
            component_class=cls if is_component else None,
            component_type=type_name if is_component else None,
            refs_used=sorted(walk_refs(doc)),
        )
        sources.append(source)

        _enqueue_refs(
            source_path=schema_path,
            source_go_path=go_path,
            refs=source.refs_used,
            cache_root=cache_root,
            versions=versions,
            schema_cache=schema_cache,
            queue=queue,
            seen=seen,
        )

    return sources, missing


# ---------------------------------------------------------------------------
# Canonical IDs
# ---------------------------------------------------------------------------


@dataclass
class IdMapping:
    """Per-source mapping from local type names to canonical bundle IDs."""

    root_id: str | None  # None for package-mode files (no root type)
    defs_ids: dict[str, str]  # `<defs key>` -> canonical id


def assign_ids(sources: list[SchemaSource]) -> dict[Path, IdMapping]:
    """Determine a canonical ID for the root and every `$defs` entry across
    all sources.

    Asserts that no two sources end up claiming the same ID. With the
    `component__<class>__<type>__<inner>` / `shared__<short-path>__<type>`
    scheme, collisions can only occur if two distinct go_paths compress
    to the same `short_label` — this would be a real bug, and the assert
    here surfaces it loudly rather than silently corrupting one schema.
    """
    mappings: dict[Path, IdMapping] = {}
    used: dict[str, Path] = {}

    def _claim(candidate: str, src_path: Path) -> str:
        owner = used.get(candidate)
        if owner is not None and owner != src_path:
            raise RuntimeError(f"canonical ID collision: {candidate!r} claimed by both {owner} and {src_path}")
        used[candidate] = src_path
        return candidate

    for src in sources:
        prefix = src.prefix
        root_id: str | None = None
        if is_component_mode(src.doc):
            root_id = _claim(prefix, src.path)

        defs_ids: dict[str, str] = {}
        for entry_name in src.doc.get("$defs") or {}:
            defs_ids[entry_name] = _claim(f"{prefix}__{entry_name}", src.path)

        mappings[src.path] = IdMapping(root_id=root_id, defs_ids=defs_ids)

    return mappings


# ---------------------------------------------------------------------------
# Ref rewriting
# ---------------------------------------------------------------------------


@dataclass
class _Resolver:
    """Bundles together everything `_canonicalise_ref` needs across a single
    bundle build. Reduces the kwarg-fanout in the recursive walker, and
    memoises `find_module_schema` results since the same module can be
    referenced from many sources during ref rewriting. The cache is
    typically shared with the discovery phase via `collect_schemas`."""

    sources_by_path: dict[Path, IdMapping]
    versions: Mapping[str, str]
    cache_root: Path
    unresolved: dict[str, list[str]]
    schema_cache: dict[str, Path | None] = field(default_factory=dict)

    def find_schema(self, go_path: str) -> Path | None:
        return _lookup_module_schema(
            go_path, cache=self.schema_cache, cache_root=self.cache_root, versions=self.versions
        )


def _resolve_in_mapping(mapping: IdMapping, type_name: str) -> str | None:
    """Look up `type_name` in a target schema's defs, falling back to its
    root for the conventional `config` name."""
    if type_name in mapping.defs_ids:
        return mapping.defs_ids[type_name]
    if type_name == "config" and mapping.root_id is not None:
        return mapping.root_id
    return None


def _record_unresolved(
    *,
    ref: str,
    source: SchemaSource,
    note: str,
    resolver: _Resolver,
) -> str:
    """Record an unresolved ref and return the placeholder JSON Pointer."""
    detail = f"used by {source.path.name}"
    if note:
        detail += f" ({note})"
    resolver.unresolved.setdefault(ref, []).append(detail)
    return f"#/$defs/{_missing_id(ref)}"


def _rewrite_refs(node: Any, *, source: SchemaSource, own_defs_ids: dict[str, str], resolver: _Resolver) -> None:
    """Walk `node` in-place, rewriting every `$ref` to a canonical JSON Pointer."""
    if isinstance(node, dict):
        for key, value in node.items():
            if key == "$ref" and isinstance(value, str):
                node[key] = _canonicalise_ref(value, source=source, own_defs_ids=own_defs_ids, resolver=resolver)
            else:
                _rewrite_refs(value, source=source, own_defs_ids=own_defs_ids, resolver=resolver)
    elif isinstance(node, list):
        for item in node:
            _rewrite_refs(item, source=source, own_defs_ids=own_defs_ids, resolver=resolver)


def _canonicalise_ref(ref: str, *, source: SchemaSource, own_defs_ids: dict[str, str], resolver: _Resolver) -> str:
    # Already a same-file JSON Pointer (M2's output).
    if ref.startswith("#/$defs/"):
        local_name = ref[len("#/$defs/") :]
        canonical = own_defs_ids.get(local_name)
        if canonical:
            return f"#/$defs/{canonical}"
        return _record_unresolved(ref=ref, source=source, note=f"no $defs entry {local_name!r}", resolver=resolver)

    if ref.startswith("#"):
        return ref  # other JSON Pointer / fragment -> pass through

    status = classify_ref(ref)

    if status.kind == "bare":
        canonical = own_defs_ids.get(ref)
        if canonical:
            return f"#/$defs/{canonical}"
        return _record_unresolved(ref=ref, source=source, note=f"no $defs entry {ref!r}", resolver=resolver)

    if status.kind == "package_type" and status.target_module:
        target_path = resolver.find_schema(status.target_module)
        if target_path and target_path in resolver.sources_by_path:
            resolved = _resolve_in_mapping(resolver.sources_by_path[target_path], status.target_type or "")
            if resolved:
                return f"#/$defs/{resolved}"
        return _record_unresolved(ref=ref, source=source, note="", resolver=resolver)

    if status.kind == "relative":
        target_path = follow_relative(source.path.parent, status.target_module)
        if target_path and target_path in resolver.sources_by_path:
            resolved = _resolve_in_mapping(resolver.sources_by_path[target_path], status.target_type or "")
            if resolved:
                return f"#/$defs/{resolved}"
        return _record_unresolved(ref=ref, source=source, note="", resolver=resolver)

    if status.kind == "namespace_relative":
        ns = repo_namespace_of(source.go_path)
        if ns is not None:
            parsed = parse_namespace_relative(ref, ns)
            if parsed is not None:
                target_module, type_name = parsed
                target_path = resolver.find_schema(target_module)
                if target_path and target_path in resolver.sources_by_path:
                    resolved = _resolve_in_mapping(resolver.sources_by_path[target_path], type_name)
                    if resolved:
                        return f"#/$defs/{resolved}"
        return _record_unresolved(ref=ref, source=source, note="", resolver=resolver)

    # uri / unknown: pass through (the meta-schema accepts external URI
    # references; consumers may or may not resolve them).
    return ref


# ---------------------------------------------------------------------------
# Bundle assembly
# ---------------------------------------------------------------------------


@dataclass
class BundleResult:
    bundle: dict[str, Any]
    sources: list[SchemaSource]
    unresolved_refs: dict[str, list[str]]
    missing_components: list[tuple[str, str, str]]


def _entry_for_root(doc: dict[str, Any]) -> dict[str, Any]:
    """Strip `$defs` (lifted to bundle level) but keep all other root fields."""
    return {k: v for k, v in doc.items() if k != "$defs"}


def _placeholder() -> dict[str, Any]:
    return {"type": "object", "additionalProperties": True}


def ensure_manifest_modules_downloaded() -> list[tuple[str, str]]:
    """Walk every manifest entry and `go mod download` any module that isn't
    already in `gomodcache`. Returns a list of `(<gomod>@<version>, error)`
    tuples for downloads that failed.

    A fresh clone has none of the OCB manifest's pinned modules in cache.
    Without this pre-flight, `build_bundle` silently treats every upstream
    component as "missing schema" and the resulting bundle is degraded into
    permissive-fallback shape — a footgun for first-time contributors
    running `dda inv otelcol-schema.{gen,check}`.
    """
    cache_root = gomodcache()
    impl_dir = MANIFEST_PATH.parent
    failures: list[tuple[str, str]] = []
    for entries in parse_manifest(MANIFEST_PATH).values():
        for gomod, version in entries:
            if not (gomod and version):
                continue
            if module_cache_dir(cache_root, gomod, version).is_dir():
                continue
            ok, msg = ensure_downloaded(gomod, version, cwd=impl_dir)
            if not ok:
                failures.append((f"{gomod}@{version}", msg))
    return failures


def build_bundle(*, missing_strategy: str = "permissive") -> BundleResult:
    if missing_strategy not in ("permissive", "strict"):
        raise ValueError(f"unknown missing strategy: {missing_strategy!r}")

    # Share the schema cache across discovery and rewriting: the same target
    # module is often referenced from both phases, so resolving once saves
    # repeated prefix walks over the module cache.
    schema_cache: dict[str, Path | None] = {}
    sources, missing_components = collect_schemas(schema_cache=schema_cache)
    mappings = assign_ids(sources)
    resolver = _Resolver(
        sources_by_path=mappings,
        versions=_module_versions(),
        cache_root=gomodcache(),
        unresolved={},
        schema_cache=schema_cache,
    )

    for src in sources:
        own_defs_ids = mappings[src.path].defs_ids
        _rewrite_refs(src.doc, source=src, own_defs_ids=own_defs_ids, resolver=resolver)

    defs: dict[str, Any] = {}
    for src in sources:
        mapping = mappings[src.path]
        if mapping.root_id is not None:
            defs[mapping.root_id] = _entry_for_root(src.doc)
        for entry_name, canonical_id in mapping.defs_ids.items():
            defs[canonical_id] = (src.doc.get("$defs") or {}).get(entry_name, {})

    if missing_strategy == "strict" and resolver.unresolved:
        msg = "; ".join(f"{ref}: {', '.join(reasons)}" for ref, reasons in resolver.unresolved.items())
        raise RuntimeError(f"strict mode: unresolved refs: {msg}")

    # permissive: insert placeholders so internal refs resolve. The
    # placeholder ID is a 1:1 hash over the original ref, so distinct
    # unresolved refs never share a $defs entry.
    for ref in resolver.unresolved:
        placeholder_id = _missing_id(ref)
        if placeholder_id not in defs:
            defs[placeholder_id] = _placeholder()
    # Invariant: one placeholder per unresolved ref. Raised (not asserted)
    # so the check survives `python -O`.
    placeholder_count = sum(1 for k in defs if k.startswith("__missing__"))
    if placeholder_count != len(resolver.unresolved):
        raise RuntimeError(f"placeholder count {placeholder_count} != unresolved-refs {len(resolver.unresolved)}")

    envelope = _stitch_envelope(sources, mappings, missing_components, missing_strategy=missing_strategy)
    if missing_strategy == "permissive":
        # The fallback placeholder for unmodeled components needs a $defs entry
        # to point at; reuse the same shape we use for unresolved $refs.
        defs.setdefault("__component__permissive__", _placeholder())

    # Identity + version metadata. These ride at the top of the bundle so
    # consumers (Fleet validator, IDE schema stores, doc generators) can
    # discover what they're looking at without parsing the file's body.
    # `x-*` extension fields are spec-valid JSON Schema (unknown keywords
    # are ignored by validators) and survive meta-schema validation.
    bundle = {
        "$schema": JSON_SCHEMA_DRAFT,
        "$id": BUNDLE_ID,
        "title": BUNDLE_TITLE,
        "x-bundle-version": BUNDLE_SCHEMA_VERSION,
        "x-collector-version": _manifest_collector_version(),
        **envelope,
        "$defs": defs,
    }
    return BundleResult(
        bundle=bundle,
        sources=sources,
        unresolved_refs=resolver.unresolved,
        missing_components=missing_components,
    )


# ---------------------------------------------------------------------------
# Envelope stitching (M4)
# ---------------------------------------------------------------------------


_ENVELOPE_PATH = Path(__file__).parent / "envelope.yaml"

# Collector class -> envelope key. Class is the singular form used inside our
# `component__<class>__<type>` IDs (matching metadata.yaml `status.class`);
# the envelope keys are the conventional plural names.
_ENVELOPE_KEY_FOR_CLASS = {
    "receiver": "receivers",
    "processor": "processors",
    "exporter": "exporters",
    "connector": "connectors",
    "extension": "extensions",
}


def _stitch_envelope(
    sources: list[SchemaSource],
    mappings: dict[Path, IdMapping],
    missing_components: list[tuple[str, str, str]],
    *,
    missing_strategy: str,
) -> dict[str, Any]:
    """Load the hand-authored envelope and fill its `patternProperties` lists
    with one entry per registered component type, pointing at the canonical
    `$defs` ID.

    Under `permissive` strategy, also adds a class-wide fallback so component
    types whose schema we couldn't bundle (no `config.schema.yaml` published
    upstream — see `missing_components`) still validate. The fallback uses
    `^.*$` ranked last by pattern matching, pointing at a permissive
    placeholder that accepts any object body.
    """
    envelope = yaml.safe_load(_ENVELOPE_PATH.read_text())

    # Group components by envelope key.
    by_key: dict[str, list[tuple[str, str]]] = defaultdict(list)
    for src in sources:
        if not src.is_component or src.component_class is None or src.component_type is None:
            continue
        key = _ENVELOPE_KEY_FOR_CLASS.get(src.component_class)
        if key is None:
            continue
        canonical = mappings[src.path].root_id
        if canonical is None:
            continue
        by_key[key].append((src.component_type, canonical))

    # Fill patternProperties for each component class.
    for key, entries in by_key.items():
        pattern_props = envelope["properties"][key].setdefault("patternProperties", {})
        for type_name, canonical in sorted(entries):
            pattern = f"^{re.escape(type_name)}(/.*)?$"
            pattern_props[pattern] = {"$ref": f"#/$defs/{canonical}"}

    # Permissive fallback for component classes that have at least one entry
    # in `missing_components`: accept any unmodeled component body without
    # complaint. Strict mode skips this — unmodeled types fail validation.
    if missing_strategy == "permissive":
        classes_with_gaps = {cls for cls, _src, _reason in missing_components}
        for cls in classes_with_gaps:
            key = _ENVELOPE_KEY_FOR_CLASS.get(cls)
            if key is None or key not in envelope["properties"]:
                continue
            pattern_props = envelope["properties"][key].setdefault("patternProperties", {})
            pattern_props.setdefault("^.*$", {"$ref": "#/$defs/__component__permissive__"})

    return {k: v for k, v in envelope.items() if k != "$schema"}


# ---------------------------------------------------------------------------
# Reporting
# ---------------------------------------------------------------------------


def write_report(result: BundleResult, out: Path) -> None:
    lines: list[str] = []
    lines.append("# DDOT Collector schema bundle — M3 build report\n")
    lines.append(f"- Sources bundled: **{len(result.sources)}**")
    lines.append(f"- `$defs` entries: **{len(result.bundle.get('$defs') or {})}**")
    lines.append(f"- Unresolved refs (placeholders inserted): **{len(result.unresolved_refs)}**")
    lines.append(f"- Components missing schema: **{len(result.missing_components)}**\n")

    by_class: dict[str, list[SchemaSource]] = defaultdict(list)
    for src in result.sources:
        if src.is_component:
            assert src.component_class
            by_class[src.component_class].append(src)
    if by_class:
        lines.append("## Components included\n")
        lines.append("| class | type | go path |")
        lines.append("|---|---|---|")
        for cls in sorted(by_class):
            for src in sorted(by_class[cls], key=lambda s: s.component_type or ""):
                lines.append(f"| {cls} | `{src.component_type}` | `{src.go_path}` |")
        lines.append("")

    shared = [s for s in result.sources if not s.is_component]
    if shared:
        lines.append("## Shared types (transitive)\n")
        lines.append("| go path | $defs entries |")
        lines.append("|---|---:|")
        for src in sorted(shared, key=lambda s: s.go_path):
            n = len(src.doc.get("$defs") or {})
            lines.append(f"| `{src.go_path}` | {n} |")
        lines.append("")

    if result.missing_components:
        lines.append("## Components missing schemas\n")
        lines.append("| class | source | reason |")
        lines.append("|---|---|---|")
        for cls, src, reason in sorted(result.missing_components):
            lines.append(f"| {cls} | `{src}` | {reason} |")
        lines.append("")

    if result.unresolved_refs:
        lines.append("## Unresolved refs (permissive placeholders)\n")
        lines.append("| ref | used by |")
        lines.append("|---|---|")
        for ref in sorted(result.unresolved_refs):
            users = "; ".join(sorted(set(result.unresolved_refs[ref])))
            lines.append(f"| `{ref}` | {users} |")
        lines.append("")

    out.write_text("\n".join(lines) + "\n")


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0] if __doc__ else "")
    parser.add_argument(
        "--out",
        default="otelcol-schema-bundle.json",
        type=Path,
        help="output path for the bundled JSON Schema",
    )
    parser.add_argument("--report", type=Path, help="optional markdown summary path")
    parser.add_argument(
        "--missing",
        choices=["permissive", "strict"],
        default="permissive",
        help="how to handle unresolved $ref targets (default: permissive)",
    )
    args = parser.parse_args(argv)

    try:
        result = build_bundle(missing_strategy=args.missing)
    except RuntimeError as e:
        print(str(e), file=sys.stderr)
        return 2

    args.out.write_text(json.dumps(result.bundle, indent=2) + "\n")

    errors = validate_meta(result.bundle)
    for err in errors:
        print(f"meta-schema error: {err}", file=sys.stderr)

    if args.report is not None:
        write_report(result, args.report)

    print(
        f"wrote {args.out} "
        f"(sources={len(result.sources)}, "
        f"$defs={len(result.bundle.get('$defs') or {})}, "
        f"unresolved={len(result.unresolved_refs)}, "
        f"missing_components={len(result.missing_components)})",
        file=sys.stderr,
    )

    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
