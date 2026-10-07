"""
Schema linter for the Datadog Agent configuration schemas.

Validates generated YAML schema files (pkg/config/schema/yaml/*.yaml) against
a set of quality rules. Run with:

    dda inv schema.lint
"""

import glob
import json
import os
import re

import yaml
from invoke import task
from invoke.exceptions import Exit

from tasks.schema.merge_schema import resolve_schema

SCHEMA_DIR = os.path.join("pkg", "config", "schema", "yaml")
CORE_SCHEMA_FILE = "core_schema.yaml"
EXCEPTIONS_FILE = os.path.join(os.path.dirname(__file__), "lint_exceptions.yaml")

VALID_TYPES = {"string", "number", "integer", "boolean", "array", "object"}
VALID_NODE_TYPES = {"section", "setting"}
VALID_PLATFORM_KEYS = {"darwin", "windows", "linux", "aix", "container", "kubernetes", "fargate", "other"}
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
# Check 11: Text fields (description, example, title) must not contain
#           literal \\n escape sequences and use scalars instead
# ---------------------------------------------------------------------------

_TEXT_LINEBREAK_RE = re.compile(r'(?m)^\s*(description|example|title):\s*"[^"]*\\n')


def check_text_scalar_mode(path):
    """
    Check that 'description', 'example', and 'title' fields do not contain
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
# Check 16: Product enablement definitions ('sku_definitions' / 'product_dependencies')
# ---------------------------------------------------------------------------

PRODUCT_DEFINITION_KEYS = ("sku_definitions", "product_dependencies")


def get_product_definitions(schema):
    """
    Return the (sku_definitions, product_dependencies) maps of a schema, keeping only well-formed
    entries (string name -> list of strings) so later checks can rely on them.
    """
    maps = []
    for key in PRODUCT_DEFINITION_KEYS:
        value = schema.get(key)
        if not isinstance(value, dict):
            maps.append({})
            continue
        maps.append(
            {
                name: entries
                for name, entries in value.items()
                if isinstance(name, str) and isinstance(entries, list) and all(isinstance(e, str) for e in entries)
            }
        )
    return maps[0], maps[1]


def _find_dependency_cycles(product_dependencies):
    """Return every dependency cycle as a list of product names, first name repeated at the end."""
    cycles = []
    seen_cycles = set()
    state = {}  # product -> "visiting" | "done"

    def visit(product, stack):
        state[product] = "visiting"
        stack.append(product)
        for dependency in product_dependencies.get(product, []):
            if state.get(dependency) == "visiting":
                cycle = stack[stack.index(dependency) :] + [dependency]
                # Report each cycle once, whatever product it was entered from
                key = frozenset(cycle)
                if key not in seen_cycles:
                    seen_cycles.add(key)
                    cycles.append(cycle)
            elif dependency in product_dependencies and dependency not in state:
                visit(dependency, stack)
        stack.pop()
        state[product] = "done"

    for product in sorted(product_dependencies):
        if product not in state:
            visit(product, [])
    return cycles


def check_product_definitions(path, schema, is_core=True):
    """
    Check the product enablement definitions declared at the top level of the core schema:
      - 'sku_definitions' maps a SKU to the products it bundles.
      - 'product_dependencies' maps every product to the products it depends on.

    Rules:
      - Both must be mappings of name -> list of product names.
      - Only the core schema can declare them.
      - A name can't be both a SKU and a product.
      - Every referenced product must be declared in 'product_dependencies', and can't be a SKU.
      - Dependencies can't form a cycle.

    Returns a list of error strings.
    """
    errors = []

    if not is_core:
        for key in PRODUCT_DEFINITION_KEYS:
            if key in schema:
                errors.append(
                    f"{path}: '{key}' can only be declared in the core schema. "
                    f"Fix: move it to core_schema.yaml, product defaults from this schema can reference it."
                )
        return errors

    for key in PRODUCT_DEFINITION_KEYS:
        if key not in schema:
            continue
        value = schema[key]
        if not isinstance(value, dict):
            errors.append(
                f"{path}: '{key}' must be a mapping of names to lists of product names. "
                f"Fix: use '{key}: {{name: [product, ...]}}'."
            )
            continue
        for name, entries in value.items():
            if not isinstance(entries, list) or not all(isinstance(e, str) for e in entries):
                errors.append(
                    f"{path}: [{key}.{name}] must be a list of product names. "
                    f"Fix: use '{name}: [product, ...]' (or '{name}: []')."
                )

    sku_definitions, product_dependencies = get_product_definitions(schema)

    for name in sorted(set(sku_definitions) & set(product_dependencies)):
        errors.append(
            f"{path}: '{name}' is declared as both a SKU and a product. "
            f"Fix: rename the SKU in 'sku_definitions' or the product in 'product_dependencies'."
        )

    for key, definitions in (("sku_definitions", sku_definitions), ("product_dependencies", product_dependencies)):
        for name, entries in definitions.items():
            for entry in entries:
                if entry in sku_definitions:
                    errors.append(
                        f"{path}: [{key}.{name}] '{entry}' is a SKU. "
                        f"Fix: SKUs and products can only reference products, list the products of '{entry}' instead."
                    )
                elif entry not in product_dependencies:
                    errors.append(
                        f"{path}: [{key}.{name}] product '{entry}' is not declared. "
                        f"Fix: add '{entry}: [...]' to 'product_dependencies'."
                    )

    for cycle in _find_dependency_cycles(product_dependencies):
        errors.append(
            f"{path}: 'product_dependencies' contains a cycle: {' -> '.join(cycle)}. "
            f"Fix: remove one of the dependencies of the cycle."
        )

    return errors


# ---------------------------------------------------------------------------
# Check 17: Product defaults ('product_defaults' / 'product_platform_defaults')
# ---------------------------------------------------------------------------

PRODUCT_DEFAULT_KEYS = ("product_defaults", "product_platform_defaults")

# Python types accepted for each JSON Schema type (bool is a subclass of int, so it's excluded explicitly)
_JSON_TYPE_CHECKS = {
    "string": lambda v: isinstance(v, str),
    "boolean": lambda v: isinstance(v, bool),
    "integer": lambda v: isinstance(v, int) and not isinstance(v, bool),
    "number": lambda v: isinstance(v, int | float) and not isinstance(v, bool),
    "array": lambda v: isinstance(v, list),
    "object": lambda v: isinstance(v, dict),
}


def iter_product_values(node):
    """
    Yield (product, platform, value) for every product default of a setting node. 'platform' is None for
    'product_defaults' entries, which apply to every platform. Malformed entries are skipped.
    """
    product_defaults = node.get("product_defaults")
    if isinstance(product_defaults, dict):
        for product, value in product_defaults.items():
            yield product, None, value
    product_platform_defaults = node.get("product_platform_defaults")
    if isinstance(product_platform_defaults, dict):
        for product, platforms in product_platform_defaults.items():
            if isinstance(platforms, dict):
                for platform, value in platforms.items():
                    yield product, platform, value


def check_product_defaults(path, schema, declared_products):
    """
    Check the 'product_defaults' and 'product_platform_defaults' keywords:
      - They can only be used on settings, and are mutually exclusive.
      - 'product_defaults' maps a product to a value, 'product_platform_defaults' maps a product to a
        mapping of platforms to values. Unlike 'platform_default', 'other' isn't required: a missing
        platform means the product doesn't change the setting on that platform.
      - Every product must be declared in the core schema's 'product_dependencies' (*declared_products*).
      - Every value must match the setting's 'type'.

    Returns a list of error strings.
    """
    errors = []
    for node_path, node in walk_nodes(schema):
        used = [key for key in PRODUCT_DEFAULT_KEYS if key in node]
        if not used:
            continue

        if node.get("node_type") != "setting":
            errors.append(
                f"{path}: [{node_path}] '{used[0]}' can only be used on settings. "
                f"Fix: move it to the settings of this section."
            )
            continue

        if len(used) > 1:
            errors.append(
                f"{path}: [{node_path}] 'product_defaults' and 'product_platform_defaults' are mutually exclusive. "
                f"Fix: use 'product_platform_defaults' with the same value for every platform."
            )

        if "product_defaults" in node and not isinstance(node["product_defaults"], dict):
            errors.append(
                f"{path}: [{node_path}] 'product_defaults' must be a mapping of products to values. "
                f"Fix: use 'product_defaults: {{product: value}}'."
            )
        if "product_platform_defaults" in node:
            platform_defaults = node["product_platform_defaults"]
            if not isinstance(platform_defaults, dict):
                errors.append(
                    f"{path}: [{node_path}] 'product_platform_defaults' must be a mapping of products to platforms. "
                    f"Fix: use 'product_platform_defaults: {{product: {{platform: value}}}}'."
                )
            else:
                for product, platforms in platform_defaults.items():
                    if not isinstance(platforms, dict):
                        errors.append(
                            f"{path}: [{node_path}] 'product_platform_defaults.{product}' must be a mapping of "
                            f"platforms to values. Fix: use '{product}: {{platform: value}}'."
                        )
                        continue
                    unknown = sorted(set(platforms) - VALID_PLATFORM_KEYS)
                    if unknown:
                        errors.append(
                            f"{path}: [{node_path}] 'product_platform_defaults.{product}' contains unknown platform "
                            f"key(s): {', '.join(repr(k) for k in unknown)}. "
                            f"Fix: use only {sorted(VALID_PLATFORM_KEYS)}."
                        )

        products = sorted({product for key in used if isinstance(node[key], dict) for product in node[key]})
        for product in products:
            if product not in declared_products:
                errors.append(
                    f"{path}: [{node_path}] product '{product}' is not declared. "
                    f"Fix: add '{product}: [...]' to 'product_dependencies' in the core schema."
                )

        type_check = _JSON_TYPE_CHECKS.get(node.get("type"))
        if type_check is None:
            continue
        for product, platform, value in iter_product_values(node):
            if not type_check(value):
                where = f"product '{product}'" + (f" on platform '{platform}'" if platform else "")
                errors.append(
                    f"{path}: [{node_path}] the value {value!r} for {where} doesn't match the setting type "
                    f"'{node['type']}'. Fix: use a value of type '{node['type']}'."
                )
    return errors


# ---------------------------------------------------------------------------
# Check 18: Product default conflicts within a SKU or a product's dependencies
# ---------------------------------------------------------------------------

PRODUCT_CONFLICT_OSES = ("linux", "windows", "darwin", "aix")


def _runtime_environments():
    """
    Yield (label, platform keys by priority) for every runtime environment of the Agent.

    The priority must stay in sync with getPlatformDefault in pkg/config/setup/config.go: the Agent uses the
    'fargate' value when running on ECS Fargate, then the 'kubernetes' value on Kubernetes, then the 'container'
    value when containerized, then the value of its OS, then 'other'. Fargate is independent from the others, and
    the Agent is containerized on Kubernetes.
    """
    for os_name in PRODUCT_CONFLICT_OSES:
        for runtime in ("host", "container", "kubernetes"):
            for fargate in (False, True):
                keys = ["fargate"] if fargate else []
                if runtime == "kubernetes":
                    keys.append("kubernetes")
                if runtime != "host":
                    keys.append("container")
                keys += [os_name, "other"]
                label = os_name + ("" if runtime == "host" else "/" + runtime) + ("/fargate" if fargate else "")
                yield label, keys


def _product_value(node, product, platform_keys):
    """Return (found, value) for the default of *product* on a setting, for the given platform priority."""
    product_defaults = node.get("product_defaults")
    if isinstance(product_defaults, dict) and product in product_defaults:
        return True, product_defaults[product]
    platforms = (node.get("product_platform_defaults") or {}).get(product)
    if isinstance(platforms, dict):
        for key in platform_keys:
            if key in platforms:
                return True, platforms[key]
    return False, None


def _canonical_value(value):
    """Typed representation of a value: the Agent treats 1, 1.0 and True as different values."""
    return json.dumps(value, sort_keys=True)


def resolve_product_closure(products, product_dependencies):
    """Return the set of *products* and all their transitive dependencies."""
    closure = set()
    pending = list(products)
    while pending:
        product = pending.pop()
        if product in closure:
            continue
        closure.add(product)
        pending.extend(product_dependencies.get(product, []))
    return closure


def check_product_conflicts(path, schema, sku_definitions, product_dependencies):
    """
    Check that no SKU, and no product with its dependencies, enables products that set different values for
    the same setting, in any runtime environment (OS, containerized, ECS Fargate).

    Products that are never enabled together (not bundled by a SKU nor dependent on each other) can conflict:
    the Agent refuses to start if a user enables them both.

    Returns a list of error strings.
    """
    groups = [
        (f"SKU '{sku}'", resolve_product_closure(products, product_dependencies))
        for sku, products in sorted(sku_definitions.items())
    ]
    groups += [
        (f"product '{product}'", resolve_product_closure([product], product_dependencies))
        for product in sorted(product_dependencies)
    ]

    errors = []
    for node_path, node in walk_nodes(schema):
        if node.get("node_type") != "setting" or not any(key in node for key in PRODUCT_DEFAULT_KEYS):
            continue
        for owner, closure in groups:
            # Group the environments sharing the same conflicting assignment to keep the error short
            conflicts = {}
            for label, platform_keys in _runtime_environments():
                values = {}
                for product in sorted(closure):
                    found, value = _product_value(node, product, platform_keys)
                    if found:
                        values[product] = value
                if len({_canonical_value(v) for v in values.values()}) > 1:
                    assignment = ", ".join(f"{product}={value!r}" for product, value in values.items())
                    conflicts.setdefault(assignment, []).append(label)
            for assignment, labels in conflicts.items():
                errors.append(
                    f"{path}: [{node_path}] {owner} enables products setting different values on "
                    f"{', '.join(labels)}: {assignment}. "
                    f"Fix: make those products agree on a single value, or stop enabling them together."
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
    """
    if not os.path.isfile(exceptions_file):
        return {"array_no_items": set()}
    with open(exceptions_file) as f:
        data = yaml.safe_load(f) or {}
    return {
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

    # Products are declared in the core schema only, but other schemas (system-probe) can set product
    # defaults: resolve the core schema first so every schema is checked against its product definitions.
    core_schema_path = os.path.join(schema_dir, CORE_SCHEMA_FILE)
    core_schema = resolve_schema(core_schema_path) if os.path.isfile(core_schema_path) else {}
    sku_definitions, product_dependencies = get_product_definitions(core_schema)

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
        all_errors.extend(
            check_product_definitions(schema_path, schema, os.path.basename(schema_path) == CORE_SCHEMA_FILE)
        )
        all_errors.extend(check_product_defaults(schema_path, schema, set(product_dependencies)))
        all_errors.extend(check_product_conflicts(schema_path, schema, sku_definitions, product_dependencies))

    if all_errors:
        print(f"\nFound {len(all_errors)} schema linting error(s):\n")
        for error in all_errors:
            print(f"  ERROR: {error}")
        print(f"\n{SLACK_HINT}")
        raise Exit(code=1)

    print(f"\nAll {len(schema_files)} schema file(s) passed linting.")
