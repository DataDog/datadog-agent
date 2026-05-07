"""M1 inventory pass for the DDOT Collector configuration JSON Schema bundle.

Produces a report describing, for every Collector component the bundle
will eventually need to cover, whether a `config.schema.yaml` is
available and what `$ref` targets the schemas reach. See
`docs/dev/plans/2026-05-07-otelcol-config-schema-plan.md` for context.

Run as:

    python -m tasks.libs.otelcol_schema.inventory \\
        [--json <path>] [--md <path>] [--no-go-mod-download]

With no flags, JSON and markdown reports are written under the current
working directory as `otelcol-schema-inventory.{json,md}`.
"""

from __future__ import annotations

import argparse
import json
import re
import shutil
import subprocess
import sys
from collections import defaultdict
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any

import yaml

# ---------------------------------------------------------------------------
# Repo + filesystem layout
# ---------------------------------------------------------------------------


def repo_root() -> Path:
    """Walk up from this file until we find the .git directory."""
    here = Path(__file__).resolve()
    for parent in (here, *here.parents):
        if (parent / ".git").exists():
            return parent
    raise RuntimeError(f"could not locate .git starting from {here}")


REPO_ROOT = repo_root()
MANIFEST_PATH = REPO_ROOT / "comp" / "otelcol" / "collector-contrib" / "impl" / "manifest.yaml"
LOCAL_OTELCOL = REPO_ROOT / "comp" / "otelcol"


# ---------------------------------------------------------------------------
# Go module cache
# ---------------------------------------------------------------------------


def gomodcache() -> Path:
    """Run `go env GOMODCACHE` and return the resolved path."""
    out = subprocess.run(
        ["go", "env", "GOMODCACHE"],
        check=True,
        capture_output=True,
        text=True,
    )
    return Path(out.stdout.strip())


def encode_module_path(path: str) -> str:
    """Apply Go's module-cache path encoding.

    Each uppercase letter becomes `!<lowercase>`. So
    `github.com/DataDog/foo` -> `github.com/!data!dog/foo`.
    """
    return "".join("!" + ch.lower() if ch.isupper() else ch for ch in path)


def module_cache_dir(cache_root: Path, gomod: str, version: str) -> Path:
    return cache_root / f"{encode_module_path(gomod)}@{version}"


def ensure_downloaded(gomod: str, version: str, *, cwd: Path) -> tuple[bool, str]:
    """`go mod download <gomod>@<version>` from `cwd`. Returns (ok, message)."""
    spec = f"{gomod}@{version}"
    proc = subprocess.run(
        ["go", "mod", "download", "-x", spec],
        cwd=cwd,
        capture_output=True,
        text=True,
    )
    if proc.returncode == 0:
        return True, ""
    return False, (proc.stderr or proc.stdout).strip().splitlines()[-1] if (
        proc.stderr or proc.stdout
    ).strip() else f"exit {proc.returncode}"


# ---------------------------------------------------------------------------
# Manifest parsing
# ---------------------------------------------------------------------------


# YAML in the manifest sometimes line-wraps the value, e.g.
#   - gomod: github.com/foo/bar
#       v0.151.0
# which loads as a single string `github.com/foo/bar v0.151.0`.
GOMOD_LINE = re.compile(r"^\s*(\S+)\s+(\S+)\s*$")

# Manifest section names whose entries describe configurable components.
COMPONENT_SECTIONS = ("receivers", "processors", "exporters", "connectors", "extensions")


def parse_manifest(path: Path) -> dict[str, list[tuple[str, str]]]:
    """Return {section: [(gomod, version), ...]} for component sections only."""
    raw = yaml.safe_load(path.read_text())
    out: dict[str, list[tuple[str, str]]] = {}
    for section in COMPONENT_SECTIONS:
        entries = raw.get(section) or []
        parsed: list[tuple[str, str]] = []
        for entry in entries:
            value = entry.get("gomod", "").strip()
            m = GOMOD_LINE.match(value)
            if not m:
                # Some entries might not have a version pinned (locally replaced).
                parsed.append((value, ""))
                continue
            parsed.append((m.group(1), m.group(2)))
        out[section] = parsed
    return out


