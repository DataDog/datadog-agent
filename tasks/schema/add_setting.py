"""
Interactively add a new setting to the agent configuration schema.

Prompts for the setting name (dot-separated), type, default value, visibility,
and description, then inserts the setting into the correct YAML schema file
under ``pkg/config/schema/yaml/``.

For ``array`` settings it also prompts for the (mandatory) element type. When
the setting is public it ensures every ancestor section is public and
described — prompting for any missing section description, as this is mandatory
— then runs ``dda inv schema.lint`` so any remaining problems are visible.

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
                f"'{first}' is a section, not a leaf setting. " "Add at least one more path component after it.",
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
    sections.  The new setting is appended after the existing properties at its
    level; the existing ordering is preserved because the schema files are not
    sorted alphabetically — they follow a hand-curated order that mirrors the
    logical grouping of settings, and re-sorting would scramble it.

    Returns the list of ``(dotted_path, section_node)`` ancestor sections that
    were traversed or created on the way to the leaf, ordered root-first. The
    caller uses this to enforce that a public setting's ancestor sections are
    themselves public.

    Raises ``Exit`` if:
    - An intermediate component exists but is not a section.
    - The leaf setting already exists.
    """
    current = schema
    ancestors = []

    # Walk to the parent node, creating intermediate sections on demand.
    walked = []
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
        walked.append(part)
        ancestors.append((".".join(walked), child))
        current = child

    leaf = path_parts[-1]
    props = current.setdefault("properties", {})
    if leaf in props:
        raise Exit(
            f"Setting '{leaf}' already exists at this path. " "Use `dda inv schema.locate` to inspect it.",
            code=1,
        )
    # Append the new setting, preserving the existing (hand-curated) order.
    props[leaf] = setting_node
    return ancestors


# ---------------------------------------------------------------------------
# Public-visibility propagation to ancestor sections
# ---------------------------------------------------------------------------

# Canonical key order for a section node — keeps diffs clean when we add
# visibility/description to an existing or freshly created section.
_SECTION_KEY_ORDER = ["node_type", "title", "type", "visibility", "description", "tags", "env_vars", "properties"]


def _reorder_section(node):
    """Reorder *node*'s keys in place to the canonical section key order."""
    ordered = {k: node[k] for k in _SECTION_KEY_ORDER if k in node}
    for k, v in node.items():
        if k not in ordered:
            ordered[k] = v
    node.clear()
    node.update(ordered)


def _ensure_public_ancestors(ancestors):
    """Make every ancestor section of a public setting public and described.

    A public setting requires that *all* its ancestor sections are public and
    carry a non-empty description — this is mandatory and enforced by
    ``dda inv schema.lint``. For each ancestor that is not yet public or lacks a
    description, this flips it to public and interactively prompts the user for
    the (mandatory) description.

    *ancestors* is a list of ``(dotted_path, section_node)`` pairs, root-first.
    """
    for anc_path, anc_node in ancestors:
        needs_public = anc_node.get("visibility") != "public"
        desc = anc_node.get("description", "")
        needs_desc = not desc or not str(desc).strip()
        if not needs_public and not needs_desc:
            continue

        print(
            f"\nParent section '{anc_path}' contains a public setting, so it must "
            f"also be public. This is MANDATORY."
        )
        if needs_public:
            anc_node["visibility"] = "public"
        if needs_desc:
            print(f"A non-empty description for section '{anc_path}' is MANDATORY.")
            while True:
                section_desc = _ask_multiline(f"Description for section '{anc_path}'")
                if section_desc:
                    anc_node["description"] = section_desc
                    break
                print("  A non-empty description is required.")
        _reorder_section(anc_node)


# ---------------------------------------------------------------------------
# Task entry point
# ---------------------------------------------------------------------------


@task(
    help={
        "schema": "Which schema to target: 'core' (default) or 'system-probe'.",
    }
)
def add_setting(ctx, schema="core"):
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
    item_type = None
    if setting_type == "array":
        print("  Default for array type is [] (empty list).")
        default_value = []
        # An array must declare the type of its elements (mandatory — the schema
        # linter rejects an array setting without an 'items' field).
        print("  An array must declare the type of its elements. This is mandatory.")
        item_type = _ask("Array item type", valid=VALID_TYPES)
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
    if item_type is not None:
        setting_node["items"] = {"type": item_type}
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
    ancestors = _insert(loaded, path_within_file, setting_node)

    # A public setting requires all of its ancestor sections to be public and
    # described. For a split sub-file, the file root itself is the enclosing
    # section (e.g. 'apm_config') and counts as an ancestor.
    if visibility == "public":
        if loaded.get("node_type") == "section":
            ancestors = [(parts[0], loaded), *ancestors]
        _ensure_public_ancestors(ancestors)

    _dump(loaded, file_path)

    # --- Summary ---
    print(f"\nAdded '{setting_name}' to {file_path}")
    print(f"  type:       {setting_type}")
    if item_type is not None:
        print(f"  items.type: {item_type}")
    print(f"  default:    {default_value!r}")
    print(f"  visibility: {visibility}")
    if description:
        preview = description[:80].replace("\n", " ")
        ellipsis = "…" if len(description) > 80 else ""
        print(f"  description: {preview}{ellipsis}")

    # --- Lint the updated schema so any problems are visible immediately ---
    print("\nRunning `dda inv schema.lint` to validate the updated schema...\n")
    result = ctx.run("dda inv schema.lint", warn=True)
    if result is None or result.exited != 0:
        raise Exit(
            f"\nSchema linting failed for {file_path}. Fix the reported errors "
            "(see `dda inv schema.lint`) before committing.",
            code=result.exited if result is not None else 1,
        )
