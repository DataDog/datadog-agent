"""
Build the Go build tag manifest read by bazel/go_tags/transition.bzl.

For each go_library import path, the manifest lists the root build tags that its transitive closure reads in
`//go:build` lines. Import paths whose closure reads no root tag are omitted. The transition drops every
other root tag, so tag-insensitive libraries are configured once instead of once per root tag set.
"""

from __future__ import annotations

import functools
import json
import os
import re
import shutil
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor

GRAPH_KINDS = "go_library|go_tool_library|go_test|go_binary|go_proto_library|go_proto_compiler|go_source|alias"
QUERY = f'kind("{GRAPH_KINDS}", deps(//...))'
MANIFEST_PATH = "bazel/go_tags/closure_tags.bzl"

_LIBRARY_KINDS = {"go_library", "go_proto_library", "go_source"}
_ROOT_KINDS = {"go_test", "go_binary"}
# go_proto_library adds its compilers' deps (grpc, the protobuf runtime) to its own.
_EDGE_ATTRS = ("deps", "embed", "compilers", "compiler", "actual")
_QUERIED_ATTRS = (*_EDGE_ATTRS, "srcs", "importpath", "gotags")
_SCANNED_EXTENSIONS = (".go", ".s", ".S", ".c", ".h", ".cc", ".cpp", ".cxx", ".hh", ".hpp", ".m", ".syso")
_IDENTIFIER = re.compile(r"[A-Za-z_][A-Za-z0-9_.]*")


def _values(attr: dict) -> list[str]:
    """String values of a query attribute, with all select() branches unioned."""
    values = list(attr.get("stringListValue", []))
    if "stringValue" in attr:
        values.append(attr["stringValue"])
    for element in attr.get("selectorList", {}).get("elements", []):
        values.extend(element.get("stringListValue", []))
        for entry in element.get("entries", []):
            values.extend(entry.get("stringListValue", []))
            if "stringValue" in entry:
                values.append(entry["stringValue"])
    return values


def parse_graph(streamed_jsonproto: str) -> dict[str, dict]:
    """Rules of `bazel query --consistent_labels --output=streamed_jsonproto`, keyed by label."""
    nodes = {}
    for line in streamed_jsonproto.splitlines():
        target = json.loads(line)
        if target.get("type") != "RULE":
            continue
        rule = target["rule"]
        attrs = {a["name"]: a for a in rule.get("attribute", [])}
        nodes[rule["name"]] = {
            "kind": rule["ruleClass"],
            "deps": [d for name in _EDGE_ATTRS if name in attrs for d in _values(attrs[name])],
            "srcs": _values(attrs["srcs"]) if "srcs" in attrs else [],
            "importpath": attrs.get("importpath", {}).get("stringValue", ""),
            "gotags": _values(attrs["gotags"]) if "gotags" in attrs else [],
        }
    return nodes


def scan_build_tags(path: str) -> set[str] | None:
    """Identifiers in the build constraints of a source file, or None if it can't be read."""
    tags = set()
    in_block = False
    try:
        with open(path, encoding="utf-8", errors="replace") as f:
            for line in f:
                line = line.strip()
                if in_block:
                    in_block = "*/" not in line
                elif line.startswith(("//go:build", "// +build")):
                    tags.update(_IDENTIFIER.findall(line.split("build", 1)[1]))
                elif line.startswith("/*"):
                    in_block = "*/" not in line[2:]
                elif line and not line.startswith("//"):
                    break  # Go only reads constraints in the leading comments, in every kind of file
    except OSError:
        return None
    return tags


_ERROR_LOCATION = re.compile(r"^ERROR: (.+?):\d+:\d+: ")


def main_repo_errors(query_stderr: str, workspace: str, output_base: str) -> list[str]:
    """
    Query errors located in the main repository.

    `--keep_going` drops a package that fails to load, and with it any external package only it depends on.
    """

    # Bazel prints Windows paths with forward slashes.
    def directory(path: str) -> str:
        return path.replace("\\", "/").rstrip("/") + "/"

    main, external = directory(workspace), directory(output_base)
    errors = []
    for line in query_stderr.splitlines():
        location = _ERROR_LOCATION.match(line)
        # CI keeps the output base inside the checkout.
        if location and location[1].startswith(main) and not location[1].startswith(external):
            errors.append(line)
    return errors


def _is_main_repo(label: str) -> bool:
    return label.startswith(("//", "@//", "@@//"))


def source_path(label: str, workspace: str, output_base: str) -> str:
    repo, _, rest = label.lstrip("@").partition("//")
    package, _, name = rest.partition(":")
    root = workspace if repo in ("", "_main") else os.path.join(output_base, "external", repo)
    return os.path.join(root, package, name)


