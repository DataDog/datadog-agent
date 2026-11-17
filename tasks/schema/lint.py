"""
Schema linter for the Datadog Agent configuration schemas.

Validates generated YAML schema files (pkg/config/schema/yaml/*.yaml) against
a set of quality rules. Run with:

    dda inv schema.lint
"""

import glob
import os
import re

import yaml
from invoke import task
from invoke.exceptions import Exit

from tasks.schema.merge_schema import resolve_schema
from tasks.schema.produce_byproduct import AGENT_FORMATS

SCHEMA_DIR = os.path.join("pkg", "config", "schema", "yaml")
EXCEPTIONS_FILE = os.path.join(os.path.dirname(__file__), "lint_exceptions.yaml")

VALID_TYPES = {"string", "number", "integer", "boolean", "array", "object"}
VALID_NODE_TYPES = {"section", "setting"}
VALID_PLATFORM_KEYS = {"darwin", "windows", "linux", "aix", "container", "fargate", "other"}
REQUIRED_PLATFORM_KEYS_WITHOUT_OTHER = {"darwin", "windows", "linux", "aix"}
VALID_ENV_PARSERS = {
    "comma_separated",
    "space_separated",
    "json",
    "comma_and_space_separated",
    "traces_span",
    "csv_comma_separated",
    "comma_then_space_separated",
    "json_list_or_comma_separated",
    "json_list_or_space_separated",
}

SLACK_HINT = "If you have any question please reach out on #fleet-automation"


# ---------------------------------------------------------------------------
# Schema traversal helpers
# ---------------------------------------------------------------------------


def walk_nodes(schema, path=""):
    """
    Recursively yield (dotted_path, node) for every non-root node in the schema.

    The root schema envelope ({properties: {...}}) is skipped intentionally —
    it is not a setting or section, just a container.
    """
    props = schema.get("properties")
    if not isinstance(props, dict):
        return
    for key, node in props.items():
        if not isinstance(node, dict):
            continue
        node_path = f"{path}.{key}" if path else key
        yield node_path, node
        if node.get("node_type") == "section":
            yield from walk_nodes(node, node_path)


def _walk_with_ancestors(schema, path="", ancestors=None):
    """
    Recursively yield (dotted_path, node, ancestor_nodes) where ancestor_nodes is
    a list of (ancestor_path, ancestor_node) pairs from root to immediate parent.
    """
    if ancestors is None:
        ancestors = []
    props = schema.get("properties")
    if not isinstance(props, dict):
        return
    for key, node in props.items():
        if not isinstance(node, dict):
            continue
        node_path = f"{path}.{key}" if path else key
        yield node_path, node, ancestors
        if node.get("node_type") == "section":
            yield from _walk_with_ancestors(node, node_path, ancestors + [(node_path, node)])


def get_tags(node):
    """Return the list of tag strings for a node, or []."""
    tags = node.get("tags", [])
    return tags if isinstance(tags, list) else []


# ---------------------------------------------------------------------------
# Check 1: YAML validity
# ---------------------------------------------------------------------------


def check_yaml_valid(path):
    """
    Check that the file at *path* is valid YAML.

    Returns a list of error strings (empty means no errors).
    """
    try:
        with open(path) as f:
            yaml.safe_load(f)
        return []
    except yaml.YAMLError as exc:
        return [f"{path}: YAML parse error: {exc}"]


# ---------------------------------------------------------------------------
# Check 2: JSON-schema structural validity
# ---------------------------------------------------------------------------


def check_json_schema_structure(path, schema, array_no_items_exceptions=None):
    """
    Check structural validity of schema nodes:
      - 'type' values must be one of the recognised JSON Schema types.
      - Nodes with type 'array' must have an 'items' field.

    *array_no_items_exceptions* is a set of dotted paths exempt from the
    array-without-items check.

    Returns a list of error strings.
    """
    if array_no_items_exceptions is None:
        array_no_items_exceptions = set()
    errors = []
    for node_path, node in walk_nodes(schema):
        type_val = node.get("type")
        if type_val is not None:
            if type_val not in VALID_TYPES:
                errors.append(
                    f"{path}: [{node_path}] Invalid type value '{type_val}'. "
                    f"Must be one of: {sorted(VALID_TYPES)}. "
                    f"Fix: change 'type' to a valid JSON Schema type."
                )
            elif type_val == "array" and "items" not in node and node_path not in array_no_items_exceptions:
                errors.append(
                    f"{path}: [{node_path}] Setting has type 'array' but is missing the 'items' field. "
                    f"Fix: add an 'items' field describing the element type, "
                    f"e.g. 'items: {{type: string}}'."
                )
    return errors


