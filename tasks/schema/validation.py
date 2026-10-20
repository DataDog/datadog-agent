"""
Add JSON-schema validation constraints to a setting in the agent configuration schema.

Run with::

    dda inv schema.add-validation <setting-name> <params>

*setting-name* is the dotted path of a leaf setting (e.g. ``process_config.max_per_message``).
*params* takes one of four forms:

1. A range expression, for ``integer``/``number`` settings only: one or two
   comparisons joined with ``and``, e.g. ``'>= 0'``, ``'> 2'``, ``'>= 10 and < 100'``.
   Translated into ``minimum``, ``exclusiveMinimum``, ``maximum`` and ``exclusiveMaximum``.
2. A comma-separated list of allowed values, e.g. ``'disabled, dry_run, enabled'``.
   Each value is made of ``[A-Za-z0-9_]``. Translated into ``enum``.
3. A JSON object, e.g. ``'{"minimum": 10, "exclusiveMaximum": 100}'``, whose fields
   are added to the setting verbatim.
4. A special expression. The only one is ``non-empty``, for ``string`` settings
   only, translated into ``minLength: 1``.

To make it easy to preserve order, new fields are inserted as text right after the
setting's last existing field. The setting is located with PyYAML ``compose()``
(which carries line numbers), following ``$ref`` split sections into their
sub-files, the same way ``schema.locate`` does.
"""

import json
import os
import re

import yaml
from invoke import task
from invoke.exceptions import Exit

from tasks.schema.locate import SCHEMAS, _mapping_entry, _mapping_get, _ref_target

NON_EMPTY = "non-empty"

_RANGE_CLAUSE = re.compile(r"^(>=|<=|>|<)\s*([-+]?(?:\d+\.?\d*|\.\d+)(?:[eE][-+]?\d+)?)$")
_RANGE_KEYWORDS = {
    ">=": "minimum",
    ">": "exclusiveMinimum",
    "<=": "maximum",
    "<": "exclusiveMaximum",
}
_ENUM_VALUE = re.compile(r"^[A-Za-z0-9_]+$")

# ---------------------------------------------------------------------------
# Params parsing
# ---------------------------------------------------------------------------


def _number(text, setting_type):
    """Convert *text* to the Python number matching *setting_type*.

    Bounds on a ``number`` setting are floats; bounds on an ``integer`` setting
    stay integers when they are integral (a fractional bound is still valid
    JSON schema, e.g. ``> 0.5``).
    """
    value = float(text)
    if setting_type == "integer" and value.is_integer():
        return int(value)
    return value


def _parse_range(params, setting_type):
    if setting_type not in ("integer", "number"):
        raise Exit(f"A range expression only applies to integer or number settings, not '{setting_type}'.", code=1)

    fields = {}
    lower = upper = None
    for clause in re.split(r"\s+and\s+", params.strip(), flags=re.IGNORECASE):
        match = _RANGE_CLAUSE.match(clause.strip())
        if match is None:
            raise Exit(
                f"Invalid range clause '{clause}'. Expected '<op> <number>' where <op> is one of >=, >, <=, <.",
                code=1,
            )
        op, raw = match.groups()
        value = _number(raw, setting_type)
        if op.startswith(">"):
            if lower is not None:
                raise Exit(f"Range expression '{params}' has more than one lower bound.", code=1)
            lower = (op, value)
        else:
            if upper is not None:
                raise Exit(f"Range expression '{params}' has more than one upper bound.", code=1)
            upper = (op, value)

    if lower and upper:
        (lop, lval), (uop, uval) = lower, upper
        if lval > uval or (lval == uval and (lop == ">" or uop == "<")):
            raise Exit(f"Range expression '{params}' does not allow any value.", code=1)

    # Lower bound first, so the output reads in the same order as the expression.
    for bound in (lower, upper):
        if bound is not None:
            op, value = bound
            fields[_RANGE_KEYWORDS[op]] = value
    return fields


def _parse_enum(params, setting_type):
    values = [v.strip() for v in params.split(",")]
    for value in values:
        if not _ENUM_VALUE.match(value):
            raise Exit(
                f"Invalid enum value '{value}' in '{params}'. Values may only contain letters, digits and '_'.",
                code=1,
            )
    if len(set(values)) != len(values):
        raise Exit(f"Enum '{params}' lists the same value more than once.", code=1)
    if setting_type in ("integer", "number"):
        try:
            return {"enum": [_number(v, setting_type) for v in values]}
        except ValueError:
            raise Exit(f"Enum values for a {setting_type} setting must be numbers: '{params}'.", code=1) from None
    return {"enum": values}


def _parse_json(params):
    try:
        fields = json.loads(params)
    except json.JSONDecodeError as e:
        raise Exit(f"Invalid JSON '{params}': {e}", code=1) from None
    if not isinstance(fields, dict) or not fields:
        raise Exit(f"JSON params must be a non-empty object, got '{params}'.", code=1)
    return fields


def parse_params(params, setting_type):
    """Return the dict of validation fields described by *params* for a setting of *setting_type*."""
    stripped = params.strip()
    if not stripped:
        raise Exit("Validation params must not be empty.", code=1)
    if stripped.startswith("{"):
        return _parse_json(stripped)
    if stripped == NON_EMPTY:
        if setting_type != "string":
            raise Exit(f"'{NON_EMPTY}' only applies to string settings, not '{setting_type}'.", code=1)
        return {"minLength": 1}
    if stripped[0] in "<>":
        return _parse_range(stripped, setting_type)
    return _parse_enum(stripped, setting_type)


