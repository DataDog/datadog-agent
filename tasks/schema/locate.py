"""
Locate a setting or section in the Agent configuration schemas.

Given a dotted config path (e.g. ``api_key``, ``proxy.https``,
``apm_config.enabled``), find where it is defined in the YAML schema source
files under ``pkg/config/schema/yaml`` and print the matching schema node.

Run with:

    dda inv -- schema.locate <setting>

The core schema is split across a top file (``core_schema.yaml``) and
per-section sub-files referenced via ``$ref``. This module reads the YAML
*source* files directly (PyYAML ``compose()`` for line numbers) so reported
locations point at editable source, and reuses ``resolve_schema`` to inline
``$ref``s when extracting the node content.
"""

import json as _json
import os

import yaml
from invoke import task
from invoke.exceptions import Exit

from tasks.schema.merge_schema import resolve_schema

SCHEMA_DIR = os.path.join("pkg", "config", "schema", "yaml")
CORE_SCHEMA = os.path.join(SCHEMA_DIR, "core_schema.yaml")
SYSPROBE_SCHEMA = os.path.join(SCHEMA_DIR, "system-probe_schema.yaml")

# (label, top file) pairs searched by default. The label is what gets reported
# to the user so they know which schema a match came from.
SCHEMAS = [
    ("core", CORE_SCHEMA),
    ("system-probe", SYSPROBE_SCHEMA),
]


# ---------------------------------------------------------------------------
# Composed-node helpers (line numbers + $ref following)
# ---------------------------------------------------------------------------


def _compose(path):
    """Return the root composed node of a YAML file (carries ``start_mark``)."""
    with open(path) as f:
        return yaml.compose(f)


def _mapping_entry(node, key):
    """Return the ``(key_node, value_node)`` pair for *key* in a MappingNode, or None."""
    if not isinstance(node, yaml.MappingNode):
        return None
    for key_node, value_node in node.value:
        if isinstance(key_node, yaml.ScalarNode) and key_node.value == key:
            return key_node, value_node
    return None


def _mapping_get(node, key):
    """Return the value node for *key* in a MappingNode, or None."""
    entry = _mapping_entry(node, key)
    return entry[1] if entry else None


def _ref_target(value_node):
    """If *value_node* is a single-key ``{$ref: <file>}`` mapping, return <file>; else None."""
    if isinstance(value_node, yaml.MappingNode) and len(value_node.value) == 1:
        key_node, ref_node = value_node.value[0]
        if isinstance(key_node, yaml.ScalarNode) and key_node.value == "$ref":
            return ref_node.value
    return None


def _locate_physical(top_file, parts):
    """Return ``(physical_file, line_1based)`` for the dotted *parts*, or None.

    Walks the composed node tree under each level's ``properties``. When a
    matched value is a ``$ref`` node and there are more parts to consume, the
    walk follows the ref into the sibling sub-file. When the ``$ref`` node is
    itself the final part (a bare split section), the reported line is the
    ``$ref:`` entry line in the current file (per Q4).
    """
    node = _compose(top_file)
    current_file = top_file
    line = None

    for i, part in enumerate(parts):
        props = _mapping_get(node, "properties")
        entry = _mapping_entry(props, part)
        if entry is None:
            return None
        key_node, value_node = entry
        line = key_node.start_mark.line + 1

        ref = _ref_target(value_node)
        if ref is not None and i < len(parts) - 1:
            # Cross into the referenced sub-file; its root is the section node.
            current_file = os.path.join(os.path.dirname(current_file), ref)
            node = _compose(current_file)
        elif ref is not None:
            # Bare split section: report the $ref entry line, not the key line.
            ref_key_node = value_node.value[0][0]
            line = ref_key_node.start_mark.line + 1
            node = value_node
        else:
            node = value_node

    return current_file, line


# ---------------------------------------------------------------------------
# Content extraction (from the merged schema) + display shaping
# ---------------------------------------------------------------------------