# ---------------------------------------------------------------------------
# Check 3: Public nodes (any node_type) have a non-empty description
# ---------------------------------------------------------------------------


def check_public_descriptions(path, schema):
    """
    Check that every node (setting or section) with visibility='public' has a
    non-empty description.

    Returns a list of error strings.
    """
    errors = []
    for node_path, node in walk_nodes(schema):
        if node.get("visibility") != "public":
            continue
        desc = node.get("description", "")
        if not desc or not str(desc).strip():
            errors.append(
                f"{path}: [{node_path}] Public node is missing a description. "
                f"Fix: add a non-empty 'description' field explaining what this setting or section is."
            )
    return errors


# ---------------------------------------------------------------------------
# Check 4: Public settings' ancestor sections are public and have descriptions
# ---------------------------------------------------------------------------


def check_public_parent_sections(path, schema):
    """
    For every public setting, verify that each ancestor section is also public
    and has a non-empty description.

    Returns a list of error strings.
    """
    errors = []
    for node_path, node, ancestors in _walk_with_ancestors(schema):
        if node.get("node_type") != "setting":
            continue
        if node.get("visibility") != "public":
            continue
        for anc_path, anc_node in ancestors:
            if anc_node.get("visibility") != "public":
                errors.append(
                    f"{path}: [{node_path}] Public setting has a non-public ancestor section '{anc_path}'. "
                    f"Fix: set 'visibility: public' on section '{anc_path}'."
                )
            desc = anc_node.get("description", "")
            if not desc or not str(desc).strip():
                errors.append(
                    f"{path}: [{node_path}] Public setting has ancestor section '{anc_path}' "
                    f"without a description. "
                    f"Fix: add a non-empty 'description' field to section '{anc_path}'."
                )
    return errors


# ---------------------------------------------------------------------------
# Check 5: Every node has node_type in {section, setting}
# ---------------------------------------------------------------------------


def check_node_types_present(path, schema):
    """
    Check that every non-root node has a 'node_type' field set to 'section' or 'setting'.

    Returns a list of error strings.
    """
    errors = []
    for node_path, node in walk_nodes(schema):
        node_type = node.get("node_type")
        if node_type not in VALID_NODE_TYPES:
            if node_type is None:
                errors.append(
                    f"{path}: [{node_path}] Node is missing the 'node_type' field. "
                    f"Fix: add 'node_type: setting' for leaf settings or "
                    f"'node_type: section' for groups."
                )
            else:
                errors.append(
                    f"{path}: [{node_path}] Node has invalid node_type='{node_type}'. "
                    f"Must be one of: {sorted(VALID_NODE_TYPES)}. "
                    f"Fix: set 'node_type' to either 'setting' or 'section'."
                )
    return errors


# ---------------------------------------------------------------------------
# Check 6: Every setting has a default value (with exception list)
# ---------------------------------------------------------------------------


def check_settings_have_default(path, schema):
    """
    Check that every setting node has a 'default' or 'platform_default' field.

    Returns a list of error strings.
    """
    errors = []
    for node_path, node in walk_nodes(schema):
        if node.get("node_type") != "setting":
            continue
        has_default = "default" in node or "platform_default" in node

        if not has_default:
            errors.append(
                f"{path}: [{node_path}] Setting has no default value. "
                f"Fix: add a 'default' or 'platform_default' field. "
            )
    return errors


# ---------------------------------------------------------------------------
# Check 7: Every setting has a type (with exception list)
# ---------------------------------------------------------------------------


def check_settings_have_type(path, schema):
    """
    Check that every setting node has a 'type' field.

    Returns a list of error strings.
    """
    errors = []
    for node_path, node in walk_nodes(schema):
        if node.get("node_type") != "setting":
            continue
        has_type = "type" in node
        if not has_type:
            errors.append(
                f"{path}: [{node_path}] Setting has no 'type' field. "
                f"Fix: add a 'type' field (one of: {sorted(VALID_TYPES)}). "
            )
    return errors


# ---------------------------------------------------------------------------
# Check 8: platform_default key validation
# ---------------------------------------------------------------------------


