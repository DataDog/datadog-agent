"""
Schema linter for the Datadog Agent configuration schemas.

Validates generated YAML schema files (pkg/config/schema/*.yaml) against
a set of quality rules. Run with:

    dda inv schema.lint
"""

import glob
import os

import yaml
from invoke import task
from invoke.exceptions import Exit

SCHEMA_DIR = os.path.join("pkg", "config", "schema")
EXCEPTIONS_FILE = os.path.join(os.path.dirname(__file__), "lint_exceptions.yaml")

VALID_TYPES = {"string", "number", "boolean", "array", "object"}
VALID_NODE_TYPES = {"section", "setting"}
VALID_PLATFORM_KEYS = {"darwin", "windows", "linux", "container", "other"}
REQUIRED_PLATFORM_KEYS_WITHOUT_OTHER = {"darwin", "windows", "linux"}

SLACK_HINT = "If you have any question please reach out on #agent-configuration"


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


def check_settings_have_default(path, schema, no_default_exceptions=None):
    """
    Check that every setting node has a 'default' or 'platform_default' field.

    Settings in *no_default_exceptions* (a set of dotted paths) are allowed to
    skip the default **only if** they also carry the 'TODO:fix-no-default' tag.
    A setting in the exception list without the required tag is still an error.

    Returns a list of error strings.
    """
    if no_default_exceptions is None:
        no_default_exceptions = set()
    errors = []
    for node_path, node in walk_nodes(schema):
        if node.get("node_type") != "setting":
            continue
        has_default = "default" in node or "platform_default" in node
        tags = get_tags(node)
        in_exceptions = node_path in no_default_exceptions

        if in_exceptions:
            # Accept TODO:fix-missing-type as an equivalent marker: settings registered
            # with BindEnvAndSetDefault(key, nil) have no meaningful default and no
            # derivable type; the builder tags them with TODO:fix-missing-type (and
            # TODO:fix-no-default after the next schema regeneration with the updated
            # builder). Either tag satisfies the requirement.
            has_marker = "TODO:fix-no-default" in tags or "TODO:fix-missing-type" in tags
            if not has_marker:
                errors.append(
                    f"{path}: [{node_path}] Setting is in the no-default exception list but is missing "
                    f"the 'TODO:fix-no-default' tag (or 'TODO:fix-missing-type' as equivalent). "
                    f"Fix: add 'TODO:fix-no-default' to the setting's tags list."
                )
            continue

        if not has_default:
            errors.append(
                f"{path}: [{node_path}] Setting has no default value. "
                f"Fix: add a 'default' or 'platform_default' field. "
                f"If this setting genuinely cannot have a default, add it to "
                f"'tasks/schema/lint_exceptions.yaml' under 'no_default' and add the "
                f"'TODO:fix-no-default' tag to the setting."
            )
    return errors


# ---------------------------------------------------------------------------
# Check 7: Every setting has a type (with exception list)
# ---------------------------------------------------------------------------


def check_settings_have_type(path, schema, no_type_exceptions=None):
    """
    Check that every setting node has a 'type' field.

    Settings in *no_type_exceptions* (a set of dotted paths) are allowed to
    skip the type **only if** they also carry the 'TODO:fix-missing-type' tag.

    Returns a list of error strings.
    """
    if no_type_exceptions is None:
        no_type_exceptions = set()
    errors = []
    for node_path, node in walk_nodes(schema):
        if node.get("node_type") != "setting":
            continue
        has_type = "type" in node
        tags = get_tags(node)
        in_exceptions = node_path in no_type_exceptions

        if in_exceptions:
            # Accept TODO:fix-no-default as an equivalent marker: settings registered
            # with BindEnv have no default and no derivable type; the builder tags them
            # with TODO:fix-no-default (and TODO:fix-missing-type after the next schema
            # regeneration with the updated builder). Either tag satisfies the requirement.
            has_marker = "TODO:fix-missing-type" in tags or "TODO:fix-no-default" in tags
            if not has_marker:
                errors.append(
                    f"{path}: [{node_path}] Setting is in the no-type exception list but is missing "
                    f"the 'TODO:fix-missing-type' tag (or 'TODO:fix-no-default' as equivalent). "
                    f"Fix: add 'TODO:fix-missing-type' to the setting's tags list."
                )
            continue

        if not has_type:
            errors.append(
                f"{path}: [{node_path}] Setting has no 'type' field. "
                f"Fix: add a 'type' field (one of: {sorted(VALID_TYPES)}). "
                f"If the type genuinely cannot be determined, add this setting to "
                f"'tasks/schema/lint_exceptions.yaml' under 'no_type' and add the "
                f"'TODO:fix-missing-type' tag to the setting."
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
# Exception list loading
# ---------------------------------------------------------------------------


def load_exceptions(exceptions_file=EXCEPTIONS_FILE):
    """
    Load the exception lists from lint_exceptions.yaml.

    Returns a dict with keys:
      - no_default: set of dotted paths (require TODO:fix-no-default tag)
      - no_type: set of dotted paths (require TODO:fix-missing-type tag)
      - array_no_items: set of dotted paths
    """
    if not os.path.isfile(exceptions_file):
        return {k: set() for k in ("no_default", "no_type", "array_no_items")}
    with open(exceptions_file) as f:
        data = yaml.safe_load(f) or {}
    return {
        "no_default": set(data.get("no_default", []) or []),
        "no_type": set(data.get("no_type", []) or []),
        "array_no_items": set(data.get("array_no_items", []) or []),
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

        with open(schema_path) as f:
            schema = yaml.safe_load(f)

        # Checks 2–8
        all_errors.extend(check_json_schema_structure(schema_path, schema, exc["array_no_items"]))
        all_errors.extend(check_public_descriptions(schema_path, schema))
        all_errors.extend(check_public_parent_sections(schema_path, schema))
        all_errors.extend(check_node_types_present(schema_path, schema))
        all_errors.extend(check_settings_have_default(schema_path, schema, exc["no_default"]))
        all_errors.extend(check_settings_have_type(schema_path, schema, exc["no_type"]))
        all_errors.extend(check_platform_default_keys(schema_path, schema))

    if all_errors:
        print(f"\nFound {len(all_errors)} schema linting error(s):\n")
        for error in all_errors:
            print(f"  ERROR: {error}")
        print(f"\n{SLACK_HINT}")
        raise Exit(code=1)

    print(f"\nAll {len(schema_files)} schema file(s) passed linting.")