# ---------------------------------------------------------------------------
# Component records
# ---------------------------------------------------------------------------


@dataclass
class Component:
    source: str  # "manifest" or "local"
    section: str  # "receivers" | "processors" | ...
    gomod: str | None  # None for local
    version: str | None  # None for local
    module_dir: str | None  # absolute path; None if not locatable
    in_cache: bool
    metadata_path: str | None
    metadata_type: str | None
    metadata_class: str | None
    schema_path: str | None
    schema_present: bool
    refs_used: list[str] = field(default_factory=list)
    notes: list[str] = field(default_factory=list)


# Local components we ship with their own config.schema.yaml. Identified by
# file location relative to the repo root.
LOCAL_SCHEMAS = [
    ("extensions", LOCAL_OTELCOL / "ddflareextension" / "impl"),
    ("extensions", LOCAL_OTELCOL / "ddprofilingextension" / "impl"),
    ("extensions", LOCAL_OTELCOL / "dogtelextension" / "impl"),
    ("exporters", LOCAL_OTELCOL / "otlp" / "components" / "exporter" / "logsagentexporter"),
    ("exporters", LOCAL_OTELCOL / "otlp" / "components" / "exporter" / "serializerexporter"),
    ("processors", LOCAL_OTELCOL / "otlp" / "components" / "processor" / "infraattributesprocessor"),
]


def load_component_from_dir(
    directory: Path, *, source: str, section: str, gomod: str | None, version: str | None
) -> Component:
    metadata_path = directory / "metadata.yaml"
    schema_path = directory / "config.schema.yaml"

    metadata_type = metadata_class = None
    if metadata_path.is_file():
        try:
            md = yaml.safe_load(metadata_path.read_text()) or {}
            metadata_type = md.get("type")
            metadata_class = (md.get("status") or {}).get("class")
        except yaml.YAMLError as e:  # noqa: PERF203
            metadata_type = metadata_class = None  # noqa: F841
            return Component(
                source=source,
                section=section,
                gomod=gomod,
                version=version,
                module_dir=str(directory),
                in_cache=True,
                metadata_path=str(metadata_path),
                metadata_type=None,
                metadata_class=None,
                schema_path=None,
                schema_present=False,
                notes=[f"metadata.yaml parse error: {e}"],
            )

    refs: list[str] = []
    if schema_path.is_file():
        try:
            schema = yaml.safe_load(schema_path.read_text()) or {}
            refs = sorted(walk_refs(schema))
        except yaml.YAMLError as e:
            return Component(
                source=source,
                section=section,
                gomod=gomod,
                version=version,
                module_dir=str(directory),
                in_cache=True,
                metadata_path=str(metadata_path) if metadata_path.is_file() else None,
                metadata_type=metadata_type,
                metadata_class=metadata_class,
                schema_path=str(schema_path),
                schema_present=False,
                notes=[f"config.schema.yaml parse error: {e}"],
            )

    return Component(
        source=source,
        section=section,
        gomod=gomod,
        version=version,
        module_dir=str(directory),
        in_cache=True,
        metadata_path=str(metadata_path) if metadata_path.is_file() else None,
        metadata_type=metadata_type,
        metadata_class=metadata_class,
        schema_path=str(schema_path) if schema_path.is_file() else None,
        schema_present=schema_path.is_file(),
        refs_used=refs,
    )


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
# $ref classification + resolution
# ---------------------------------------------------------------------------