# ---------------------------------------------------------------------------
# Locating the setting
# ---------------------------------------------------------------------------


def _compose(path):
    with open(path) as f:
        return yaml.compose(f)


def find_setting(parts, top_file):
    """Return ``(file_path, setting_node)`` for the dotted *parts* in the schema rooted at *top_file*.

    *setting_node* is the composed ``MappingNode`` of the setting, in the file
    that physically defines it. Returns None when the path does not exist.
    """
    file_path = top_file
    node = _compose(file_path)
    for part in parts:
        value = _mapping_get(_mapping_get(node, "properties"), part)
        if value is None:
            return None
        ref = _ref_target(value)
        if ref is not None:
            file_path = os.path.join(os.path.dirname(file_path), ref)
            value = _compose(file_path)
        node = value
    return file_path, node


def _scalar(node, key):
    value = _mapping_get(node, key)
    return value.value if isinstance(value, yaml.ScalarNode) else None


# ---------------------------------------------------------------------------
# Text insertion
# ---------------------------------------------------------------------------


def _value_end(node):
    """Return the end mark of the last scalar (or flow collection) under *node*.

    A block collection's own end mark points at the start of whatever follows
    it, possibly the next key's line, so descend to its last leaf instead.
    """
    while isinstance(node, yaml.MappingNode | yaml.SequenceNode) and not node.flow_style and node.value:
        last = node.value[-1]
        node = last[1] if isinstance(node, yaml.MappingNode) else last
    return node.end_mark


def _insert_fields(file_path, setting_node, fields):
    """Insert *fields* as YAML text at the end of *setting_node* in *file_path*."""
    with open(file_path) as f:
        lines = f.read().splitlines(keepends=True)

    first_key, _ = setting_node.value[0]
    indent = first_key.start_mark.column
    end = _value_end(setting_node)
    # A value ending mid-line (plain/quoted scalar) means the next line is
    # free; one ending at column 0 (block scalar) already consumed its trailing
    # newline, so insert at that line.
    insert_at = end.line if end.column == 0 else end.line + 1
    # Don't swallow trailing blank lines into the setting block.
    while insert_at > 0 and not lines[insert_at - 1].strip():
        insert_at -= 1
    if insert_at > 0 and not lines[insert_at - 1].endswith("\n"):
        lines[insert_at - 1] += "\n"

    dumped = yaml.safe_dump(fields, default_flow_style=False, sort_keys=False, allow_unicode=True)
    new_lines = [f"{" " * indent}{line}\n" for line in dumped.splitlines()]
    lines[insert_at:insert_at] = new_lines

    with open(file_path, "w") as f:
        f.writelines(lines)
    return insert_at + 1


def add_validation_to_schema(setting, params, schemas=SCHEMAS):
    """Add the validation fields described by *params* to *setting*.

    Returns ``(file_path, line, fields)``: where the fields were inserted and
    what they are.
    """
    parts = setting.split(".")
    if not all(parts):
        raise Exit(f"Invalid setting name '{setting}'.", code=1)

    matches = []
    for label, top_file in schemas:
        found = find_setting(parts, top_file)
        if found is not None:
            matches.append((label, *found))
    if not matches:
        raise Exit(f"Setting '{setting}' not found in the schema. Use `dda inv schema.locate` to search.", code=1)
    if len(matches) > 1:
        labels = ", ".join(label for label, _, _ in matches)
        raise Exit(f"Setting '{setting}' exists in several schemas ({labels}); pass --schema to pick one.", code=1)
    _, file_path, node = matches[0]

    if not isinstance(node, yaml.MappingNode) or _scalar(node, "node_type") != "setting":
        raise Exit(f"'{setting}' is not a leaf setting.", code=1)

    fields = parse_params(params, _scalar(node, "type"))

    existing = [key for key in fields if _mapping_entry(node, key) is not None]
    if existing:
        raise Exit(
            f"Setting '{setting}' already defines {', '.join(existing)} in {file_path}. Edit it by hand instead.",
            code=1,
        )

    line = _insert_fields(file_path, node, fields)
    return file_path, line, fields


# ---------------------------------------------------------------------------
# Task entry point
# ---------------------------------------------------------------------------


@task(
    help={
        "setting": "Dotted path of the setting to constrain (e.g. 'process_config.max_per_message').",
        "params": (
            "The constraint: a range ('>= 10 and < 100'), a list of allowed values ('one, two, three'), "
            "a JSON object of schema fields ('{\"minimum\": 10}'), or 'non-empty'."
        ),
        "schema": "Restrict the lookup to a single schema: 'core' or 'system-probe' (default: both).",
    },
    positional=["setting", "params"],
)
def add_validation(_ctx, setting, params, schema=None):
    """
    Add validation constraints to a setting in the agent configuration schema.
    """
    schemas = SCHEMAS
    if schema is not None:
        schemas = [(label, path) for label, path in SCHEMAS if label == schema]
        if not schemas:
            valid = ", ".join(label for label, _ in SCHEMAS)
            raise Exit(f"Invalid --schema value '{schema}'. Must be one of: {valid}.", code=1)

    file_path, line, fields = add_validation_to_schema(setting, params, schemas)
    print(f"Added validation to '{setting}' at {file_path}:{line}")
    for key, value in fields.items():
        print(f"  {key}: {json.dumps(value)}")
