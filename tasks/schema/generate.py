"""
Schema generation tasks
"""

import os
import re
import shutil
import sys
import tempfile

import yaml
from invoke import task
from invoke.context import Context
from invoke.exceptions import Exit

from tasks.flavor import FipsBackend
from tasks.libs.build.bazel import bazel
from tasks.schema.codegen_init_settings import run_codegen, run_constants_codegen, run_core_constant_codegen
from tasks.schema.merge_schema import resolve_schema
from tasks.schema.produce_byproduct import produce_byproduct

SCHEMA_DIR = os.path.join("pkg", "config", "schema", "yaml")
COMPRESS_DIR = os.path.join("pkg", "config", "schema")
SETUP_INIT_DIR = os.path.join("pkg", "config", "setup")
# In-repo copy of the generatedsettings module. go.work and every replace directive point here, and its generated
# *.go files are gitignored, so in-repo builds always use code matching the current schema.
GENERATED_SETTINGS_DIR = os.path.join(SETUP_INIT_DIR, "generatedsettings_local")
# Committed snapshot of the generatedsettings module: the one external consumers download at a released tag.
GENERATED_SETTINGS_SNAPSHOT_DIR = os.path.join(SETUP_INIT_DIR, "generatedsettings")
# Files codegen used to write straight into SETUP_INIT_DIR. They were gitignored, so they linger in existing
# checkouts, where they no longer compile.
_LEGACY_SETUP_GENERATED_FILES = ("all_settings.go", "generated.go", "system_probe_settings.go")
_GENERATED_FILE_MARKER = "// NOTE! This is a generated file, do not modify it."
CORE_SCHEMA_MAIN_FILE = os.path.join(SCHEMA_DIR, "core_schema.yaml")
SYSTEM_PROBE_SCHEMA_MAIN_FILE = os.path.join(SCHEMA_DIR, "system-probe_schema.yaml")

# Schema entry points published as pure JSON Schema, keyed by the name of the
# config file each one validates: the generated files are named after that
# config file (datadog.schema.json, system-probe.schema.json, see
# json_schema_file_name) so users can tell which file each one validates.
JSON_SCHEMA_ENTRY_POINTS = {
    "datadog": CORE_SCHEMA_MAIN_FILE,
    "system-probe": SYSTEM_PROBE_SCHEMA_MAIN_FILE,
}


_SCRIPTS_DIR = os.path.dirname(__file__)


def str_presenter(dumper, data):
    if "\n" in data:
        return dumper.represent_scalar('tag:yaml.org,2002:str', data, style='|')
    return dumper.represent_scalar('tag:yaml.org,2002:str', data)


yaml.add_representer(str, str_presenter)


@task
def compress(ctx, output_dir=COMPRESS_DIR, fips_backend=FipsBackend.none):
    """
    Compress the schema files for embedding into the Go binary.

    Uses bazel, except on AIX build hosts, which don't have bazel: there,
    transparently falls back to `_compress_no_bazel`.
    """
    if sys.platform == "aix":
        _compress_no_bazel(ctx, output_dir)
        return
    bazel(
        "run",
        "//pkg/config/schema:install_compressed",
        "--",
        f"--destdir={os.path.abspath(output_dir)}",
        fips_backend=fips_backend,
    )


# Must match the ZSTD_ARGS in pkg/config/schema/BUILD.bazel: --no-check to
# match DataDog/zstd Go library behavior (no XXH64 frame checksum), -5 to
# match DataDog/zstd's DefaultCompression.
_ZSTD_ARGS = "--no-check -5"


def _compress_no_bazel(ctx, output_dir=COMPRESS_DIR):
    """
    Compress the schema files without bazel.

    Reimplements the pipeline in pkg/config/schema/BUILD.bazel (inline $refs,
    strip build-time-only keys, zstd-compress) by calling the same helpers
    bazel wraps as py_binary tools, plus a system `zstd` binary. Used on
    build hosts that cannot run bazel (e.g. AIX).
    """
    compressed_dir = os.path.join(output_dir, "compressed")
    os.makedirs(compressed_dir, exist_ok=True)

    for name, top_schema in (
        ("core_schema", CORE_SCHEMA_MAIN_FILE),
        ("system-probe_schema", SYSTEM_PROBE_SCHEMA_MAIN_FILE),
    ):
        embedded_fd, embedded_path = tempfile.mkstemp(suffix=".yaml")
        os.close(embedded_fd)
        try:
            produce_byproduct("embedded", top_schema, embedded_path)
            out_path = os.path.join(compressed_dir, f"{name}.yaml.zstd")
            ctx.run(f"zstd --force {_ZSTD_ARGS} {embedded_path} -o {out_path}")
        finally:
            os.remove(embedded_path)


# Where the JSON schemas are published: the DataDog/schema repository, which
# holds one folder per Agent version (agent/v7.85.1/datadog.schema.json) and a
# copy of the latest release at the root of agent/ (agent/datadog.schema.json).
# The `$id` of each published file must be its own URL, so that a pinned version
# and the latest copy never claim the same identifier.
_JSON_SCHEMA_ID_PREFIX = "https://raw.githubusercontent.com/DataDog/schema/main/agent/"
_AGENT_VERSION_RE = re.compile(r"^v?(\d+\.\d+\.\d+)$")


def json_schema_file_name(config_name):
    """Return the file name of the JSON schema for ``config_name``, e.g. ``datadog.schema.json``.

    The same name is used for the generated file and in its published URL (its ``$id``),
    so the file can be copied as-is into the DataDog/schema repository.
    """
    return f"{config_name}.schema.json"