@dataclass
class RefStatus:
    ref: str
    kind: str  # "uri" | "namespace_relative" | "package_type" | "bare" | "unknown"
    target_module: str | None = None  # Go import path of the schema we'd look in
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
        # Upstream-style namespace-relative form: assume the ref's namespace
        # is the repo it was generated in. We can't resolve these without
        # knowing that namespace, so mark them and move on.
        return RefStatus(ref=ref, kind="namespace_relative")

    # `<package_path>.<snake_type>` form: package path contains '/' so we know
    # it's not a bare same-file ref. Split on the rightmost '.'.
    if "/" in ref and "." in ref:
        idx = ref.rfind(".")
        pkg = ref[:idx]
        type_name = ref[idx + 1 :]
        return RefStatus(ref=ref, kind="package_type", target_module=pkg, target_type=type_name)

    if "." not in ref and "/" not in ref:
        return RefStatus(ref=ref, kind="bare")

    return RefStatus(ref=ref, kind="unknown")


GOMOD_REQUIRE_LINE = re.compile(r"^\s*(\S+)\s+(v\S+)\s*(?://.*)?$")
GOSUM_LINE = re.compile(r"^(\S+)\s+(v\S+?)(?:/go\.mod)?\s+h1:")


def parse_go_sum_versions(go_sum: Path) -> dict[str, str]:
    """Extract `<module> -> <version>` from go.sum, taking any version seen.
    go.sum lists every transitive dep, including the ones go.mod's `require`
    block doesn't restate. Multiple versions may appear for one module; we
    keep the first observed since that's typically the resolved one."""
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


def schema_contains_type(doc: dict[str, Any], type_name: str) -> bool:
    """A ref `<path>.<type>` resolves if `<type>` is in the file's `$defs`,
    or if the file is a component-mode schema (has root `properties`) and
    `<type>` matches the root — by convention `config`, but we also accept
    when no `$defs` are present and the file has root properties (the
    schemagen component-mode shape)."""
    if type_name in (doc.get("$defs") or {}):
        return True
    has_root_props = isinstance(doc.get("properties"), dict) or "allOf" in doc
    if has_root_props and type_name == "config":
        return True
    return False


def resolve_relative(
    status: RefStatus,
    *,
    consumers: list[Component],
) -> None:
    """Resolve a `./<rel>.<type>` ref against the source schema's own module."""
    assert status.kind == "relative"
    rel_path = (status.target_module or "").lstrip("./")
    type_name = status.target_type or ""

    sibling_dir_exists = False
    for consumer in consumers:
        if not consumer.schema_path:
            continue
        schema_dir = Path(consumer.schema_path).parent
        candidate = schema_dir / rel_path / "config.schema.yaml"
        if (schema_dir / rel_path).is_dir():
            sibling_dir_exists = True
        if candidate.is_file():
            try:
                doc = yaml.safe_load(candidate.read_text()) or {}
            except yaml.YAMLError as e:
                status.note = f"parse error: {e}"
                return
            if schema_contains_type(doc, type_name):
                status.resolved = True
                status.schema_path = str(candidate)
                return
            status.note = f"file present, type {type_name!r} not found"
            status.schema_path = str(candidate)
            return
    if sibling_dir_exists:
        status.note = "sibling dir cached but no config.schema.yaml generated upstream"
    else:
        status.note = "sibling dir does not exist at the resolved path"


def resolve_package_type(
    status: RefStatus,
    *,
    cache_root: Path,
    module_versions: dict[str, str],
) -> None:
    """Walk up the package path looking for a containing module that has a
    config.schema.yaml. If found, look up the type in its $defs."""
    assert status.kind == "package_type"
    pkg = status.target_module or ""
    type_name = status.target_type or ""

    # Try progressively shorter prefixes — the schema lives at the package
    # level it was generated for, which may be the leaf or a parent.
    parts = pkg.split("/")
    matched_module: Path | None = None
    for i in range(len(parts), 0, -1):
        candidate = "/".join(parts[:i])
        version = module_versions.get(candidate)
        if not version:
            continue
        module_dir = module_cache_dir(cache_root, candidate, version)
        rel = "/".join(parts[i:])
        schema_file = module_dir / rel / "config.schema.yaml" if rel else module_dir / "config.schema.yaml"
        if matched_module is None and module_dir.is_dir():
            matched_module = module_dir
        if schema_file.is_file():
            try:
                doc = yaml.safe_load(schema_file.read_text()) or {}
            except yaml.YAMLError as e:
                status.note = f"parse error: {e}"
                return
            if schema_contains_type(doc, type_name):
                status.resolved = True
                status.schema_path = str(schema_file)
            else:
                status.note = f"file present, type {type_name!r} not found"
                status.schema_path = str(schema_file)
            return
    if matched_module is not None:
        status.note = "module cached but no config.schema.yaml generated upstream"
    else:
        status.note = "no version known for any prefix of the package path"


