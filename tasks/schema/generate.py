"""
Schema generation tasks
"""

import os
import sys
import tempfile

import yaml
from invoke import task
from invoke.context import Context

from tasks.libs.build.bazel import bazel
from tasks.schema.codegen_init_settings import run_codegen, run_constant_codegen
from tasks.schema.merge_schema import resolve_schema
from tasks.schema.produce_byproduct import produce_byproduct

SCHEMA_DIR = os.path.join("pkg", "config", "schema", "yaml")
COMPRESS_DIR = os.path.join("pkg", "config", "schema")
SETUP_INIT_DIR = os.path.join("pkg", "config", "setup")
CORE_SCHEMA_MAIN_FILE = os.path.join(SCHEMA_DIR, "core_schema.yaml")
SYSTEM_PROBE_SCHEMA_MAIN_FILE = os.path.join(SCHEMA_DIR, "system-probe_schema.yaml")


_SCRIPTS_DIR = os.path.dirname(__file__)

# Top-level sections of the core schema that get split into their own YAML
# file. Each entry in this list becomes a sibling file `yaml/<name>.yaml` in
# the same directory as yaml/core_schema.yaml, and the top file's entry is
# replaced with `{$ref: "<name>.yaml"}`. The list is the set of top-level
# entries with at least 8 children (chosen at the time of the split).
CORE_SPLIT_SECTIONS = [
    "logs_config",
    "apm_config",
    "sbom",
    "process_config",
    "cluster_agent",
    "admission_controller",
    "agent_telemetry",
    "container_image",
    "container_lifecycle",
    "external_metrics_provider",
    "private_action_runner",
    "orchestrator_explorer",
    "remote_configuration",
    "cluster_checks",
    "compliance_config",
    "snmp_listener",
    "internal_profiling",
    "multi_region_failover",
    "gpu",
    "otelcollector",
    "runtime_security_config",
]


def str_presenter(dumper, data):
    if "\n" in data:
        return dumper.represent_scalar('tag:yaml.org,2002:str', data, style='|')
    return dumper.represent_scalar('tag:yaml.org,2002:str', data)


yaml.add_representer(str, str_presenter)


@task
def compress(ctx, output_dir=COMPRESS_DIR):
    """
    Compress the schema files for embedding into the Go binary.

    Uses bazel, except on AIX build hosts, which don't have bazel: there,
    transparently falls back to `_compress_no_bazel`.
    """
    if sys.platform == "aix":
        _compress_no_bazel(ctx, output_dir)
        return
    bazel(ctx, "run", "//pkg/config/schema:install_compressed", "--", f"--destdir={os.path.abspath(output_dir)}")


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


_SUBSCHEMA_DIALECT = "https://json-schema.org/draft/2020-12/schema"
_SUBSCHEMA_ID_PREFIX = "https://raw.githubusercontent.com/DataDog/schema/main/agent/"


def _prepend_header(schema, schema_id, title=None, description=None):
    """Return a new dict with the JSON-schema header keys first.

    Adds ``$schema`` / ``$id`` (and optionally ``title`` / ``description``) to
    the *front* of the schema's key order. ``yaml.dump(sort_keys=False)``
    preserves insertion order, so this ensures the header is written at the
    top of the file rather than appended after the body.
    """
    header = {"$schema": _SUBSCHEMA_DIALECT, "$id": schema_id}
    if title is not None:
        header["title"] = title
    if description is not None:
        header["description"] = description
    return {**header, **{k: v for k, v in schema.items() if k not in header}}


@task
def produce_embedded(ctx, input_path, output_path):
    """
    Produce the "embedded" schema byproduct from a (merged) schema.

    Trims build-time-only data (documentation strings, ...) so the artifact that
    gets compressed and embedded into the Go binary stays small. Output is YAML.
    """
    produce_byproduct("embedded", input_path, output_path)


@task
def produce_jsonschema(ctx, input_path, output_path):
    """
    Produce the pure JSON Schema byproduct from a (merged) schema.

    Strips every Agent-specific extension so the result is 100% compatible with
    https://json-schema.org/ and validates with any conforming library. Output
    is JSON, for external consumers (e.g. SchemaStore).
    """
    produce_byproduct("json_schema", input_path, output_path)


def filter(expect, filename):
    def comparator(othername):
        actual = filename == othername
        return actual == expect

    return comparator


def schema_codegen(ctx):
    """
    Code generator for config schema.

    Writes the generated files straight into SETUP_INIT_DIR.
    """

    # Some test run tasks command with a 'unittest.mock.MagicMock' instead of a Context
    if not isinstance(ctx, Context):
        return

    core_schema = resolve_schema(CORE_SCHEMA_MAIN_FILE)
    system_probe_schema = resolve_schema(SYSTEM_PROBE_SCHEMA_MAIN_FILE)

    run_codegen(core_schema, filter(False, "system_probe_settings.go"), SETUP_INIT_DIR)
    run_codegen(system_probe_schema, filter(True, "system_probe_settings.go"), SETUP_INIT_DIR)
    run_constant_codegen(core_schema, system_probe_schema, SETUP_INIT_DIR)


@task
def codegen(ctx):
    """
    Generate the pkg/config/setup Go files that register the settings from the schema.

    The generated files are written directly into pkg/config/setup.
    """
    # Some test panic if a @task is called from a 'unittest.mock.MagicMock' which is done often.
    # Codegen call schema_codegen where we check for MagicMock
    return schema_codegen(ctx)