def check_platform_default_keys(path, schema):
    """
    Check that 'platform_default' nodes only use valid platform keys and that
    either 'other' is present or all of darwin/windows/linux are present.

    Returns a list of error strings.
    """
    errors = []
    for node_path, node in walk_nodes(schema):
        pd = node.get("platform_default")
        if pd is None:
            continue
        if not isinstance(pd, dict):
            errors.append(
                f"{path}: [{node_path}] 'platform_default' must be a mapping. "
                f"Fix: ensure platform_default is a YAML mapping with platform keys."
            )
            continue

        keys = set(pd.keys())
        unknown = keys - VALID_PLATFORM_KEYS
        if unknown:
            errors.append(
                f"{path}: [{node_path}] 'platform_default' contains unknown platform key(s): "
                f"{sorted(unknown)}. "
                f"Allowed keys: {sorted(VALID_PLATFORM_KEYS)}. "
                f"Fix: remove or rename the invalid key(s)."
            )

        if "other" not in keys:
            missing = REQUIRED_PLATFORM_KEYS_WITHOUT_OTHER - keys
            if missing:
                errors.append(
                    f"{path}: [{node_path}] 'platform_default' has no 'other' key, so "
                    f"{sorted(REQUIRED_PLATFORM_KEYS_WITHOUT_OTHER)} are all required, "
                    f"but {sorted(missing)} are missing. "
                    f"Fix: add the missing platform key(s) or add an 'other' key as a catch-all."
                )
    return errors


# ---------------------------------------------------------------------------
# Check 9: Every section has at least one child
# ---------------------------------------------------------------------------


def check_sections_have_children(path, schema):
    """
    Check that every section node has a non-empty 'properties' mapping.

    A section with no children is structurally invalid regardless of visibility.

    Returns a list of error strings.
    """
    errors = []
    for node_path, node in walk_nodes(schema):
        if node.get("node_type") != "section":
            continue
        props = node.get("properties")
        if not isinstance(props, dict) or len(props) == 0:
            errors.append(
                f"{path}: [{node_path}] Section has no children (empty or missing 'properties'). "
                f"Fix: add at least one child setting or sub-section, or remove the section."
            )
    return errors


# ---------------------------------------------------------------------------
# Check 10: Public sections have at least one direct public child
# ---------------------------------------------------------------------------


def check_public_section_has_public_child(path, schema):
    """
    Check that every public section has at least one direct child with
    visibility='public'.

    A public section with only private children is meaningless from a
    documentation perspective.

    Returns a list of error strings.
    """
    errors = []
    for node_path, node in walk_nodes(schema):
        if node.get("node_type") != "section":
            continue
        if node.get("visibility") != "public":
            continue
        props = node.get("properties") or {}
        has_public_child = any(
            isinstance(child, dict) and child.get("visibility") == "public" for child in props.values()
        )
        if not has_public_child:
            errors.append(
                f"{path}: [{node_path}] Public section has no direct public child. "
                f"Fix: set 'visibility: public' on at least one direct child setting or "
                f"sub-section, or remove 'visibility: public' from this section."
            )
    return errors


# ---------------------------------------------------------------------------
# Check 11: Text fields (description, example, title, banner) must not contain
#           literal \\n escape sequences and use scalars instead
# ---------------------------------------------------------------------------

_TEXT_LINEBREAK_RE = re.compile(r'(?m)^\s*(description|example|title|banner):\s*"[^"]*\\n')


def check_text_scalar_mode(path):
    """
    Check that 'description', 'example', 'title' and 'banner' fields do not contain
    literal \\n escape sequences in double-quoted YAML strings.

    Reads the raw file (no YAML parsing) and searches for the literal two-character
    sequence backslash-n inside double-quoted values for those fields.

    Returns a list of error strings.
    """
    with open(path) as f:
        raw = f.read()
    errors = []
    for m in _TEXT_LINEBREAK_RE.finditer(raw):
        field = m.group(1)
        lineno = raw[: m.start()].count("\n") + 1
        errors.append(
            f"{path}:{lineno}: Field '{field}' contains a literal \\n escape sequence. "
            f"Fix: remove the embedded \\n and use a literal style scalar instead to "
            f"improve readability of the schema (see: https://yaml.org/spec/1.2.2/#literal-style)."
        )
    return errors


# ---------------------------------------------------------------------------
# Check 12: Relative defaults use known variables
# ---------------------------------------------------------------------------

VALID_RELATIVE_DEFAULT_VARS = {"run_path", "install_path", "conf_path", "log_path"}