def json_schema_id(config_name, version=None):
    """Return the ``$id`` of the published JSON schema for ``config_name``.

    With an Agent ``version`` (``7.85.1`` or ``v7.85.1``), the URL of that version's
    file; without one, the URL of the latest copy. Only stable releases are
    published, so pre-release versions (``7.85.0-rc.1``) are rejected.
    """
    file_name = json_schema_file_name(config_name)
    if version is None:
        return f"{_JSON_SCHEMA_ID_PREFIX}{file_name}"
    match = _AGENT_VERSION_RE.match(version)
    if not match:
        raise ValueError(f"invalid Agent version {version!r}: expected a stable release such as '7.85.1'")
    return f"{_JSON_SCHEMA_ID_PREFIX}v{match.group(1)}/{file_name}"


@task
def produce_embedded(ctx, input_path, output_path):
    """
    Produce the "embedded" schema byproduct from a (merged) schema.

    Trims build-time-only data (documentation strings, ...) so the artifact that
    gets compressed and embedded into the Go binary stays small. Output is YAML.
    """
    produce_byproduct("embedded", input_path, output_path)


@task(help={"version": "Agent version the schemas are published for (e.g. 7.85.1). Omit for the latest copy."})
def produce_jsonschema(ctx, output_dir, version=None):
    """
    Produce the pure JSON Schema byproducts for every Agent config file.

    The schema entry points are known (core -> datadog.yaml, system-probe ->
    system-probe.yaml), so the task only needs an output directory: it writes
    one <config-file-name>.schema.json per entry point into it (datadog.schema.json,
    system-probe.schema.json), the name it is published under.

    Strips every Agent-specific extension so the result is 100% compatible with
    https://json-schema.org/ and validates with any conforming library. Output
    is JSON, for external consumers (e.g. SchemaStore).

    The `$id` of each file is the URL it is published at in the DataDog/schema
    repository: the folder of `version` when given, the latest copy otherwise.
    """
    try:
        schema_ids = {name: json_schema_id(name, version) for name in JSON_SCHEMA_ENTRY_POINTS}
    except ValueError as e:
        raise Exit(str(e), code=1) from e
    os.makedirs(output_dir, exist_ok=True)
    for config_name, top_schema in JSON_SCHEMA_ENTRY_POINTS.items():
        out_path = os.path.join(output_dir, json_schema_file_name(config_name))
        produce_byproduct("json_schema", top_schema, out_path, schema_id=schema_ids[config_name])
        print(f"wrote {out_path} ($id: {schema_ids[config_name]})")


def schema_codegen(ctx):
    """
    Code generator for config schema.

    Writes the generated settings into GENERATED_SETTINGS_DIR and the constants into SETUP_INIT_DIR/constants.
    """

    # Some test run tasks command with a 'unittest.mock.MagicMock' instead of a Context
    if not isinstance(ctx, Context):
        return

    core_schema = resolve_schema(CORE_SCHEMA_MAIN_FILE)
    system_probe_schema = resolve_schema(SYSTEM_PROBE_SCHEMA_MAIN_FILE)

    _remove_legacy_setup_files()
    _generate_settings(core_schema, system_probe_schema, GENERATED_SETTINGS_DIR)
    run_constants_codegen(core_schema, system_probe_schema, os.path.join(SETUP_INIT_DIR, "constants"))


def _remove_legacy_setup_files(setup_dir=SETUP_INIT_DIR):
    """Remove the settings files codegen used to write into pkg/config/setup, leaving any hand-written file alone."""
    for name in _LEGACY_SETUP_GENERATED_FILES:
        path = os.path.join(setup_dir, name)
        try:
            with open(path) as f:
                generated = _GENERATED_FILE_MARKER in f.read(1024)
        except FileNotFoundError:
            continue
        if generated:
            print(f"Removing {path}, now generated into {GENERATED_SETTINGS_DIR}")
            os.remove(path)


def _generate_settings(core_schema, system_probe_schema, output_dir):
    run_codegen(core_schema, output_dir)
    run_codegen(system_probe_schema, output_dir, sysprobe=True)
    run_core_constant_codegen(core_schema, output_dir)


@task
def codegen(ctx):
    """
    Generate the Go files that register the settings from the schema.

    The settings are written into pkg/config/setup/generatedsettings_local, the copy in-repo builds use.
    """
    # Some test panic if a @task is called from a 'unittest.mock.MagicMock' which is done often.
    # Codegen call schema_codegen where we check for MagicMock
    return schema_codegen(ctx)


@task
def codegen_snapshot(_):
    """
    Refresh the committed pkg/config/setup/generatedsettings snapshot from the current schema.

    External consumers of pkg/config/setup build against this snapshot, at the version their go.mod requires.
    In-repo builds never use it. Release automation runs this task after each Agent release; do not run it and
    commit the result in regular PRs. doc.go, go.mod and go.sum are copied from the in-repo copy,
    whose go.mod and go.sum `dda inv tidy` keeps up to date.
    """
    core_schema = resolve_schema(CORE_SCHEMA_MAIN_FILE)
    system_probe_schema = resolve_schema(SYSTEM_PROBE_SCHEMA_MAIN_FILE)

    _generate_settings(core_schema, system_probe_schema, GENERATED_SETTINGS_SNAPSHOT_DIR)
    for name in ("doc.go", "go.mod", "go.sum"):
        shutil.copyfile(os.path.join(GENERATED_SETTINGS_DIR, name), os.path.join(GENERATED_SETTINGS_SNAPSHOT_DIR, name))
