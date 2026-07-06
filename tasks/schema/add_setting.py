"""
Interactively add a new setting to the agent configuration schema.

Prompts for the setting name (dot-separated), type, default value, visibility,
and description, then inserts the setting into the correct YAML schema file
under ``pkg/config/schema/yaml/``.

Run with::

    dda inv schema.add-setting

The schema is split across:
- ``core_schema.yaml`` — top-level core settings; some sections are split into
  their own sibling files (e.g. ``apm_config.yaml``).
- ``system-probe_schema.yaml`` — system-probe settings.

When you specify a setting path whose first component is a split section
(e.g. ``apm_config.enabled``), this task edits the split sub-file directly.
Otherwise it edits ``core_schema.yaml``.  Use ``--schema=system-probe`` to
target the system-probe schema instead.
"""

import os

import yaml
from invoke import task
from invoke.exceptions import Exit

from tasks.schema.generate import CORE_SPLIT_SECTIONS, SCHEMA_DIR

# ---------------------------------------------------------------------------
# File layout constants
# ---------------------------------------------------------------------------

CORE_SCHEMA = os.path.join(SCHEMA_DIR, "core_schema.yaml")
SYSPROBE_SCHEMA = os.path.join(SCHEMA_DIR, "system-probe_schema.yaml")

VALID_TYPES = ["string", "integer", "number", "boolean", "array", "object"]

# ---------------------------------------------------------------------------
# YAML helpers — preserve multiline strings with the literal block style (|)
# ---------------------------------------------------------------------------


def _str_presenter(dumper, data):
    if "\n" in data:
        return dumper.represent_scalar("tag:yaml.org,2002:str", data, style="|")
    return dumper.represent_scalar("tag:yaml.org,2002:str", data)


yaml.add_representer(str, _str_presenter)


def _load(path):
    with open(path) as f:
        return yaml.safe_load(f)


def _dump(schema, path):
    with open(path, "w") as f:
        yaml.dump(schema, f, default_flow_style=False, sort_keys=False, allow_unicode=True)


# ---------------------------------------------------------------------------
# Interactive prompts
# ---------------------------------------------------------------------------


def _ask(prompt, *, valid=None, default=None, allow_empty=False):
    """Prompt the user for a single-line value, with optional validation."""
    while True:
        suffix = f" [{default}]" if default is not None else ""
        if valid:
            hint = "/".join(valid)
            raw = input(f"{prompt} ({hint}){suffix}: ").strip()
        else:
            raw = input(f"{prompt}{suffix}: ").strip()

        if not raw:
            if default is not None:
                return default
            if allow_empty:
                return ""
            print("  This field is required.")
            continue

        if valid and raw not in valid:
            print(f"  Invalid choice. Must be one of: {', '.join(valid)}")
            continue

        return raw


def _ask_multiline(prompt):
    """Prompt the user for a multi-line value. An empty line ends input."""
    print(f"{prompt} (empty line to finish):")
    lines = []
    while True:
        line = input()
        if line == "":
            break
        lines.append(line)
    return "\n".join(lines).strip()


def _parse_default(value_str, type_name):
    """Convert a string entered by the user to the correct Python type."""
    if type_name == "integer":
        try:
            return int(value_str)
        except ValueError:
            raise Exit(f"Not a valid integer: '{value_str}'", code=1) from None
    if type_name == "number":
        try:
            return float(value_str)
        except ValueError:
            raise Exit(f"Not a valid number: '{value_str}'", code=1) from None
    if type_name == "boolean":
        if value_str.lower() in ("true", "yes", "1"):
            return True
        if value_str.lower() in ("false", "no", "0"):
            return False
        raise Exit(f"Not a valid boolean: '{value_str}'. Use true or false.", code=1)
    # string / array / object — return as-is (array/object handled before calling)
    return value_str


# ---------------------------------------------------------------------------
# Routing: which file to edit, and which path within that file
# ---------------------------------------------------------------------------


def _resolve_target(parts, schema_label):
    """Return ``(file_path, path_within_file)`` for a dotted setting path.

    *parts* is the list of dot-split components of the setting name.
    *schema_label* is ``"core"`` or ``"system-probe"``.

    For split sections (e.g. ``apm_config``), the sub-file is targeted and the
    section name is stripped from *path_within_file* because the sub-file root
    *is* that section.
    """
    if schema_label == "system-probe":
        return SYSPROBE_SCHEMA, parts

    # Core schema
    first = parts[0]
    if first in CORE_SPLIT_SECTIONS:
        sub_file = os.path.join(SCHEMA_DIR, f"{first}.yaml")
        inner = parts[1:]
        if not inner:
            raise Exit(
                f"'{first}' is a section, not a leaf setting. Add at least one more path component after it.",
                code=1,
            )
        return sub_file, inner

    return CORE_SCHEMA, parts