# ---------------------------------------------------------------------------
# Driver
# ---------------------------------------------------------------------------


def collect(do_download: bool) -> tuple[list[Component], dict[str, RefStatus], dict[str, str]]:
    """Returns (components, ref_index_by_ref, manifest_module_versions)."""
    cache_root = gomodcache()

    manifest_sections = parse_manifest(MANIFEST_PATH)
    # Build a lookup of every module version mentioned in the manifest...
    module_versions: dict[str, str] = {}
    for entries in manifest_sections.values():
        for gomod, version in entries:
            if gomod and version:
                module_versions[gomod] = version
    # ...plus dependency versions from the OCB consumer's go.mod, so that
    # refs into shared upstream packages (confighttp, exporterhelper, the
    # contrib pkg/datadog/config helper, etc.) resolve.
    impl_dir = MANIFEST_PATH.parent
    for gomod, version in parse_go_mod_versions(impl_dir / "go.mod").items():
        module_versions.setdefault(gomod, version)
    # go.sum is the only place we see fully-transitive deps reliably.
    for gomod, version in parse_go_sum_versions(impl_dir / "go.sum").items():
        module_versions.setdefault(gomod, version)

    components: list[Component] = []
    download_cwd = MANIFEST_PATH.parent  # has go.mod; safe place to run go mod download

    for section, entries in manifest_sections.items():
        for gomod, version in entries:
            if not gomod:
                continue
            module_dir = module_cache_dir(cache_root, gomod, version) if version else None
            in_cache = bool(module_dir and module_dir.is_dir())

            if not in_cache and version and do_download:
                ok, err = ensure_downloaded(gomod, version, cwd=download_cwd)
                in_cache = bool(module_dir and module_dir.is_dir())
                if not ok and not in_cache:
                    components.append(
                        Component(
                            source="manifest",
                            section=section,
                            gomod=gomod,
                            version=version,
                            module_dir=str(module_dir) if module_dir else None,
                            in_cache=False,
                            metadata_path=None,
                            metadata_type=None,
                            metadata_class=None,
                            schema_path=None,
                            schema_present=False,
                            notes=[f"go mod download failed: {err}"],
                        )
                    )
                    continue

            if not in_cache:
                components.append(
                    Component(
                        source="manifest",
                        section=section,
                        gomod=gomod,
                        version=version,
                        module_dir=str(module_dir) if module_dir else None,
                        in_cache=False,
                        metadata_path=None,
                        metadata_type=None,
                        metadata_class=None,
                        schema_path=None,
                        schema_present=False,
                        notes=["module not in cache"],
                    )
                )
                continue

            assert module_dir is not None
            components.append(
                load_component_from_dir(module_dir, source="manifest", section=section, gomod=gomod, version=version)
            )

    for section, directory in LOCAL_SCHEMAS:
        components.append(load_component_from_dir(directory, source="local", section=section, gomod=None, version=None))

    # Index every ref used by any component, then resolve package_type ones.
    ref_index: dict[str, RefStatus] = {}
    for comp in components:
        for ref in comp.refs_used:
            status = ref_index.setdefault(ref, classify_ref(ref))
            label = comp.metadata_type or (comp.gomod or comp.module_dir or "?")
            status.used_by.append(f"{comp.section}/{label}")

    # Index consumers per ref so relative refs can find their source dir.
    consumers_by_ref: dict[str, list[Component]] = defaultdict(list)
    for comp in components:
        for ref in comp.refs_used:
            consumers_by_ref[ref].append(comp)

    for ref, status in ref_index.items():
        if status.kind == "package_type":
            resolve_package_type(status, cache_root=cache_root, module_versions=module_versions)
        elif status.kind == "relative":
            resolve_relative(status, consumers=consumers_by_ref[ref])

    return components, ref_index, module_versions