def _check_relative_value(path, node_path, value):
    """Check a single default value for well-formed and known ${var} placeholders."""
    errors = []
    if not isinstance(value, str) or "${" not in value:
        return errors

    pos = 0
    while True:
        idx = value.find("${", pos)
        if idx == -1:
            break
        close = value.find("}", idx + 2)
        if close == -1:
            errors.append(
                f"{path}: [{node_path}] Default value contains an unclosed '${{' "
                f"placeholder: '{value}'. "
                f"Fix: close the placeholder with '}}'."
            )
            break
        var_name = value[idx + 2 : close]
        if var_name not in VALID_RELATIVE_DEFAULT_VARS:
            errors.append(
                f"{path}: [{node_path}] Default value uses unknown variable "
                f"'${{{var_name}}}' in '{value}'. "
                f"Allowed variables: {sorted(VALID_RELATIVE_DEFAULT_VARS)}. "
                f"Fix: use one of the allowed variables."
            )
        pos = close + 1

    return errors


def check_relative_defaults(path, schema):
    """
    Check that relative default values (those containing ${var} placeholders) use
    only known variables and have well-formed syntax.

    Applies to both 'default' and each value inside 'platform_default'.

    Returns a list of error strings.
    """
    errors = []
    for node_path, node in walk_nodes(schema):
        if node.get("node_type") != "setting":
            continue

        errors.extend(_check_relative_value(path, node_path, node.get("default")))

        platform_default = node.get("platform_default")
        if isinstance(platform_default, dict):
            for platform_key, platform_value in platform_default.items():
                errors.extend(
                    _check_relative_value(
                        path,
                        f"{node_path}[platform_default.{platform_key}]",
                        platform_value,
                    )
                )

    return errors


# ---------------------------------------------------------------------------
# Check 13: env_parser value validation
# ---------------------------------------------------------------------------


def check_env_parser(path, schema):
    """
    Check that 'env_parser' fields:
      - only appear on setting nodes (not section nodes)
      - use a recognised value

    Returns a list of error strings.
    """
    errors = []
    for node_path, node in walk_nodes(schema):
        env_parser = node.get("env_parser")
        if env_parser is None:
            continue
        if node.get("node_type") == "section":
            errors.append(
                f"{path}: [{node_path}] 'env_parser' is not valid on section nodes. "
                f"Fix: remove 'env_parser' from this section node."
            )
            continue
        if env_parser not in VALID_ENV_PARSERS:
            errors.append(
                f"{path}: [{node_path}] Invalid 'env_parser' value '{env_parser}'. "
                f"Must be one of: {sorted(VALID_ENV_PARSERS)}. "
                f"Fix: change 'env_parser' to a valid value."
            )
    return errors


# ---------------------------------------------------------------------------
# Check 14: generate_const tag validation
# ---------------------------------------------------------------------------

GENERATE_CONST_PREFIX = "generate_const:"
# The constant name must start with an ASCII letter (any case) and otherwise be composed only of
# ASCII letters and digits.
GENERATE_CONST_NAME_RE = re.compile(r"^[A-Za-z][A-Za-z0-9]*$")


def check_generate_const_tag(path, schema):
    """
    Check that 'generate_const:<name>' tags:
      - only appear on setting nodes (not section nodes)
      - have the form 'generate_const:<name>' where <name> starts with a letter and is
        otherwise composed only of ASCII letters (lower and upper case) and digits

    Returns a list of error strings.
    """
    errors = []
    for node_path, node in walk_nodes(schema):
        for tag in get_tags(node):
            if not isinstance(tag, str) or not tag.startswith(GENERATE_CONST_PREFIX):
                continue
            if node.get("node_type") != "setting":
                errors.append(
                    f"{path}: [{node_path}] '{tag}' tag is only valid on setting nodes, not sections. "
                    f"Fix: remove the '{GENERATE_CONST_PREFIX}...' tag from this section node."
                )
                continue
            name = tag[len(GENERATE_CONST_PREFIX) :]
            if not GENERATE_CONST_NAME_RE.match(name):
                errors.append(
                    f"{path}: [{node_path}] Invalid tag '{tag}'. "
                    f"The name must have the form '{GENERATE_CONST_PREFIX}<name>' where <name> starts "
                    f"with a letter and is otherwise composed only of letters and digits. "
                    f"Fix: use a valid Go constant name after '{GENERATE_CONST_PREFIX}'."
                )
    return errors


# ---------------------------------------------------------------------------
# Check 15: renamed_from validation
# ---------------------------------------------------------------------------