# ---------------------------------------------------------------------------
# YAML schema manipulation
# ---------------------------------------------------------------------------


def _insert(schema, path_parts, setting_node):
    """Insert *setting_node* at *path_parts* within *schema*.

    Intermediate path components that do not yet exist are created as new
    sections.  Properties at each level are kept sorted alphabetically, which
    matches the convention used throughout the existing schema files.

    Raises ``Exit`` if:
    - An intermediate component exists but is not a section.
    - The leaf setting already exists.
    """
    current = schema

    # Walk to the parent node, creating intermediate sections on demand.
    for part in path_parts[:-1]:
        props = current.setdefault("properties", {})
        if part not in props:
            props[part] = {
                "node_type": "section",
                "type": "object",
                "properties": {},
            }
        child = props[part]
        # Guard: refuse to tunnel through a $ref that wasn't resolved.
        if "$ref" in child:
            raise Exit(
                f"Path component '{part}' is a split section ($ref). "
                "Specify the full dotted path — the task will route to the "
                "correct sub-file automatically.",
                code=1,
            )
        if child.get("node_type") == "setting":
            raise Exit(
                f"Path component '{part}' is already a leaf setting, not a section.",
                code=1,
            )
        current = child

    leaf = path_parts[-1]
    props = current.setdefault("properties", {})
    if leaf in props:
        raise Exit(
            f"Setting '{leaf}' already exists at this path. Use `dda inv schema.locate` to inspect it.",
            code=1,
        )
    props[leaf] = setting_node
    # Sort properties alphabetically — matches the schema file convention.
    current["properties"] = dict(sorted(props.items()))


# ---------------------------------------------------------------------------
# Task entry point
# ---------------------------------------------------------------------------


@task(
    help={
        "schema": "Which schema to target: 'core' (default) or 'system-probe'.",
    }
)
def add_setting(_ctx, schema="core"):
    """
    Interactively add a new setting to the agent configuration schema.

    Prompts for the setting name, type, default value, visibility, and
    description, then inserts the setting into the correct YAML file under
    pkg/config/schema/yaml/.
    """
    valid_schemas = ("core", "system-probe")
    if schema not in valid_schemas:
        raise Exit(f"Invalid --schema value '{schema}'. Must be one of: {', '.join(valid_schemas)}", code=1)

    print("=== Add a new agent configuration setting ===")
    print("Setting names use dot notation: e.g. 'api_key', 'proxy.https',")
    print("'apm_config.enabled', or 'logs_config.batch_max_size'.\n")

    # --- Setting name ---
    setting_name = _ask("Setting name (dot-separated path)")
    parts = [p.strip() for p in setting_name.split(".") if p.strip()]
    if not parts:
        raise Exit("Setting name must be non-empty.", code=1)

    # --- Type ---
    setting_type = _ask("Type", valid=VALID_TYPES)

    # --- Default value ---
    if setting_type == "array":
        print("  Default for array type is [] (empty list).")
        default_value = []
    elif setting_type == "object":
        print("  Default for object type is {} (empty mapping).")
        default_value = {}
    else:
        default_str = _ask(f"Default value ({setting_type})")
        default_value = _parse_default(default_str, setting_type)

    # --- Visibility ---
    print(
        "\nVisibility controls whether this setting appears in public documentation.\n"
        "  public   — documented; requires a description.\n"
        "  internal — not externally documented; description is optional.\n"
    )
    visibility = _ask("Visibility", valid=["public", "internal"])

    # --- Description ---
    if visibility == "public":
        print("\nDescription is required for public settings.")
        while True:
            description = _ask_multiline("Description")
            if description:
                break
            print("  A non-empty description is required for public settings.")
    else:
        description = _ask_multiline("Description (optional — press Enter to skip)")

    # --- Build the setting node ---
    # Key order follows the convention used in existing schema files.
    setting_node = {
        "node_type": "setting",
        "type": setting_type,
        "default": default_value,
    }
    if visibility == "public":
        setting_node["visibility"] = "public"
    if description:
        setting_node["description"] = description

    # --- Resolve target file ---
    file_path, path_within_file = _resolve_target(parts, schema)

    if not os.path.exists(file_path):
        raise Exit(f"Schema file not found: {file_path}", code=1)

    # --- Load, modify, write ---
    loaded = _load(file_path)
    _insert(loaded, path_within_file, setting_node)
    _dump(loaded, file_path)

    # --- Summary ---
    print(f"\nAdded '{setting_name}' to {file_path}")
    print(f"  type:       {setting_type}")
    print(f"  default:    {default_value!r}")
    print(f"  visibility: {visibility}")
    if description:
        preview = description[:80].replace("\n", " ")
        ellipsis = "…" if len(description) > 80 else ""
        print(f"  description: {preview}{ellipsis}")
    print("\nRun `dda inv schema.lint` to validate the updated schema.")
