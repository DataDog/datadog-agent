"""Shared helpers for the otelcol_schema tools.

Holds the small primitives that inventory.py, convert.py and bundle.py
all need: walking `$ref`s, classifying their shapes, locating the Go
module that contains a referenced schema, and resolving namespace-
relative refs to a full Go import path.

Kept dependency-free of the higher-level modules so it can be imported
from any of them without cycles.
"""

from __future__ import annotations

import re
from collections.abc import Mapping
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

# ---------------------------------------------------------------------------
# Known repo namespaces
# ---------------------------------------------------------------------------

# In order of specificity, used to resolve namespace-relative refs back
# to a full Go package path based on the source schema's location, and
# to compress IDs in `short_label`.
KNOWN_NAMESPACES: tuple[str, ...] = (
    "github.com/open-telemetry/opentelemetry-collector-contrib",
    "go.opentelemetry.io/collector",
    "github.com/DataDog/datadog-agent",
)


def repo_namespace_of(go_path: str) -> str | None:
    """Return the repo namespace prefix that contains a given Go path."""
    for ns in KNOWN_NAMESPACES:
        if go_path == ns or go_path.startswith(ns + "/"):
            return ns
    return None


# ---------------------------------------------------------------------------
# Walking $refs
# ---------------------------------------------------------------------------


def walk_refs(node: Any, acc: set[str] | None = None) -> set[str]:
    """Recursively collect every `$ref` string in a parsed schema document."""
    if acc is None:
        acc = set()
    if isinstance(node, dict):
        for k, v in node.items():
            if k == "$ref" and isinstance(v, str):
                acc.add(v)
            else:
                walk_refs(v, acc)
    elif isinstance(node, list):
        for item in node:
            walk_refs(item, acc)
    return acc


# ---------------------------------------------------------------------------
# Classifying refs
# ---------------------------------------------------------------------------


@dataclass
class RefStatus:
    ref: str
    kind: str  # "uri" | "namespace_relative" | "package_type" | "relative" | "bare" | "unknown"
    target_module: str | None = None  # Go import path or relative dir of the schema we'd look in
    target_type: str | None = None  # snake_case type name within that schema
    resolved: bool = False
    schema_path: str | None = None  # absolute path of the resolving file
    used_by: list[str] = field(default_factory=list)
    note: str = ""


def classify_ref(ref: str) -> RefStatus:
    if "://" in ref:
        return RefStatus(ref=ref, kind="uri")

    if ref.startswith("./") or ref.startswith("../"):
        # Schemagen's relative form for refs into a sibling sub-package of
        # the source schema's own module. Resolved against the schema file's
        # directory rather than against any module root.
        idx = ref.rfind(".")
        return RefStatus(
            ref=ref,
            kind="relative",
            target_module=ref[:idx],
            target_type=ref[idx + 1 :],
        )

    if ref.startswith("/"):
        # Upstream-style namespace-relative form: resolved against the
        # repo namespace of the source schema's own location.
        return RefStatus(ref=ref, kind="namespace_relative")

    # `<package_path>.<snake_type>` form: package path contains '/' so we know
    # it's not a bare same-file ref. Split on the rightmost '.'.
    if "/" in ref and "." in ref:
        idx = ref.rfind(".")
        return RefStatus(ref=ref, kind="package_type", target_module=ref[:idx], target_type=ref[idx + 1 :])

    if "." not in ref and "/" not in ref:
        return RefStatus(ref=ref, kind="bare")

    return RefStatus(ref=ref, kind="unknown")


# ---------------------------------------------------------------------------
# Namespace-relative parsing
# ---------------------------------------------------------------------------


def parse_namespace_relative(ref: str, source_namespace: str) -> tuple[str, str] | None:
    """Convert a `/<rel>.<type>` ref into `(<full Go path>, <type>)`.

    Returns None if the ref is malformed (no separating '.', or the part
    before '.' is empty). Caller is responsible for checking the result
    actually exists in the cache.
    """
    if not ref.startswith("/"):
        return None
    rel_and_type = ref[1:]
    idx = rel_and_type.rfind(".")
    if idx <= 0:
        return None
    rel = rel_and_type[:idx]
    type_name = rel_and_type[idx + 1 :]
    return f"{source_namespace}/{rel}", type_name


# ---------------------------------------------------------------------------
# Module-cache lookup
# ---------------------------------------------------------------------------


def encode_module_path(path: str) -> str:
    """Apply Go's module-cache path encoding.

    Each uppercase letter becomes `!<lowercase>`. So
    `github.com/DataDog/foo` -> `github.com/!data!dog/foo`.
    """
    return "".join("!" + ch.lower() if ch.isupper() else ch for ch in path)


def module_cache_dir(cache_root: Path, gomod: str, version: str) -> Path:
    return cache_root / f"{encode_module_path(gomod)}@{version}"