# A full semantic Agent version: 'MAJOR.MINOR.BUGFIX' (for example '7.71.0').
RENAMED_FROM_VERSION_RE = re.compile(r"^\d+\.\d+\.\d+$")


def check_renamed_from(path, schema):
    """
    Check that 'renamed_from' fields:
      - only appear on setting nodes
      - are a mapping of former name -> Agent version that deprecated that name
      - contain at least one entry
      - only use non-empty strings as former names
      - only use quoted, full semantic versions ('MAJOR.MINOR.BUGFIX') as versions
      - never reuse the same version for two former names of the same setting

    Every former name is a fully qualified name of the setting.

    Returns a list of error strings.
    """
    errors = []
    for node_path, node in walk_nodes(schema):
        if "renamed_from" not in node:
            continue

        if node.get("node_type") != "setting":
            errors.append(
                f"{path}: [{node_path}] 'renamed_from' is only allowed on setting nodes, "
                f"not on sections. "
                f"Fix: remove 'renamed_from' from this section and add it to each renamed "
                f"setting it contains, using their former fully qualified names."
            )
            continue

        renamed_from = node["renamed_from"]

        if not isinstance(renamed_from, dict):
            errors.append(
                f"{path}: [{node_path}] 'renamed_from' must be a mapping of former name to the "
                f"Agent version that deprecated it, got {type(renamed_from).__name__}. "
                f"Fix: use a YAML mapping, e.g. 'renamed_from: {{old_name: \"7.71.0\"}}'."
            )
            continue

        if len(renamed_from) == 0:
            errors.append(
                f"{path}: [{node_path}] 'renamed_from' is empty. "
                f"Fix: list at least one former name of this setting, or remove 'renamed_from'."
            )
            continue

        names_by_version = {}
        for name, version in renamed_from.items():
            if not isinstance(name, str) or not name.strip():
                errors.append(
                    f"{path}: [{node_path}] 'renamed_from' contains an invalid former name '{name}'. "
                    f"Fix: every key must be a non-empty string holding a former "
                    f"fully qualified name of this setting."
                )
                continue

            if not isinstance(version, str):
                errors.append(
                    f"{path}: [{node_path}] 'renamed_from' entry '{name}' has a non-string version "
                    f"'{version}' ({type(version).__name__}). "
                    f"Fix: quote the Agent version so YAML keeps it as a string, "
                    f"e.g. '{name}: \"7.71.0\"'."
                )
                continue

            if not RENAMED_FROM_VERSION_RE.match(version):
                errors.append(
                    f"{path}: [{node_path}] 'renamed_from' entry '{name}' has an invalid version "
                    f"'{version}'. "
                    f"Fix: use the full semantic Agent version that deprecated this name, as "
                    f"'MAJOR.MINOR.BUGFIX', e.g. '{name}: \"7.71.0\"'."
                )
                continue

            # Versions are what order the renames, so each one must identify a single rename.
            names_by_version.setdefault(tuple(int(part) for part in version.split(".")), []).append(name)

        for version, names in names_by_version.items():
            if len(names) > 1:
                shared = ", ".join(f"'{name}'" for name in sorted(names))
                errors.append(
                    f"{path}: [{node_path}] 'renamed_from' reuses version "
                    f"'{'.'.join(str(part) for part in version)}' for {shared}. "
                    f"Fix: a setting cannot be renamed twice in the same Agent version. Give each "
                    f"former name the version that deprecated it, so the renames stay ordered."
                )
    return errors


# ---------------------------------------------------------------------------
# Check 16: setting and section names are snake_case
# ---------------------------------------------------------------------------

# Lowercase ASCII letters and digits, in words separated by single underscores.
SNAKE_CASE_RE = re.compile(r"^[a-z0-9]+(_[a-z0-9]+)*$")


def check_snake_case_names(path, schema, non_snake_case_exceptions=None):
    """
    Check that the name of every node (setting or section) is snake_case: lowercase
    ASCII letters and digits, in words separated by single underscores.

    Only the node's own name is checked, its ancestors are reported on their own.
    *non_snake_case_exceptions* is a set of dotted paths exempt from this check.

    Returns a list of error strings.
    """
    if non_snake_case_exceptions is None:
        non_snake_case_exceptions = set()
    errors = []
    for node_path, _ in walk_nodes(schema):
        name = node_path.rsplit(".", 1)[-1]
        if SNAKE_CASE_RE.match(name) or node_path in non_snake_case_exceptions:
            continue
        errors.append(
            f"{path}: [{node_path}] Name '{name}' is not snake_case. "
            f"Fix: only use lowercase letters and digits, in words separated by single "
            f"underscores, e.g. 'my_setting_name'."
        )
    return errors