# ---------------------------------------------------------------------------
# Reporting
# ---------------------------------------------------------------------------


def summary(components: list[Component], ref_index: dict[str, RefStatus]) -> dict[str, Any]:
    by_section: dict[str, dict[str, int]] = defaultdict(lambda: defaultdict(int))
    for comp in components:
        b = by_section[comp.section]
        b["total"] += 1
        if comp.in_cache:
            b["in_cache"] += 1
        else:
            b["missing_module"] += 1
        if comp.schema_present:
            b["schema_present"] += 1
        else:
            b["schema_missing"] += 1

    resolvable_kinds = {"package_type", "relative"}
    refs_by_kind: dict[str, int] = defaultdict(int)
    refs_resolved = 0
    refs_dangling = 0
    for status in ref_index.values():
        refs_by_kind[status.kind] += 1
        if status.kind in resolvable_kinds:
            if status.resolved:
                refs_resolved += 1
            else:
                refs_dangling += 1

    return {
        "components_by_section": {k: dict(v) for k, v in by_section.items()},
        "total_components": len(components),
        "components_with_schema": sum(1 for c in components if c.schema_present),
        "components_missing_schema": sum(1 for c in components if not c.schema_present),
        "unique_refs": len(ref_index),
        "refs_by_kind": dict(refs_by_kind),
        "refs_resolved": refs_resolved,
        "refs_dangling": refs_dangling,
    }


def _portable_paths(node: Any, gomodcache_root: str) -> Any:
    """Rewrite absolute paths to portable forms for the JSON report:
    paths under the repo become `<repo>/...`, paths under the Go module
    cache become `<GOMODCACHE>/...`."""
    if isinstance(node, dict):
        return {k: _portable_paths(v, gomodcache_root) for k, v in node.items()}
    if isinstance(node, list):
        return [_portable_paths(v, gomodcache_root) for v in node]
    if isinstance(node, str):
        if node.startswith(str(REPO_ROOT) + "/"):
            return "<repo>/" + node[len(str(REPO_ROOT)) + 1 :]
        if gomodcache_root and node.startswith(gomodcache_root + "/"):
            return "<GOMODCACHE>/" + node[len(gomodcache_root) + 1 :]
    return node


def emit_json(
    out: Path,
    components: list[Component],
    ref_index: dict[str, RefStatus],
    module_versions: dict[str, str],
    do_download: bool,
) -> None:
    cache_root = str(gomodcache())
    raw = {
        "metadata": {
            "manifest": str(MANIFEST_PATH.relative_to(REPO_ROOT)),
            "go_mod_download_attempted": do_download,
        },
        "summary": summary(components, ref_index),
        "components": [asdict(c) for c in components],
        "refs": [asdict(r) for r in sorted(ref_index.values(), key=lambda s: s.ref)],
        "module_versions_in_manifest": module_versions,
    }
    portable = _portable_paths(raw, cache_root)
    out.write_text(json.dumps(portable, indent=2, sort_keys=False) + "\n")