def build_manifest(nodes: dict[str, dict], scan) -> tuple[list[str], dict[str, list[str] | None]]:
    """
    Returns (tag universe, import path -> closure tags within that universe).

    `scan` maps a source label to its build tags, or None when it can't be read (generated, not yet built).
    Import paths whose closure can't be fully scanned map to None, so the transition keeps all their tags.
    Raises ValueError when the graph has no main-repository library or a dependency is missing from it, i.e. its
    package failed to load.
    """
    universe = {
        tag
        for label, node in nodes.items()
        if node["kind"] in _ROOT_KINDS and _is_main_repo(label)
        for tag in node["gotags"]
    }

    @functools.cache
    def closure(label: str) -> frozenset[str] | None:
        node = nodes.get(label)
        if node is None:
            raise ValueError(f"{label} is missing from the `bazel query` graph, check the query errors above")
        tags = set()
        for src in node["srcs"]:
            if src.endswith(_SCANNED_EXTENSIONS):
                found = scan(src)
                if found is None:
                    return None
                tags |= found
        for dep in node["deps"]:
            dep_tags = closure(dep)
            if dep_tags is None:
                return None
            tags |= dep_tags
        return frozenset(tags & universe)

    manifest: dict[str, set[str] | None] = {}
    main_paths = set()
    for label, node in nodes.items():
        if node["kind"] != "go_library" or not node["importpath"]:
            continue
        path = node["importpath"]
        tags = closure(label)
        known = manifest.get(path, set())
        manifest[path] = None if tags is None or known is None else known | tags
        if _is_main_repo(label):
            main_paths.add(path)
    # The transition treats omitted import paths as tag-free, so an incomplete graph must not be written.
    if not main_paths:
        raise ValueError("`bazel query` returned no main-repository go_library, check the query errors above")
    return sorted(universe), {
        path: None if tags is None else sorted(tags) for path, tags in manifest.items() if tags != set()
    }


def render_manifest(universe: list[str], manifest: dict[str, list[str] | None]) -> str:
    lines = [
        "# Code generated by `dda inv go.write-tag-manifest` - DO NOT EDIT.",
        "# Resolve merge conflicts by rerunning it.",
        "",
        f"TAG_UNIVERSE = {json.dumps(universe)}",
        "",
        "CLOSURE_TAGS = {",
        *(
            f"    {json.dumps(path)}: {'None' if manifest[path] is None else json.dumps(manifest[path])},"
            for path in sorted(manifest)
        ),
        "}",
    ]
    return "\n".join(lines) + "\n"


def main() -> int:
    workspace = os.environ.get("BUILD_WORKSPACE_DIRECTORY")
    if not workspace:
        print("Run it with `bazel run //tasks:write_go_tag_manifest`", file=sys.stderr)
        return 1

    # `bazel run` passes on the variables bazelisk set for tools/bazel; without them, it reruns tools/bazel, which adds
    # the CI and cache flags the outer command had.
    env = {k: v for k, v in os.environ.items() if k not in ("BAZEL_REAL", "BAZELISK_SKIP_WRAPPER")}
    command = shutil.which("bazelisk", path=env.get("PATH")) or "bazel"

    def bazel(*args: str) -> subprocess.CompletedProcess:
        return subprocess.run([command, *args], cwd=workspace, env=env, capture_output=True, text=True)

    query = bazel(
        "query",
        "--consistent_labels",
        "--keep_going",
        "--output=streamed_jsonproto",
        f"--proto:output_rule_attrs={','.join(_QUERIED_ATTRS)}",
        "--proto:rule_inputs_and_outputs=false",
        "--proto:locations=false",
        QUERY,
    )
    info = bazel("info", "output_base")
    if info.returncode != 0:
        print(info.stderr, file=sys.stderr)
        return info.returncode
    output_base = info.stdout.strip()
    # 3: some packages failed to load, e.g. Windows-only toolchain repositories on other hosts. That is only fatal
    # for main-repository packages, or when a Go package is among them, which build_manifest detects.
    if query.returncode not in (0, 3) or main_repo_errors(query.stderr, workspace, output_base):
        print(query.stderr, "`bazel query` failed, see the errors above", sep="\n", file=sys.stderr)
        return query.returncode or 1
    try:
        nodes = parse_graph(query.stdout)
        sources = list({src for node in nodes.values() for src in node["srcs"] if src.endswith(_SCANNED_EXTENSIONS)})
        # Opening tens of thousands of files dominates the run, and file I/O releases the GIL.
        with ThreadPoolExecutor(16) as pool:
            found = pool.map(lambda label: scan_build_tags(source_path(label, workspace, output_base)), sources)
            scans = dict(zip(sources, found, strict=True))
        universe, manifest = build_manifest(nodes, scans.__getitem__)
    except ValueError as e:
        print(query.stderr, e, sep="\n", file=sys.stderr)
        return 1
    with open(os.path.join(workspace, MANIFEST_PATH), "w", encoding="utf-8", newline="\n") as bzl:
        bzl.write(render_manifest(universe, manifest))
    return 0


if __name__ == "__main__":
    sys.exit(main())
