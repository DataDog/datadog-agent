"""
Produce processed variants of an Agent schema YAML.

The schema files under ``pkg/config/schema/yaml`` are the source of truth and
carry rich metadata (documentation strings, env-var lists, visibility, tags,
...). Most of that metadata is only useful for build-time tooling (docs
generation, config template rendering, linting) and is not needed by every
consumer of the schema.

This module derives *outputs* from a schema. Each output is a named
transformation registered in ``OUTPUTS`` that maps a parsed schema document to
a new document. The outputs are:

- ``embedded`` -- the trimmed schema that ships compiled into the Go binary,
  where it is used solely for JSON-Schema validation, so any keyword the
  validator ignores is dropped to keep the embedded artifact small and runtime
  RSS low.
- ``json_schema`` -- a *pure* JSON Schema, stripped of every Agent-specific
  extension so it is 100% compatible with the official specification
  (https://json-schema.org/) and validates with any conforming library. It
  keeps every standard validation/annotation keyword (``type``, ``properties``,
  ``required``, ``enum``, ...) so it can still fully validate an Agent
  configuration; it only drops the keywords we added on top of the spec
  (``node_type``, ``platform_default``, ``visibility``, ``env_vars``, ``tags``,
  ...). It is published for external consumers (e.g. SchemaStore for IDE
  autocompletion).

Future outputs are added by registering another function in ``OUTPUTS``; the
choice of what each output keeps or drops lives here, not in the build files.

It never touches the YAML source of truth. The input is read through
``merge_schema.resolve_schema``, which inlines every relative ``$ref`` -- so
pointing it at a top schema (e.g. ``core_schema.yaml``) yields a single
self-contained document holding that schema and all its sub-sections, with no
separate merge step required. (It is also wired into the Bazel build; see
``pkg/config/schema/BUILD.bazel``.)

Each output also picks its serialization format (see ``OUTPUTS``): ``embedded``
stays YAML for the Go embedding pipeline, while ``json_schema`` is written as
JSON for external consumers.

CLI (so Bazel can run it as a ``py_binary``):
    python produce_byproduct.py <output> <top_schema.yaml> <output.{yaml,json}>
"""

import json
import sys

import yaml

# resolve_schema inlines every relative ``$ref`` so a top schema and its
# per-section sub-files collapse into a single self-contained document. Import
# works both as a package (``tasks.schema``) and as a flat script / Bazel
# py_binary, where the sibling module sits next to this file.
from tasks.schema.merge_schema import resolve_schema

# Keys whose *immediate children are user-defined names* (config field names,
# $defs entry names), not schema keywords. When stripping keyword keys we must
# recurse into their values but never drop one of those names just because it
# collides with a keyword being stripped (e.g. a config field literally named
# "description").
_NAME_KEYED_KEYS = frozenset({"properties", "patternProperties", "$defs", "definitions", "dependentSchemas"})

# Keys whose *values are instance data*, not schemas: a literal default value, a
# const, or sample/enum instances. Their nested keys are arbitrary config data
# (e.g. a ``default`` object whose keys are config field names) and must never be
# interpreted as schema keywords, so the whole value is copied verbatim instead
# of being recursed into.
_INSTANCE_DATA_KEYS = frozenset({"default", "const", "enum", "examples", "example"})

# Every keyword defined by JSON Schema draft 2020-12 (the dialect the Agent
# schema declares via ``$schema``). The ``json_schema`` output keeps ONLY these
# and drops everything else, so the result is guaranteed to contain no
# Agent-specific extension and to validate with any conforming library. Listing
# the full spec vocabulary (not just the keywords currently used) keeps the
# output correct if the schema starts using more standard keywords later.
_STANDARD_JSON_SCHEMA_KEYWORDS = frozenset(
    {
        # Core (§8)
        "$schema",
        "$vocabulary",
        "$id",
        "$anchor",
        "$dynamicAnchor",
        "$ref",
        "$dynamicRef",
        "$defs",
        "$comment",
        # Applicators (§10)
        "allOf",
        "anyOf",
        "oneOf",
        "not",
        "if",
        "then",
        "else",
        "dependentSchemas",
        "prefixItems",
        "items",
        "contains",
        "properties",
        "patternProperties",
        "additionalProperties",
        "propertyNames",
        "unevaluatedItems",
        "unevaluatedProperties",
        # Validation (§6)
        "type",
        "enum",
        "const",
        "multipleOf",
        "maximum",
        "exclusiveMaximum",
        "minimum",
        "exclusiveMinimum",
        "maxLength",
        "minLength",
        "pattern",
        "maxItems",
        "minItems",
        "uniqueItems",
        "maxContains",
        "minContains",
        "maxProperties",
        "minProperties",
        "required",
        "dependentRequired",
        # Meta-data (§9)
        "title",
        "description",
        "default",
        "deprecated",
        "readOnly",
        "writeOnly",
        "examples",
        # Format (§7) and Content (§8.4)
        "format",
        "contentEncoding",
        "contentMediaType",
        "contentSchema",
    }
)