# ---------------------------------------------------------------------------
# Check 17: template_section tags are only used on public nodes
# ---------------------------------------------------------------------------

TEMPLATE_SECTION_PREFIX = "template_section:"


def check_template_section_on_public(path, schema):
    """
    Check that 'template_section:<name>' tags only appear on nodes with
    visibility='public'.

    The config template only renders public nodes, so the tag has no effect on
    private ones.

    Returns a list of error strings.
    """
    errors = []
    for node_path, node in walk_nodes(schema):
        if node.get("visibility") == "public":
            continue
        for tag in get_tags(node):
            if isinstance(tag, str) and tag.startswith(TEMPLATE_SECTION_PREFIX):
                errors.append(
                    f"{path}: [{node_path}] '{tag}' tag is only valid on public nodes. "
                    f"Fix: remove the '{TEMPLATE_SECTION_PREFIX}...' tag or set 'visibility: public'."
                )
    return errors


# ---------------------------------------------------------------------------
# Check 18: sensitive field validation
# ---------------------------------------------------------------------------


def check_sensitive(path, schema):
    """
    Check that 'sensitive' fields:
      - only appear on setting nodes (not section nodes)
      - are booleans

    Returns a list of error strings.
    """
    errors = []
    for node_path, node in walk_nodes(schema):
        if "sensitive" not in node:
            continue
        if node.get("node_type") == "section":
            errors.append(
                f"{path}: [{node_path}] 'sensitive' is not valid on section nodes. "
                f"Fix: remove 'sensitive' from this section node and set it on each setting holding a credential."
            )
            continue
        if not isinstance(node["sensitive"], bool):
            errors.append(
                f"{path}: [{node_path}] 'sensitive' must be a boolean, got '{node['sensitive']}'. "
                f"Fix: set 'sensitive: true', or remove it."
            )
    return errors


# ---------------------------------------------------------------------------
# Check 19: no setting accepts null
# ---------------------------------------------------------------------------
#
# Why null is a problem: the Agent configuration has no null value.
#
#   - An empty or null value in a configuration file (`infrastructure_mode:`)
#     is treated as "not set": the setting keeps its default, IsConfigured()
#     returns false and the value is not even passed to schema validation.
#     Allowing null in the schema documents a value the Agent never holds.
#
#   - In JSON Schema, null is a type of its own: a `type: string` setting
#     rejects null before `enum` is checked. A null in such an enum is dead: the
#     Agent's validator, IDEs and every other JSON Schema tool still reject
#     null ("got null, want string"). It also misleads users, since validation
#     errors list it as an allowed value ("must be one of 'basic', ..., <nil>").
#
#   - Actually accepting null would need `type: [string, "null"]`, and it would
#     still mean nothing: the Agent treats that null as "not set" (see above),
#     exactly like omitting the setting.
#
# To describe "no value", omit the setting or rely on its default.
#
# The two exceptions in lint_exceptions.yaml (`null_value`) predate this check:
# they were copied from the remote-config validator (#58436) and are already
# rejected by their `type`, so removing them changes nothing at runtime. No new
# exception may be added: an exception can't make null work, it can only add a
# misleading value. Remove entries instead; tests enforce the list only shrinks.

# Keywords whose value is instance data rather than a schema: never walked.
_NULL_CHECK_INSTANCE_KEYS = frozenset({"default", "const", "enum", "examples", "example"})


def _find_null_values(node, node_path):
    """
    Yield (location, reason) for every way the value schema *node* accepts null:
    an `enum` listing null, `const: null`, or a `type` that is or includes "null".

    Walks every sub-schema of the value (`items`, `properties`,
    `additionalProperties`, `anyOf`, ...), but never the instance data under
    `default`/`enum`/`examples`, whose content is a config value, not a schema.
    """
    if isinstance(node, list):
        for i, item in enumerate(node):
            yield from _find_null_values(item, f"{node_path}[{i}]")
        return
    if not isinstance(node, dict):
        return
    enum = node.get("enum")
    if isinstance(enum, list) and None in enum:
        yield node_path, "'enum' lists null"
    if "const" in node and node["const"] is None:
        yield node_path, "'const' is null"
    type_val = node.get("type")
    types = type_val if isinstance(type_val, list) else [type_val]
    if "null" in types or ("type" in node and None in types):
        yield node_path, f"'type' accepts null ({type_val!r})"
    for key, value in node.items():
        if key in _NULL_CHECK_INSTANCE_KEYS:
            continue
        if key in ("properties", "patternProperties", "$defs", "dependentSchemas") and isinstance(value, dict):
            for name, sub in value.items():
                yield from _find_null_values(sub, f"{node_path}.{key}.{name}")
        elif isinstance(value, dict | list):
            yield from _find_null_values(value, f"{node_path}.{key}")