def _navigate_merged(merged, parts):
    """Return the resolved node dict for the dotted *parts* in a merged schema, or None.

    Only traverses named ``properties`` children (settings + sections); any
    extra part under a leaf, or a missing key, yields None.
    """
    node = merged
    for part in parts:
        if not isinstance(node, dict):
            return None
        props = node.get("properties")
        if not isinstance(props, dict) or part not in props:
            return None
        node = props[part]
    return node


def _is_section(node):
    """A node is a section if it says so or carries child ``properties``."""
    return node.get("node_type") == "section" or "properties" in node


def _display_node(node):
    """Shape *node* for display.

    Settings are returned in full. Sections are returned with their own metadata
    but with ``properties`` reduced to the sorted list of immediate child key
    names, so locating a large section does not dump thousands of lines.
    """
    if not isinstance(node, dict) or not _is_section(node):
        return node
    shaped = {key: value for key, value in node.items() if key != "properties"}
    props = node.get("properties")
    if isinstance(props, dict):
        shaped["properties"] = sorted(props.keys())
    return shaped


# ---------------------------------------------------------------------------
# Cross-schema aggregation
# ---------------------------------------------------------------------------


def locate_setting(setting, schemas=SCHEMAS):
    """Locate *setting* (a dotted path) across *schemas*.

    Returns a list of match dicts ``{schema, path, file, line, node}``, one per
    schema in which the path resolves. A schema contributes a match only when
    BOTH the physical location and the merged-content lookup succeed (they read
    the same source, so they agree); otherwise that schema is skipped.
    """
    parts = setting.split(".")
    matches = []
    for label, top_file in schemas:
        physical = _locate_physical(top_file, parts)
        if physical is None:
            continue
        node = _navigate_merged(resolve_schema(top_file), parts)
        if node is None:
            continue
        file_path, line = physical
        matches.append(
            {
                "schema": label,
                "path": setting,
                "file": file_path,
                "line": line,
                "node": _display_node(node),
            }
        )
    return matches


# ---------------------------------------------------------------------------
# Rendering + task entry point
# ---------------------------------------------------------------------------


def _str_presenter(dumper, data):
    if "\n" in data:
        return dumper.represent_scalar("tag:yaml.org,2002:str", data, style="|")
    return dumper.represent_scalar("tag:yaml.org,2002:str", data)


def _render(matches, as_json):
    """Render *matches* as a string: a JSON array if *as_json*, else human text."""
    if as_json:
        return _json.dumps(matches, indent=2, sort_keys=False)

    yaml.add_representer(str, _str_presenter)
    blocks = []
    for match in matches:
        header = f"[{match['schema']}] {match['file']}:{match['line']}"
        body = yaml.dump({match["path"]: match["node"]}, sort_keys=False).rstrip()
        blocks.append(f"{header}\n{body}")
    return "\n\n".join(blocks)


def _select_schemas(schemas, target):
    """Return the subset of *schemas* selected by *target* (a label or None for all)."""
    if target is None:
        return schemas
    selected = [(label, path) for label, path in schemas if label == target]
    if not selected:
        valid = ", ".join(label for label, _ in schemas)
        raise Exit(f"Invalid target '{target}'. Must be one of: {valid}.", code=1)
    return selected


def run_locate(setting, target=None, as_json=False, schemas=SCHEMAS):
    """Locate *setting* and return the rendered output string.

    Raises ``Exit(code=1)`` if *target* is invalid or the path is not found.
    """
    selected = _select_schemas(schemas, target)
    matches = locate_setting(setting, selected)
    if not matches:
        scope = f" in schema '{target}'" if target else ""
        raise Exit(f"Setting or section '{setting}' not found{scope}.", code=1)
    return _render(matches, as_json)


@task(
    help={
        "setting": "Dotted config path to locate, e.g. 'api_key', 'proxy.https', 'apm_config.enabled'.",
        "target": "Restrict the search to a single schema: 'core' or 'system-probe' (default: both).",
        "json": "Emit a JSON array of matches instead of human-readable text.",
    },
    positional=["setting"],
)
def locate(_ctx, setting, target=None, json=False):
    """
    Locate a setting or section in the Agent configuration schemas.

    Prints the schema node and the source file + line where it is defined,
    searching both the core and system-probe schemas.
    """
    print(run_locate(setting, target=target, as_json=json))