def _filter_keys(node, should_drop):
    """Return a copy of ``node`` with every key for which ``should_drop(key)``
    is true removed.

    Schema-aware: keys under ``properties``/``$defs``/... are treated as names
    and are never dropped, only their subtrees are recursed into.
    """
    if isinstance(node, dict):
        result = {}
        for key, value in node.items():
            if should_drop(key):
                continue
            if key in _INSTANCE_DATA_KEYS:
                # Value is instance data, not a schema: copy it verbatim so its
                # keys are never mistaken for schema keywords and dropped.
                result[key] = value
            elif key in _NAME_KEYED_KEYS and isinstance(value, dict):
                # Children here are names, not keywords: keep every name, filter
                # only the schema subtree each name points to.
                result[key] = {name: _filter_keys(sub, should_drop) for name, sub in value.items()}
            else:
                result[key] = _filter_keys(value, should_drop)
        return result
    if isinstance(node, list):
        return [_filter_keys(item, should_drop) for item in node]
    return node


def _strip_keys(node, drop_keys):
    """Return a copy of ``node`` with every occurrence of ``drop_keys`` removed."""
    return _filter_keys(node, lambda key: key in drop_keys)


def _keep_keys(node, keep_keys):
    """Return a copy of ``node`` keeping only keys in ``keep_keys``."""
    return _filter_keys(node, lambda key: key not in keep_keys)


def embedded(document):
    """Schema variant embedded into the Go binary for JSON-Schema validation.

    Drops human-facing documentation strings (``description``/``title``) -- the
    single largest contributor to the embedded schema size. The Go loader
    compiles the schema for validation only, so these keywords are unused at
    runtime.
    """
    return _strip_keys(document, frozenset({"description", "example", "title", "comment", "tags"}))


def json_schema(document):
    """Pure JSON Schema variant -- 100% compatible with https://json-schema.org/.

    Keeps only standard JSON Schema draft 2020-12 keywords and drops every
    Agent-specific extension (``node_type``, ``platform_default``,
    ``visibility``, ``env_vars``, ``env_parser``, ``tags``, ``comment``, the
    OpenAPI-style singular ``example``, ...). All standard validation and
    annotation keywords are retained, so the result can still fully validate an
    Agent configuration with any conforming JSON Schema library.

    Note: keeping only an allowlist (rather than dropping a blocklist) means any
    extension keyword added to the schema in the future is excluded
    automatically, with no change needed here.
    """
    return _keep_keys(document, _STANDARD_JSON_SCHEMA_KEYWORDS)


# Registry of named schema outputs. Each entry maps a name to ``(transform,
# fmt)``: the transform fully decides what the output keeps or drops, and
# ``fmt`` ("yaml" or "json") selects how it is serialized. ``embedded`` stays
# YAML because it feeds the YAML -> zstd -> Go embedding pipeline; ``json_schema``
# is emitted as JSON so external consumers get a ready-to-use ``.json`` schema.
OUTPUTS = {
    "embedded": (embedded, "yaml"),
    "json_schema": (json_schema, "json"),
}


def _processor_for(output):
    """Return the ``(transform, fmt)`` pair registered for ``output``."""
    try:
        return OUTPUTS[output]
    except KeyError:
        raise ValueError(f"unknown output {output!r}; known outputs: {sorted(OUTPUTS)}") from None


def process_schema(document, output):
    """Return ``document`` transformed by the named ``output`` processor."""
    processor, _fmt = _processor_for(output)
    return processor(document)


def _str_presenter(dumper, data):
    if "\n" in data:
        return dumper.represent_scalar("tag:yaml.org,2002:str", data, style="|")
    return dumper.represent_scalar("tag:yaml.org,2002:str", data)


def _dump(document, fmt, f):
    """Serialize ``document`` to ``f`` in the given ``fmt`` ("yaml" or "json")."""
    if fmt == "json":
        json.dump(document, f, indent=2, ensure_ascii=False, sort_keys=False)
        f.write("\n")
    else:
        yaml.add_representer(str, _str_presenter)
        yaml.dump(document, f, sort_keys=False)


def produce_byproduct(output, in_path, out_path):
    """Read the schema at ``in_path``, derive the named ``output`` byproduct and
    write it to ``out_path`` in that output's serialization format.

    Inlines every ``$ref`` so the result is a single self-contained file holding
    the top schema and all its sub-sections. A no-op for schemas with no refs
    (e.g. an already-merged file, or system-probe).
    """
    _processor, fmt = _processor_for(output)
    document = resolve_schema(in_path)
    processed = process_schema(document, output)
    with open(out_path, "w", encoding="utf-8") as f:
        _dump(processed, fmt, f)


def _main(argv):
    if len(argv) != 4:
        print(f"usage: {argv[0]} <output> <input.yaml> <output.{{yaml,json}}>", file=sys.stderr)
        print(f"known outputs: {sorted(OUTPUTS)}", file=sys.stderr)
        return 2
    output, in_path, out_path = argv[1], argv[2], argv[3]
    produce_byproduct(output, in_path, out_path)
    return 0


if __name__ == "__main__":
    sys.exit(_main(sys.argv))