def check_no_null_values(path, schema, null_value_exceptions=None):
    """
    Check that no setting accepts null as a value. See the comment above for why
    null is a problem and why no new exception may be granted.

    *null_value_exceptions* is a set of dotted setting paths exempt from this check.

    Returns a list of error strings.
    """
    if null_value_exceptions is None:
        null_value_exceptions = set()
    errors = []
    for node_path, node in walk_nodes(schema):
        if node_path in null_value_exceptions:
            continue
        if node.get("node_type") == "section":
            # A section's 'properties' are its child nodes, which walk_nodes yields on their own: only check the
            # section's own keywords.
            node = {k: v for k, v in node.items() if k != "properties"}
        for location, reason in _find_null_values(node, node_path):
            errors.append(
                f"{path}: [{location}] {reason}, but a setting can't be null: an empty or null value in a "
                f"configuration file is treated as 'not set' and the setting keeps its default. "
                f"Fix: remove null; to describe 'no value', rely on the setting's default. "
                f"Do not add an exception, see check_no_null_values in tasks/schema/lint.py."
            )
    return errors


# ---------------------------------------------------------------------------
# Check 20: settings use 'banner', not 'title', to start a template banner
# ---------------------------------------------------------------------------


def check_no_title_on_settings(path, schema):
    """
    Check that no setting node has a 'title'.

    A banner in the generated config example (datadog.yaml.example) is set with
    the Agent's 'banner' keyword. 'title' is a standard JSON Schema keyword: the
    short label of a value, which IDEs and other external tools show when
    hovering or completing a setting. On a setting used to start a group of
    settings, it would label that one setting with the group's name (e.g.
    'api_key' shown as "Basic Configuration"). Sections may keep a 'title': it
    is both their banner and a correct label for them.

    Returns a list of error strings.
    """
    errors = []
    for node_path, node in walk_nodes(schema):
        if node.get("node_type") == "setting" and "title" in node:
            errors.append(
                f"{path}: [{node_path}] Setting has a 'title' ({node['title']!r}). 'title' is the label IDEs show "
                f"for this setting's value, not a banner. "
                f"Fix: to start a banner in the config example, rename 'title' to 'banner'; "
                f"otherwise remove it and document the setting with 'description'."
            )
    return errors


# ---------------------------------------------------------------------------
# Check 21: 'format' names an Agent format, on a string
# ---------------------------------------------------------------------------


def _iter_value_schemas(node, node_path):
    """
    Yield (location, schema) for *node* and every sub-schema of its value
    (`items`, `properties` of an object value, `anyOf`, ...), never walking the
    instance data under `default`/`enum`/`examples`.
    """
    if isinstance(node, list):
        for i, item in enumerate(node):
            yield from _iter_value_schemas(item, f"{node_path}[{i}]")
        return
    if not isinstance(node, dict):
        return
    yield node_path, node
    for key, value in node.items():
        if key in _NULL_CHECK_INSTANCE_KEYS:
            continue
        if key in ("properties", "patternProperties", "$defs", "dependentSchemas") and isinstance(value, dict):
            for name, sub in value.items():
                yield from _iter_value_schemas(sub, f"{node_path}.{key}.{name}")
        elif isinstance(value, dict | list):
            yield from _iter_value_schemas(value, f"{node_path}.{key}")


