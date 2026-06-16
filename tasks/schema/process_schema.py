"""
Produce processed variants of an Agent schema YAML.

The schema files under ``pkg/config/schema/yaml`` are the source of truth and
carry rich metadata (documentation strings, env-var lists, visibility, tags,
...). Most of that metadata is only useful for build-time tooling (docs
generation, config template rendering, linting) and is not needed by every
consumer of the schema.

This module derives *outputs* from a schema. Each output is a named
transformation registered in ``OUTPUTS`` that maps a parsed schema document to
a new document. Today the only output is ``embedded`` -- the trimmed schema
that ships compiled into the Go binary, where it is used solely for JSON-Schema
validation, so any keyword the validator ignores is dropped to keep the
embedded artifact small and runtime RSS low. Future outputs (e.g. a variant
tailored for an external docs site) are added by registering another function
in ``OUTPUTS``; the choice of what each output keeps or drops lives here, not in
the build files.

It is wired into the Bazel build between the merge and the zstd-compress steps
(see ``pkg/config/schema/BUILD.bazel``); it never touches the YAML source of
truth.

CLI (so Bazel can run it as a ``py_binary``):
    python process_schema.py <output> <input.yaml> <output.yaml>
"""

import sys

import yaml

# Keys whose *immediate children are user-defined names* (config field names,
# $defs entry names), not schema keywords. When stripping keyword keys we must
# recurse into their values but never drop one of those names just because it
# collides with a keyword being stripped (e.g. a config field literally named
# "description").
_NAME_KEYED_KEYS = frozenset({"properties", "patternProperties", "$defs", "definitions", "dependentSchemas"})


def _strip_keys(node, drop_keys):
    """Return a copy of ``node`` with every occurrence of ``drop_keys`` removed.

    Schema-aware: keys under ``properties``/``$defs``/... are treated as names
    and are never dropped, only their subtrees are recursed into.
    """
    if isinstance(node, dict):
        result = {}
        for key, value in node.items():
            if key in drop_keys:
                continue
            if key in _NAME_KEYED_KEYS and isinstance(value, dict):
                # Children here are names, not keywords: keep every name, strip
                # only the schema subtree each name points to.
                result[key] = {name: _strip_keys(sub, drop_keys) for name, sub in value.items()}
            else:
                result[key] = _strip_keys(value, drop_keys)
        return result
    if isinstance(node, list):
        return [_strip_keys(item, drop_keys) for item in node]
    return node


def embedded(document):
    """Schema variant embedded into the Go binary for JSON-Schema validation.

    Drops human-facing documentation strings (``description``/``title``) -- the
    single largest contributor to the embedded schema size. The Go loader
    compiles the schema for validation only, so these keywords are unused at
    runtime.
    """
    return _strip_keys(document, frozenset({"description", "title", "comment", "tags"}))


# Registry of named schema outputs. Add an entry to expose a new derived form;
# the function fully decides what that output keeps or drops.
OUTPUTS = {
    "embedded": embedded,
}


def process_schema(document, output):
    """Return ``document`` transformed by the named ``output`` processor."""
    try:
        processor = OUTPUTS[output]
    except KeyError:
        raise ValueError(f"unknown output {output!r}; known outputs: {sorted(OUTPUTS)}") from None
    return processor(document)


def _str_presenter(dumper, data):
    if "\n" in data:
        return dumper.represent_scalar("tag:yaml.org,2002:str", data, style="|")
    return dumper.represent_scalar("tag:yaml.org,2002:str", data)


def _main(argv):
    if len(argv) != 4:
        print(f"usage: {argv[0]} <output> <input.yaml> <output.yaml>", file=sys.stderr)
        print(f"known outputs: {sorted(OUTPUTS)}", file=sys.stderr)
        return 2
    output, in_path, out_path = argv[1], argv[2], argv[3]
    with open(in_path) as f:
        document = yaml.safe_load(f)
    processed = process_schema(document, output)
    yaml.add_representer(str, _str_presenter)
    with open(out_path, "w") as f:
        yaml.dump(processed, f, sort_keys=False)
    return 0


if __name__ == "__main__":
    sys.exit(_main(sys.argv))
