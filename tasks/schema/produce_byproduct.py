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
- ``json_schema`` -- the JSON Schema published for external consumers (e.g.
  SchemaStore for IDE autocompletion), valid for the official specification
  (https://json-schema.org/) and usable with any conforming library. It drops
  every keyword we added on top of the spec (``node_type``,
  ``platform_default``, ``visibility``, ``env_vars``, ``tags``, ...) and
  ``default``, and keeps every other standard keyword (``type``,
  ``properties``, ``required``, ``enum``, ...), so it can still fully validate
  an Agent configuration. Its only non-standard keyword is
  ``patternErrorMessage``, an editor keyword other tools ignore, on the
  definitions of the Agent formats.

Both outputs replace the Agent formats (``format: duration``, ``format: url``),
custom to the Agent, with the regexp they stand for, shared through ``$defs``:
see ``AGENT_FORMATS``.

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
from typing import NamedTuple

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
# schema declares via ``$schema``). The ``json_schema`` output keeps only these,
# except ``default`` (see ``_JSON_SCHEMA_KEYWORDS``), and drops everything else,
# so no Agent-specific extension can reach the published schema. The only
# non-standard keyword it contains is added afterwards, on purpose:
# ``patternErrorMessage``, see ``AGENT_FORMATS``. Listing the full spec
# vocabulary (not just the keywords currently used) keeps the output correct if
# the schema starts using more standard keywords later.
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


# ---------------------------------------------------------------------------
# Agent formats
# ---------------------------------------------------------------------------
#
# The Agent schema marks string settings with an Agent-specific syntax with
# `format: <name>` (`format: duration`, `format: url`). These are custom Agent
# formats, not JSON Schema formats:
#
#   - JSON Schema defines no `url` format, and its `duration` means an ISO 8601
#     duration (`PT15S`), not the Go durations (`15s`) the Agent reads.
#   - Validators treat `format` as an annotation by default, the Agent's own
#     included: a format alone validates nothing.
#
# So the schema only names the format, and every generated schema (the one
# embedded in the Agent and the JSON Schema published for external tools)
# replaces `format: <name>` with `$ref: '#/$defs/<name>'`, a definition holding
# the regexp: each regexp is written once, below, and once in each generated
# schema. The Agent, IDEs and any other validator check values the same way.
#
# To add a format:
#   1. Write its regexp below. Use the portable ECMA-262 subset recommended by
#      JSON Schema (no lookarounds, no non-capturing groups), so every
#      validator reads it the same way, and keep it linear-time: no nested
#      repetition. IDEs use backtracking regex engines, and a pattern that
#      backtracks catastrophically freezes them on invalid input
#      (tasks/unit_tests/schema_byproduct_tests.py checks every pattern).
#   2. Register it in AGENT_FORMATS, with the error message IDEs show for a
#      value that doesn't match: what's expected, and examples of valid values.
#   3. Use `format: <name>` on string settings in the YAML schema; schema.lint
#      rejects unregistered formats.
#   4. Document it in doc/reference/agent-schema/keywords.md.

# A Go duration, as read by the Agent: exactly the strings Go's
# ``time.ParseDuration`` accepts. An optional sign, then either a bare ``0`` or
# one or more ``<number><unit>`` pairs, where the number may be fractional
# (``1.5h``, ``.5s``, ``5.s``) and the unit is one of ``ns``, ``us``, ``µs``
# (U+00B5), ``μs`` (U+03BC), ``ms``, ``s``, ``m``, ``h``. Examples: ``15s``,
# ``1m0s``, ``2h30m21s``, ``500ms``, ``-5m``. Units Go rejects, such as ``min``
# or ``d``, are rejected too. Every repetition starts with a digit or a dot and
# ends with a unit letter, so there is a single way to split a value.
GO_DURATION_PATTERN = r"^[-+]?(0|(([0-9]+(\.[0-9]*)?|\.[0-9]+)(ns|us|µs|μs|ms|s|m|h))+)$"

# An Agent endpoint URL, or "" for "unset" (the Agent then uses `site`).
# Deliberately lax, since rejecting a working URL is worse than accepting a
# bad one: an optional http:// or https:// scheme, optional `user:password@`,
# a host name or a bracketed IPv6 address (`[::1]`), an optional port, then
# anything without spaces starting with a path, query or fragment (`/`, `?`,
# `#`). It rejects spaces, other schemes (`ftp://`) and an empty host. Each part
# ends on a character the next one can't start with, so matching stays fast.
URL_PATTERN = r"^((https?://)?([^\s/?#@]+@)?(\[[^\]\s/]+\]|[a-zA-Z0-9._-]+)(:[0-9]{1,5})?([/?#]\S*)?)?$"


class AgentFormat(NamedTuple):
    """An Agent format: the regexp of its values, and the message IDEs show for a value that doesn't match it."""

    pattern: str
    error_message: str


# The Agent formats, by name: `format: <name>` in the YAML schema becomes a
# `$ref` to `$defs/<name>`, holding the regexp, in every generated schema.
#
# The error message is published as `patternErrorMessage`, an editor keyword
# read by VS Code's JSON support and its YAML extension: they show it instead
# of the regexp when a value doesn't match. Other tools ignore it and show
# their own message. Only the published JSON Schema has it: the Agent's own
# validator doesn't read it.
AGENT_FORMATS = {
    "duration": AgentFormat(
        GO_DURATION_PATTERN,
        "Expected a duration: a number with a unit (ns, us, ms, s, m, h), such as 30s, 1m30s or 1.5h. "
        'Units like "min" or "d" aren\'t supported.',
    ),
    "url": AgentFormat(
        URL_PATTERN,
        "Expected a URL or a host, such as https://app.datadoghq.com, intake.datadoghq.com:443 or https://[::1]:8443.",
    ),
}


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


def _map_schemas(node, transform):
    """Return a copy of ``node`` where ``transform`` was applied to every schema
    object, bottom-up.

    Walks the same way as ``_filter_keys``: values under ``default``/``enum``/...
    are instance data and are copied verbatim, and maps under
    ``properties``/``$defs``/... are walked as names pointing to schemas, so
    ``transform`` only ever sees real schema objects.
    """
    if isinstance(node, dict):
        result = {}
        for key, value in node.items():
            if key in _INSTANCE_DATA_KEYS:
                result[key] = value
            elif key in _NAME_KEYED_KEYS and isinstance(value, dict):
                result[key] = {name: _map_schemas(sub, transform) for name, sub in value.items()}
            else:
                result[key] = _map_schemas(value, transform)
        return transform(result)
    if isinstance(node, list):
        return [_map_schemas(item, transform) for item in node]
    return node


def _formats_to_refs(document, error_messages=False):
    """Replace every Agent format (``format: <name>``) with a ``$ref`` to a shared definition holding its regexp.

    The regexp of each format used is written once, in ``$defs/<name>`` at the
    root of ``document``, and each setting refers to it: ``$ref: '#/$defs/<name>'``.
    Copying it in every setting instead would repeat the duration regexp about
    a hundred times. Formats that aren't Agent formats are left as written
    (schema.lint rejects them in the YAML schema). See ``AGENT_FORMATS``.

    With ``error_messages``, each definition also gets the format's error
    message as ``patternErrorMessage``, shown by IDEs instead of the regexp.
    """
    used = set()

    def transform(schema):
        name = schema.get("format")
        if name not in AGENT_FORMATS:
            return schema
        used.add(name)
        ref = f"#/$defs/{name}"
        result = {k: v for k, v in schema.items() if k != "format"}
        if "$ref" in result:
            # A schema holds one ``$ref``: keep the existing one and require both.
            result["allOf"] = [*result.get("allOf", []), {"$ref": ref}]
        else:
            result["$ref"] = ref
        return result

    result = _map_schemas(document, transform)
    if not used:
        return result
    defs = dict(result.get("$defs", {}))
    for name in sorted(used):
        agent_format = AGENT_FORMATS[name]
        defs[name] = {"type": "string", "pattern": agent_format.pattern}
        if error_messages:
            defs[name]["patternErrorMessage"] = agent_format.error_message
    return {**result, "$defs": defs}


# Platforms of `platform_default`, in the order they're listed, with how they
# read in a sentence. `other` covers every platform not listed.
_PLATFORM_PHRASES = {
    "linux": ("on", "Linux"),
    "windows": ("on", "Windows"),
    "darwin": ("on", "macOS"),
    "aix": ("on", "AIX"),
    "container": ("in", "containers"),
    "fargate": ("on", "ECS Fargate"),
}


def _join(words):
    return words[0] if len(words) == 1 else f"{', '.join(words[:-1])} and {words[-1]}"


def _format_default(value):
    """Write a default in plain text: IDEs don't render a ``description`` as Markdown, so no backticks.

    Strings are quoted as they are (``"info"``, ``""``), without JSON escaping so Windows paths stay readable;
    other values use their JSON notation (``5000``, ``true``, ``[]``).
    """
    if isinstance(value, str):
        return f'"{value}"'
    return json.dumps(value, ensure_ascii=False)


def platform_defaults_sentence(platform_default):
    """Describe a ``platform_default`` mapping, e.g. "Default: 5002 on Windows and macOS, -1 elsewhere."

    Platforms sharing a value are grouped, in the order of ``_PLATFORM_PHRASES``, and ``other`` reads "elsewhere".
    """
    groups = {}  # rendered value -> platform keys, in insertion order
    for key in [*_PLATFORM_PHRASES, "other"]:
        if key in platform_default:
            groups.setdefault(_format_default(platform_default[key]), []).append(key)
    parts = []
    for value, keys in groups.items():
        where = []
        if "other" not in keys or len(keys) > 1:
            # group platform names by preposition: "on Linux and Windows and in containers"
            by_preposition = {}
            for key in keys:
                if key != "other":
                    preposition, name = _PLATFORM_PHRASES[key]
                    by_preposition.setdefault(preposition, []).append(name)
            where = [f"{preposition} {_join(names)}" for preposition, names in by_preposition.items()]
        if "other" in keys:
            where.append("elsewhere" if len(groups) > 1 else "on every platform")
        parts.append(f"{value} {' and '.join(where)}")
    return f"Default: {', '.join(parts)}."


def _describe_default(schema):
    """Add the default of a public setting to its ``description``, e.g. 'Default: "15s".'.

    The published schema declares no ``default`` (see ``json_schema``), so the
    description is where users see it: IDEs show it on hover. Built from
    ``default``, or from ``platform_default`` with each platform's value.
    Settings that aren't public are left as they are: their description, when
    they have one, isn't meant for users.
    """
    if schema.get("node_type") != "setting" or schema.get("visibility") != "public":
        return schema
    if isinstance(schema.get("platform_default"), dict):
        sentence = platform_defaults_sentence(schema["platform_default"])
    elif "default" in schema:
        sentence = f"Default: {_format_default(schema['default'])}."
    else:
        return schema
    description = schema.get("description")
    return {**schema, "description": f"{description.rstrip()}\n\n{sentence}" if description else sentence}


def _strip_keys(node, drop_keys):
    """Return a copy of ``node`` with every occurrence of ``drop_keys`` removed."""
    return _filter_keys(node, lambda key: key in drop_keys)


def _keep_keys(node, keep_keys):
    """Return a copy of ``node`` keeping only keys in ``keep_keys``."""
    return _filter_keys(node, lambda key: key not in keep_keys)


def embedded(document):
    """Schema variant embedded into the Go binary for JSON-Schema validation.

    Drops human-facing documentation strings (``description``/``title``/
    ``banner``) -- the single largest contributor to the embedded schema size.
    The Go loader compiles the schema for validation only, so these keywords
    are unused at runtime.

    The Agent formats (``format: duration``, ``format: url``) are replaced with
    a reference to their regexp so the Agent validates them, see
    ``_formats_to_refs``.
    """
    stripped = _strip_keys(document, frozenset({"description", "example", "title", "banner", "comment", "tags"}))
    return _formats_to_refs(stripped)


# The keywords kept in the published JSON Schema: every standard keyword except
# ``default`` (see ``json_schema``).
_JSON_SCHEMA_KEYWORDS = _STANDARD_JSON_SCHEMA_KEYWORDS - {"default"}


def json_schema(document):
    """JSON Schema variant published for external tools -- valid for https://json-schema.org/.

    Keeps only standard JSON Schema draft 2020-12 keywords, except ``default``,
    and drops every Agent-specific extension (``node_type``, ``platform_default``,
    ``visibility``, ``env_vars``, ``env_parser``, ``tags``, ``comment``, the
    OpenAPI-style singular ``example``, ...). All standard validation and
    annotation keywords are retained, so the result can still fully validate an
    Agent configuration with any conforming JSON Schema library.

    Note: keeping only an allowlist (rather than dropping a blocklist) means any
    extension keyword added to the schema in the future is excluded
    automatically, with no change needed here.

    The Agent formats (``format: duration``, ``format: url``) are replaced with
    a reference to their regexp, see ``_formats_to_refs``: they aren't JSON
    Schema formats. Their definitions also carry ``patternErrorMessage``, the
    only non-standard keyword of the output: an editor keyword other tools
    ignore, see ``AGENT_FORMATS``.

    ``default`` is dropped everywhere, as in most published schemas: IDEs
    insert every declared default when completing an object (the YAML
    extension fills in each child setting), and a default depending on the
    platform can't be expressed in a schema serving every platform. The default
    of each public setting is added to its ``description`` instead, see
    ``_describe_default``.
    """
    described = _map_schemas(document, _describe_default)
    return _formats_to_refs(_keep_keys(described, _JSON_SCHEMA_KEYWORDS), error_messages=True)


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


def _with_schema_id(document, schema_id):
    """Return a copy of ``document`` whose ``$id`` is ``schema_id``.

    Keeps the key order: ``$id`` replaces the existing one in place, or is added
    right after ``$schema`` (at the top otherwise), so the header stays first.
    """
    if "$id" in document:
        return {key: (schema_id if key == "$id" else value) for key, value in document.items()}
    result = {}
    if "$schema" in document:
        result["$schema"] = document["$schema"]
    result["$id"] = schema_id
    result.update((key, value) for key, value in document.items() if key != "$schema")
    return result


def produce_byproduct(output, in_path, out_path, schema_id=None):
    """Read the schema at ``in_path``, derive the named ``output`` byproduct and
    write it to ``out_path`` in that output's serialization format.

    Inlines every ``$ref`` so the result is a single self-contained file holding
    the top schema and all its sub-sections. A no-op for schemas with no refs
    (e.g. an already-merged file, or system-probe).

    ``schema_id``, when set, replaces the ``$id`` of the output: the source
    schema's ``$id`` is the location of its YAML file in the datadog-agent
    repository, which is wrong for a byproduct published elsewhere.
    """
    _processor, fmt = _processor_for(output)
    document = resolve_schema(in_path)
    processed = process_schema(document, output)
    if schema_id is not None:
        processed = _with_schema_id(processed, schema_id)
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