def emit_markdown(out: Path, components: list[Component], ref_index: dict[str, RefStatus]) -> None:
    s = summary(components, ref_index)
    lines: list[str] = []
    lines.append("# DDOT Collector schema bundle — M1 inventory\n")
    lines.append(f"Manifest: `{MANIFEST_PATH.relative_to(REPO_ROOT)}`\n")

    lines.append("## Summary\n")
    lines.append(f"- Total components considered: **{s['total_components']}**")
    lines.append(f"- With `config.schema.yaml`: **{s['components_with_schema']}**")
    lines.append(f"- Without: **{s['components_missing_schema']}**")
    lines.append(f"- Unique `$ref` targets: **{s['unique_refs']}**")
    lines.append(f"- Resolved (package_type + relative): **{s['refs_resolved']}**")
    lines.append(f"- Dangling: **{s['refs_dangling']}**\n")

    lines.append("### Per-section breakdown\n")
    lines.append("| section | total | in cache | schema present | schema missing |")
    lines.append("|---|---:|---:|---:|---:|")
    for section, counts in sorted(s["components_by_section"].items()):
        lines.append(
            f"| {section} | {counts.get('total', 0)} | {counts.get('in_cache', 0)} | "
            f"{counts.get('schema_present', 0)} | {counts.get('schema_missing', 0)} |"
        )
    lines.append("")

    lines.append("### Ref kinds\n")
    lines.append("| kind | count |")
    lines.append("|---|---:|")
    for kind, n in sorted(s["refs_by_kind"].items()):
        lines.append(f"| `{kind}` | {n} |")
    lines.append("")

    lines.append("## Components missing `config.schema.yaml`\n")
    missing = [c for c in components if not c.schema_present]
    if not missing:
        lines.append("_None._\n")
    else:
        lines.append("| section | source | gomod | in cache | reason |")
        lines.append("|---|---|---|---|---|")
        for c in sorted(missing, key=lambda c: (c.section, c.gomod or c.module_dir or "")):
            reason = c.notes[0] if c.notes else "no schema in module"
            ic = "yes" if c.in_cache else "no"
            lines.append(f"| {c.section} | {c.source} | `{c.gomod or c.module_dir}` | {ic} | {reason} |")
        lines.append("")

    lines.append("## Dangling `$ref` targets\n")
    dangling = [r for r in ref_index.values() if r.kind in ("package_type", "relative") and not r.resolved]
    if not dangling:
        lines.append("_None._\n")
    else:
        lines.append("| ref | reason | used by |")
        lines.append("|---|---|---|")
        for r in sorted(dangling, key=lambda r: r.ref):
            users = ", ".join(sorted(set(r.used_by)))
            lines.append(f"| `{r.ref}` | {r.note or 'unresolved'} | {users} |")
        lines.append("")

    lines.append("## Components with schemas\n")
    present = [c for c in components if c.schema_present]
    if not present:
        lines.append("_None._\n")
    else:
        lines.append("| section | type | source | refs out |")
        lines.append("|---|---|---|---:|")
        for c in sorted(present, key=lambda c: (c.section, c.metadata_type or "")):
            lines.append(f"| {c.section} | `{c.metadata_type or '?'}` | {c.source} | {len(c.refs_used)} |")
        lines.append("")

    out.write_text("\n".join(lines) + "\n")


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0] if __doc__ else "")
    parser.add_argument(
        "--json", default="otelcol-schema-inventory.json", type=Path, help="output path for the JSON report"
    )
    parser.add_argument(
        "--md", default="otelcol-schema-inventory.md", type=Path, help="output path for the markdown summary"
    )
    parser.add_argument(
        "--no-go-mod-download", action="store_true", help="do not run `go mod download` for missing modules"
    )
    args = parser.parse_args(argv)

    if shutil.which("go") is None:
        print("error: 'go' not found on PATH", file=sys.stderr)
        return 2

    do_download = not args.no_go_mod_download
    components, ref_index, module_versions = collect(do_download)

    emit_json(args.json, components, ref_index, module_versions, do_download)
    emit_markdown(args.md, components, ref_index)

    s = summary(components, ref_index)
    print(
        f"wrote {args.json} and {args.md}; "
        f"components: {s['total_components']} "
        f"(schema: {s['components_with_schema']}, missing: {s['components_missing_schema']}); "
        f"refs: {s['unique_refs']} "
        f"(resolved: {s['refs_resolved']}, dangling: {s['refs_dangling']})"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
