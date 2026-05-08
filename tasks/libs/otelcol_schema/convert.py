"""M2 single-component converter for the DDOT Collector schema bundle.

Converts one schemagen-produced `config.schema.yaml` document into a
JSON-Schema-2020-12-conformant fragment. The only structural fix
required is rewriting bare same-file `$ref`s (e.g. `$ref: foo` produced
by schemagen) to JSON Pointers (`$ref: "#/$defs/foo"`). External refs
(URI, full-package-path, relative, namespace-relative) pass through
verbatim; the bundling step (M3) is responsible for canonicalising them
once the bundle's `$defs` registry is known.

Run as:

    python -m tasks.libs.otelcol_schema.convert <input.yaml> [--out <path>]
    python -m tasks.libs.otelcol_schema.convert --all

The `--all` mode runs the converter on every locally-authored
`config.schema.yaml` under `comp/otelcol/`, validates each against the
JSON Schema meta-schema, and prints a summary. Useful as a smoke test
while iterating on M2/M3.
"""

from __future__ import annotations

import argparse
import copy
import json
import sys
from pathlib import Path
from typing import Any

import yaml
from jsonschema import Draft202012Validator
from jsonschema.exceptions import SchemaError

from tasks.libs.otelcol_schema._refs import classify_ref
from tasks.libs.otelcol_schema.inventory import LOCAL_SCHEMAS, REPO_ROOT

JSON_SCHEMA_DRAFT = "https://json-schema.org/draft/2020-12/schema"


# ---------------------------------------------------------------------------
# Ref classification
# ---------------------------------------------------------------------------


def is_bare_ref(ref: str) -> bool:
    """A schemagen `bare` ref is a same-file `$defs` lookup. Already-
    rewritten `#/...` JSON Pointers are not bare."""
    if not ref or ref.startswith("#"):
        return False
    return classify_ref(ref).kind == "bare"


# ---------------------------------------------------------------------------
# Conversion
# ---------------------------------------------------------------------------


def convert_component(doc: dict[str, Any]) -> dict[str, Any]:
    """Return a JSON Schema 2020-12 fragment derived from a parsed
    `config.schema.yaml` document.

    Currently performs:
    - rewrite bare `$ref`s to JSON Pointers
    - prepend `$schema` declaring Draft 2020-12

    Does NOT mutate the input.
    """
    out = copy.deepcopy(doc)

    defs_names = set((out.get("$defs") or {}).keys())
    _rewrite_refs_inplace(out, defs_names)

    # Prepend $schema. dict insertion order is preserved on Python 3.7+; we
    # rebuild so it lands first for human readability of the JSON output.
    return {"$schema": JSON_SCHEMA_DRAFT, **out}


def _rewrite_refs_inplace(node: Any, defs_names: set[str]) -> None:
    """Walk the tree, rewriting bare `$ref` strings to `#/$defs/<name>`.
    External refs (anything not bare) pass through unchanged."""
    if isinstance(node, dict):
        for key, value in node.items():
            if key == "$ref" and isinstance(value, str) and is_bare_ref(value):
                if value in defs_names:
                    node[key] = f"#/$defs/{value}"
                # else: leave as-is; an unrecognised bare ref is invalid
                # but we surface it as-is rather than guessing.
            else:
                _rewrite_refs_inplace(value, defs_names)
    elif isinstance(node, list):
        for item in node:
            _rewrite_refs_inplace(item, defs_names)


def validate_meta(doc: dict[str, Any]) -> list[str]:
    """Validate a converted fragment against the JSON Schema 2020-12
    meta-schema. Returns a list of error messages (empty if valid)."""
    try:
        Draft202012Validator.check_schema(doc)
    except SchemaError as e:
        return [str(e)]
    return []


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def _convert_file(input_path: Path, out_path: Path | None) -> tuple[bool, list[str]]:
    """Convert one YAML schema to JSON Schema. Returns (ok, errors)."""
    try:
        doc = yaml.safe_load(input_path.read_text()) or {}
    except yaml.YAMLError as e:
        return False, [f"YAML parse error in {input_path}: {e}"]

    converted = convert_component(doc)
    errors = validate_meta(converted)

    if out_path is not None:
        out_path.write_text(json.dumps(converted, indent=2) + "\n")

    return not errors, errors


def _smoke_all() -> int:
    """Run the converter on every locally-authored config.schema.yaml.
    Print one line per component and a final summary."""
    total = 0
    ok = 0
    for _section, directory in LOCAL_SCHEMAS:
        schema = directory / "config.schema.yaml"
        if not schema.is_file():
            continue
        total += 1
        rel = schema.relative_to(REPO_ROOT)
        success, errors = _convert_file(schema, None)
        if success:
            ok += 1
            print(f"  OK   {rel}")
        else:
            for err in errors:
                print(f"  FAIL {rel}: {err}")
    print(f"\n{ok}/{total} local schemas converted and validated.")
    return 0 if ok == total else 1


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0] if __doc__ else "")
    parser.add_argument("input", nargs="?", type=Path, help="path to a config.schema.yaml")
    parser.add_argument("--out", type=Path, help="write JSON output to this path (default: stdout)")
    parser.add_argument(
        "--all",
        action="store_true",
        help="run on every locally-authored config.schema.yaml under comp/otelcol/",
    )
    args = parser.parse_args(argv)

    if args.all:
        return _smoke_all()

    if args.input is None:
        parser.error("expected an input file path or --all")

    success, errors = _convert_file(args.input, args.out)
    for err in errors:
        print(err, file=sys.stderr)

    if args.out is None and success:
        # Print the converted JSON to stdout when no output file is given.
        doc = yaml.safe_load(args.input.read_text()) or {}
        print(json.dumps(convert_component(doc), indent=2))

    return 0 if success else 1


if __name__ == "__main__":
    sys.exit(main())