def find_module_schema(
    go_path: str, *, cache_root: Path, versions: Mapping[str, str]
) -> tuple[Path | None, Path | None]:
    """Walk module-path prefixes until we find one with a config.schema.yaml.

    Returns `(schema_path_or_None, matched_module_dir_or_None)`. The
    matched-module path lets callers distinguish "no version known" from
    "module cached but no schema generated upstream".
    """
    parts = go_path.split("/")
    matched_module: Path | None = None
    for i in range(len(parts), 0, -1):
        prefix = "/".join(parts[:i])
        version = versions.get(prefix)
        if not version:
            continue
        module_dir = module_cache_dir(cache_root, prefix, version)
        if matched_module is None and module_dir.is_dir():
            matched_module = module_dir
        rel = "/".join(parts[i:])
        candidate = module_dir / rel / "config.schema.yaml" if rel else module_dir / "config.schema.yaml"
        if candidate.is_file():
            return candidate, module_dir
    return None, matched_module


# ---------------------------------------------------------------------------
# go.mod / go.sum parsing
# ---------------------------------------------------------------------------


GOMOD_REQUIRE_LINE = re.compile(r"^\s*(\S+)\s+(v\S+)\s*(?://.*)?$")
GOSUM_LINE = re.compile(r"^(\S+)\s+(v\S+?)(?:/go\.mod)?\s+h1:")


def parse_go_sum_versions(go_sum: Path) -> dict[str, str]:
    """Extract `<module> -> <version>` from go.sum, taking the first observed
    version per module. go.sum lists every transitive dep, including ones
    go.mod's `require` block doesn't restate."""
    if not go_sum.is_file():
        return {}
    out: dict[str, str] = {}
    for line in go_sum.read_text().splitlines():
        m = GOSUM_LINE.match(line)
        if m:
            out.setdefault(m.group(1), m.group(2))
    return out


def parse_go_mod_versions(go_mod: Path) -> dict[str, str]:
    """Extract `<module> <version>` pairs from `require` blocks in go.mod."""
    if not go_mod.is_file():
        return {}
    out: dict[str, str] = {}
    in_require = False
    for line in go_mod.read_text().splitlines():
        s = line.strip()
        if s.startswith("require ("):
            in_require = True
            continue
        if in_require and s == ")":
            in_require = False
            continue
        if s.startswith("require ") and "(" not in s:
            s = s[len("require ") :]
            m = GOMOD_REQUIRE_LINE.match(s)
            if m:
                out[m.group(1)] = m.group(2)
            continue
        if in_require:
            m = GOMOD_REQUIRE_LINE.match(s)
            if m:
                out[m.group(1)] = m.group(2)
    return out


# ---------------------------------------------------------------------------
# Schema-shape helpers
# ---------------------------------------------------------------------------


def schema_contains_type(doc: dict[str, Any], type_name: str) -> bool:
    """Whether `<type_name>` resolves against this schema document.

    Resolution targets:
      - explicit entry under the document's `$defs`, or
      - the document's root, when the file is a component-mode schema
        (has root `properties`/`allOf`) and the requested name is the
        conventional `config`.
    """
    if type_name in (doc.get("$defs") or {}):
        return True
    has_root_props = isinstance(doc.get("properties"), dict) or "allOf" in doc
    return bool(has_root_props and type_name == "config")


def is_component_mode(doc: dict[str, Any]) -> bool:
    """Schemagen's component-mode files put the primary type at the root
    (with `type`/`properties`/`allOf`); package-mode files have only `$defs`.

    Tests for *presence* of these keys, not truthiness — an empty
    `properties: {}` still indicates component mode.
    """
    return any(key in doc for key in ("type", "properties", "allOf"))


# ---------------------------------------------------------------------------
# Relative-ref resolution
# ---------------------------------------------------------------------------


def follow_relative(schema_dir: Path, target_module: str | None) -> Path | None:
    """Resolve a `./<rel>` or `../<rel>` ref's target dir, then point at that
    dir's `config.schema.yaml`. Returns the path if the file exists, else
    None. Handles `..` correctly via `Path.resolve()`."""
    if not target_module:
        return None
    rel = target_module
    if rel.startswith("./"):
        rel = rel[2:]
    candidate = (schema_dir / rel / "config.schema.yaml").resolve()
    return candidate if candidate.is_file() else None


def resolve_relative_go_path(source_go_path: str, rel: str) -> str:
    """Apply Go-import-path semantics to a relative ref string.

    Mirrors `Path.resolve()` for go_path strings: `./sub` adds segments,
    `../` pops one. The earlier `lstrip("./").replace("..", "")` shortcut
    silently turned `../sibling` into `sibling`, producing a synthetic
    go_path that pointed back into the source instead of up-and-over.

    Examples:
        ("a/b", "./internal/foo") -> "a/b/internal/foo"
        ("a/b", "../sibling")     -> "a/sibling"
        ("a/b", "./.")            -> "a/b"
    """
    parts = source_go_path.split("/") if source_go_path else []
    if rel.startswith("./"):
        rel = rel[2:]
    for segment in rel.split("/"):
        if segment in ("", "."):
            continue
        if segment == "..":
            if parts:
                parts.pop()
            continue
        parts.append(segment)
    return "/".join(parts)
