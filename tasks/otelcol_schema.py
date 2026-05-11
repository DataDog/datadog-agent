"""Invoke tasks for the consolidated DDOT Collector configuration JSON Schema.

Wraps `tasks.libs.otelcol_schema.bundle` (M3+M4) under `dda inv`. The
underlying tooling can also be invoked directly with
`python -m tasks.libs.otelcol_schema.{inventory,convert,bundle}`; these
tasks are the ergonomic entry points for day-to-day use.

Note: `jsonschema` is an optional dependency. The bundler runs without
it, but the meta-schema self-validation step is silently skipped. Use
`--validate` on `gen` to make the absence an error instead. See the
plan doc (docs/dev/plans/2026-05-07-otelcol-config-schema-plan.md) for
the upstream-dda followup that lifts `jsonschema` into the
`legacy-tasks` group.
"""

from __future__ import annotations

import difflib
import json
import sys
from pathlib import Path

from invoke.exceptions import Exit
from invoke.tasks import task

# Bundle artifact lives at the top of the otelcol component dir so it's
# discoverable next to the manifest it's derived from.
DEFAULT_BUNDLE_PATH = "comp/otelcol/collector-config.schema.json"
DEFAULT_INVENTORY_JSON = "otelcol-schema-inventory.json"
DEFAULT_INVENTORY_MD = "otelcol-schema-inventory.md"


def _serialise_bundle(bundle: dict) -> str:
    """Canonical bundle serialisation for `gen` and `check` to share. The
    `check` task relies on this being byte-stable across runs."""
    return json.dumps(bundle, indent=2) + "\n"


def _ensure_manifest_modules_downloaded() -> None:
    """Pre-flight: download any manifest-pinned Go modules missing from the
    local cache. Raises Exit(2) with a clear remediation message if any
    download fails (e.g. offline, network glitch, private module).

    Called by `gen` and `check` so a fresh-clone contributor doesn't end up
    silently with a permissive-fallback bundle just because the Go module
    cache hasn't been warmed yet. Skipped when the caller passes
    `--no-download` (which is appropriate for known-offline runs)."""
    from tasks.libs.otelcol_schema.bundle import ensure_manifest_modules_downloaded

    failures = ensure_manifest_modules_downloaded()
    if not failures:
        return
    lines = ["could not download all manifest-pinned modules:"]
    for spec, err in failures:
        lines.append(f"  - {spec}: {err}")
    lines.append("Pass --no-download to skip the pre-flight if running offline.")
    raise Exit(message="\n".join(lines), code=2)


@task(
    help={
        "output": "Path to write the bundle JSON to.",
        "missing": "How to handle unresolved $ref targets: 'permissive' (default) inserts placeholders, 'strict' errors out.",
        "report": "Optional path to write a markdown build report to.",
        "validate": "Treat missing `jsonschema` as an error rather than silently skipping meta-schema validation.",
        "no_download": "Skip the pre-flight `go mod download` for modules missing from the cache.",
    }
)
def gen(_ctx, output=DEFAULT_BUNDLE_PATH, missing="permissive", report=None, validate=False, no_download=False):
    """Generate the consolidated DDOT Collector configuration JSON Schema bundle."""
    from tasks.libs.otelcol_schema.bundle import build_bundle, write_report
    from tasks.libs.otelcol_schema.convert import jsonschema_available, validate_meta

    if validate and not jsonschema_available():
        raise Exit(
            message=(
                "jsonschema is not available in this environment, but --validate was passed.\n"
                "Install it (`pip install jsonschema`) or drop --validate to skip meta-schema "
                "self-validation."
            ),
            code=2,
        )

    if not no_download:
        _ensure_manifest_modules_downloaded()

    try:
        result = build_bundle(missing_strategy=missing)
    except RuntimeError as e:
        raise Exit(message=str(e), code=2) from e

    out_path = Path(output)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(_serialise_bundle(result.bundle))

    errors = validate_meta(result.bundle)
    if errors:
        for err in errors:
            print(f"meta-schema error: {err}", file=sys.stderr)
        raise Exit(code=1)

    if report:
        write_report(result, Path(report))

    print(
        f"wrote {out_path} "
        f"(sources={len(result.sources)}, "
        f"$defs={len(result.bundle.get('$defs') or {})}, "
        f"unresolved={len(result.unresolved_refs)}, "
        f"missing_components={len(result.missing_components)})"
    )


@task(
    help={
        "against": "Path to the checked-in bundle to diff against.",
        "missing": "How to handle unresolved $ref targets (must match how the artifact was generated).",
        "no_download": "Skip the pre-flight `go mod download` for modules missing from the cache.",
    }
)
def check(_ctx, against=DEFAULT_BUNDLE_PATH, missing="permissive", no_download=False):
    """Regenerate the bundle and diff against the checked-in copy.

    Exits non-zero if the freshly-generated bundle differs from the file
    on disk. Suitable as a CI gate to prevent the schema from drifting
    away from its upstream sources.
    """
    from tasks.libs.otelcol_schema.bundle import build_bundle

    expected_path = Path(against)
    if not expected_path.is_file():
        raise Exit(
            message=f"checked-in bundle not found at {expected_path}; run `dda inv otelcol-schema.gen` first", code=2
        )

    if not no_download:
        _ensure_manifest_modules_downloaded()

    try:
        result = build_bundle(missing_strategy=missing)
    except RuntimeError as e:
        raise Exit(message=str(e), code=2) from e

    fresh = _serialise_bundle(result.bundle)
    expected = expected_path.read_text()

    if fresh == expected:
        print(f"OK: {expected_path} is up to date.")
        return

    diff = difflib.unified_diff(
        expected.splitlines(keepends=True),
        fresh.splitlines(keepends=True),
        fromfile=str(expected_path),
        tofile=f"{expected_path} (regenerated)",
        n=3,
    )
    sys.stderr.write("".join(diff))
    raise Exit(
        message=f"\n{expected_path} is out of date — run `dda inv otelcol-schema.gen` to update.",
        code=1,
    )


@task(
    help={
        "json_out": "Path for the JSON inventory report (default: %s)" % DEFAULT_INVENTORY_JSON,
        "md": "Path for the markdown summary (default: %s)" % DEFAULT_INVENTORY_MD,
        "no_download": "Skip running `go mod download` for missing modules.",
    }
)
def inventory(_ctx, json_out=DEFAULT_INVENTORY_JSON, md=DEFAULT_INVENTORY_MD, no_download=False):
    """Audit which Collector components in the manifest ship a config.schema.yaml.

    Produces JSON + markdown reports describing schema-coverage gaps and
    `$ref` targets that don't resolve. Useful as a pre-flight before
    running `gen`, and as a way to track upstream schemagen rollout.
    """
    from tasks.libs.otelcol_schema.inventory import main as inventory_main

    argv = ["--json", json_out, "--md", md]
    if no_download:
        argv.append("--no-go-mod-download")
    rc = inventory_main(argv)
    if rc != 0:
        raise Exit(code=rc)
