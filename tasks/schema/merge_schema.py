"""
Resolve relative JSON-Schema `$ref` references in an Agent schema YAML.

The Agent core schema is split across a top file (``yaml/core_schema.yaml``)
and per-section sub-files (e.g. ``yaml/logs_config.yaml``). The top file
references each sub-file with ``$ref: "<name>.yaml"`` (relative path). This
module provides:

- ``resolve_schema(path)``: load a schema file and return a dict with every
  relative ``$ref`` replaced by the content of its target file. Used by
  ``tasks/schema/template.py``, ``tasks/schema/lint.py``, and the Bazel
  merge step.
- A CLI entry point so Bazel can invoke it as a ``py_binary``:
  ``python -m tasks.schema.merge_schema <input.yaml> <output.yaml>``.

Only relative file refs are supported; absolute URLs are rejected. Refs are
resolved against the directory of the file that contains them.
"""

import os
import sys

import yaml


def resolve_schema(path):
    """Load a YAML schema and inline every relative ``$ref``.

    Returns a dict equivalent to a single-file schema. Raises
    ``FileNotFoundError`` if a ``$ref`` target does not exist, and
    ``ValueError`` for absolute URL refs (not supported in this codebase).
    """
    path = os.path.abspath(path)
    with open(path) as f:
        document = yaml.safe_load(f)
    return _resolve_node(document, os.path.dirname(path))


def _resolve_node(node, base_dir):
    if isinstance(node, dict):
        if "$ref" in node and len(node) == 1:
            return _load_ref(node["$ref"], base_dir)
        return {key: _resolve_node(value, base_dir) for key, value in node.items()}
    if isinstance(node, list):
        return [_resolve_node(item, base_dir) for item in node]
    return node


# JSON-schema metadata keys that label the *containing* document and have no
# meaning once the document is inlined into a parent. They are stripped on
# inline so they do not leak into the merged form.
_INLINE_STRIPPED_KEYS = ("$schema", "$id")


def _load_ref(ref, base_dir):
    if "://" in ref:
        raise ValueError(f"absolute $ref not supported: {ref!r} (only relative file refs are allowed)")
    target = os.path.join(base_dir, ref)
    if not os.path.isfile(target):
        raise FileNotFoundError(f"$ref target not found: {target}")
    with open(target) as f:
        sub_document = yaml.safe_load(f)
    if isinstance(sub_document, dict):
        sub_document = {k: v for k, v in sub_document.items() if k not in _INLINE_STRIPPED_KEYS}
    return _resolve_node(sub_document, os.path.dirname(target))


def _str_presenter(dumper, data):
    if "\n" in data:
        return dumper.represent_scalar("tag:yaml.org,2002:str", data, style="|")
    return dumper.represent_scalar("tag:yaml.org,2002:str", data)


def _main(argv):
    if len(argv) != 3:
        print(f"usage: {argv[0]} <input.yaml> <output.yaml>", file=sys.stderr)
        return 2
    merged = resolve_schema(argv[1])
    yaml.add_representer(str, _str_presenter)
    with open(argv[2], "w") as f:
        yaml.dump(merged, f, sort_keys=False)
    return 0


if __name__ == "__main__":
    sys.exit(_main(sys.argv))