def check_formats(path, schema):
    """
    Check that every 'format' is one of the Agent formats (AGENT_FORMATS in
    produce_byproduct.py), used on a string.

    The published and embedded schemas replace an Agent format with its pattern;
    any other format would be kept as is, and silently not validated by most
    JSON Schema tools (a typo like 'format: uri' would disable the validation).

    Returns a list of error strings.
    """
    errors = []
    for node_path, node in walk_nodes(schema):
        if node.get("node_type") == "section":
            # A section's 'properties' are its child nodes, which walk_nodes yields on their own.
            node = {k: v for k, v in node.items() if k != "properties"}
        for location, sub in _iter_value_schemas(node, node_path):
            if "format" not in sub:
                continue
            name = sub["format"]
            if name not in AGENT_FORMATS:
                errors.append(
                    f"{path}: [{location}] Unknown format {name!r}. "
                    f"Fix: use one of {sorted(AGENT_FORMATS)}, or a 'pattern'; to add a format, "
                    f"register it in AGENT_FORMATS in tasks/schema/produce_byproduct.py."
                )
            elif sub.get("type") != "string":
                errors.append(
                    f"{path}: [{location}] 'format: {name}' only applies to strings, but the type is "
                    f"{sub.get('type')!r}. Fix: set 'type: string' or remove 'format'."
                )
    return errors


# ---------------------------------------------------------------------------
# Exception list loading
# ---------------------------------------------------------------------------


def load_exceptions(exceptions_file=EXCEPTIONS_FILE):
    """
    Load the exception lists from lint_exceptions.yaml.

    Returns a dict with keys:
      - array_no_items: set of dotted paths
      - non_snake_case: set of dotted paths
      - null_value: set of dotted paths
    """
    if not os.path.isfile(exceptions_file):
        return {"array_no_items": set(), "non_snake_case": set(), "null_value": set()}
    with open(exceptions_file) as f:
        data = yaml.safe_load(f) or {}
    return {
        "array_no_items": set(data.get("array_no_items", []) or []),
        "non_snake_case": set(data.get("non_snake_case", []) or []),
        "null_value": set(data.get("null_value", []) or []),
    }


# ---------------------------------------------------------------------------
# Invoke task
# ---------------------------------------------------------------------------


@task
def lint(ctx, schema_dir=SCHEMA_DIR, exceptions_file=EXCEPTIONS_FILE):
    """
    Lint all *_schema.yaml files in schema_dir against the schema quality rules.

    Exits non-zero if any violations are found.
    """
    schema_files = sorted(glob.glob(os.path.join(schema_dir, "*_schema.yaml")))
    if not schema_files:
        print(f"No schema files found in {schema_dir}")
        raise Exit(code=1)

    exc = load_exceptions(exceptions_file)

    all_errors = []

    for schema_path in schema_files:
        print(f"Linting {schema_path}...")

        # Check 1: YAML validity (also loads the schema for subsequent checks)
        yaml_errors = check_yaml_valid(schema_path)
        if yaml_errors:
            all_errors.extend(yaml_errors)
            # Cannot continue linting an unparseable file
            continue

        # Use resolve_schema so that lint checks see the fully merged content
        # (split sub-files inlined). Linting operates on the logical schema,
        # not on the on-disk fragments.
        schema = resolve_schema(schema_path)

        all_errors.extend(check_json_schema_structure(schema_path, schema, exc["array_no_items"]))
        all_errors.extend(check_public_descriptions(schema_path, schema))
        all_errors.extend(check_public_parent_sections(schema_path, schema))
        all_errors.extend(check_node_types_present(schema_path, schema))
        all_errors.extend(check_settings_have_default(schema_path, schema))
        all_errors.extend(check_settings_have_type(schema_path, schema))
        all_errors.extend(check_platform_default_keys(schema_path, schema))
        all_errors.extend(check_sections_have_children(schema_path, schema))
        all_errors.extend(check_public_section_has_public_child(schema_path, schema))
        all_errors.extend(check_text_scalar_mode(schema_path))
        all_errors.extend(check_relative_defaults(schema_path, schema))
        all_errors.extend(check_env_parser(schema_path, schema))
        all_errors.extend(check_generate_const_tag(schema_path, schema))
        all_errors.extend(check_renamed_from(schema_path, schema))
        all_errors.extend(check_snake_case_names(schema_path, schema, exc["non_snake_case"]))
        all_errors.extend(check_template_section_on_public(schema_path, schema))
        all_errors.extend(check_sensitive(schema_path, schema))
        all_errors.extend(check_no_null_values(schema_path, schema, exc["null_value"]))
        all_errors.extend(check_no_title_on_settings(schema_path, schema))
        all_errors.extend(check_formats(schema_path, schema))

    if all_errors:
        print(f"\nFound {len(all_errors)} schema linting error(s):\n")
        for error in all_errors:
            print(f"  ERROR: {error}")
        print(f"\n{SLACK_HINT}")
        raise Exit(code=1)

    print(f"\nAll {len(schema_files)} schema file(s) passed linting.")
