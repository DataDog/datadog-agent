"""
Schema generation tasks
"""

import json
import os
import shutil
import tempfile

import yaml
from invoke import Failure, task
from invoke.exceptions import Exit

from tasks.libs.build.bazel import bazel
from tasks.libs.common.color import color_message
from tasks.schema.add_comments import add_comments
from tasks.schema.codegen_init_settings import run_codegen
from tasks.schema.fixes import fix_schema
from tasks.schema.merge_schema import resolve_schema
from tasks.schema.produce_byproduct import produce_byproduct
from tasks.schema.settings_source_analyzer import extract_imperative_code_hints
from tasks.schema.template_parser import parse_template

SCHEMA_DIR = os.path.join("pkg", "config", "schema", "yaml")
COMPRESS_DIR = os.path.join("pkg", "config", "schema")
SETUP_INIT_DIR = os.path.join("pkg", "config", "setup")
CORE_TEMPLATE = os.path.join("pkg", "config", "config_template.yaml")
SYSPROBE_TEMPLATE = os.path.join("pkg", "config", "system-probe_template.yaml")
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

SYSPROBE_SPLIT_SECTIONS = [
    ["runtime_security_config", "system-probe-cws"],
    ["service_monitoring_config", "system-probe-usm"],
]


def str_presenter(dumper, data):
    if "\n" in data:
        return dumper.represent_scalar('tag:yaml.org,2002:str', data, style='|')
    return dumper.represent_scalar('tag:yaml.org,2002:str', data)


yaml.add_representer(str, str_presenter)


@task
def compress(ctx, output_dir=COMPRESS_DIR):
    bazel(ctx, "run", "//pkg/config/schema:install_compressed", "--", f"--destdir={os.path.abspath(output_dir)}")


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


def split_and_write_schema(schema, output_dir, sections, name):
    """Optionally split named top-level sections out of *schema* into sibling
    YAML files, then write the (possibly-mutated) top schema to
    ``<output_dir>/<name>.yaml``.

    If *sections* is falsy (None or empty), no splitting happens — the schema
    is written as-is. Otherwise, for each section name in *sections* that
    exists at ``schema["properties"][<section_name>]``, the section's content
    is written to ``<output_dir>/<section_file>.yaml`` and the entry in the
    in-memory schema is replaced with ``{"$ref": "<section_file>.yaml"}``.
    Sections not present in the schema are silently skipped.

    The values of each element in ``sections`` can be either a string or list.
    If the item is a string it is used for the section name and file. If it is
    a list, then item[0] is the section name and the item[1] is the file name.

    Each sub-file is written with a JSON-schema header (``$schema``, ``$id``)
    so it is a self-contained, navigable schema document. The companion
    ``merge_schema.resolve_schema`` strips these fields when inlining so they
    do not pollute the merged form.
    """
    if sections:
        properties = schema.get("properties") or {}
        for section_row in sections:
            section_name, section_file = (section_row, section_row)
            if isinstance(section_row, list):
                section_name, section_file = (section_row[0], section_row[1])
            if section_name not in properties:
                continue
            sub_path = os.path.join(output_dir, f"{section_file}.yaml")
            body = _prepend_header(
                properties[section_name],
                schema_id=f"{_SUBSCHEMA_ID_PREFIX}{section_file}.yaml.schema.json",
            )
            with open(sub_path, "w") as f:
                yaml.dump(body, f, sort_keys=False)
            properties[section_name] = {"$ref": f"{section_file}.yaml"}

    top_path = os.path.join(output_dir, f"{name}.yaml")
    with open(top_path, "w") as f:
        yaml.dump(schema, f, sort_keys=False)


@task
def generate(ctx, agent_bin, output_dir=SCHEMA_DIR):
    """
    Generate the enriched schema files for the core agent and system-probe.

    Steps:
    1. Run the agent binary to generate the base schemas (core_schema.yaml, system-probe_schema.yaml)
    2. Enrich the schemas with documentation from config_template.yaml
    3. Apply OS-specific fixes to the enriched schemas
    """
    if not os.path.isfile(agent_bin):
        raise Exit(
            f"Agent binary not found at {agent_bin}. Build the agent first with: dda inv agent.build",
            code=1,
        )

    os.makedirs(output_dir, exist_ok=True)

    # Step 1: Generate base schema using the agent binary.
    # The createschema command writes output files to the current directory,
    # so we cd into the output dir and use an absolute path for the binary.
    print("Generating base schema files...")
    agent_bin_abs = os.path.abspath(agent_bin)
    with ctx.cd(output_dir):
        core_schema = ctx.run(
            f"{agent_bin_abs} createschema --target core", env={"DD_CREATE_SCHEMA": "true"}, hide="out"
        ).stdout
        sysprobe_schema = ctx.run(
            f"{agent_bin_abs} createschema --target system-probe", env={"DD_CREATE_SCHEMA": "true"}, hide="out"
        ).stdout

    core_schema = yaml.safe_load(core_schema)
    sysprobe_schema = yaml.safe_load(sysprobe_schema)

    print("Enriching schemas with documentation from config_template.yaml...")
    core_schema = parse_template(CORE_TEMPLATE, core_schema)
    sysprobe_schema = parse_template(SYSPROBE_TEMPLATE, sysprobe_schema)

    print("Applying OS-specific fixes...")
    core_schema, sysprobe_schema = fix_schema(core_schema, sysprobe_schema)

    comments_info = extract_comments(ctx)
    add_comments(core_schema, comments_info)

    # Add the JSON-schema header at the *top* of each file. Dict insertion
    # order is preserved by yaml.dump(sort_keys=False), so listing $schema /
    # $id / title / description before the schema body puts them first.
    core_schema = _prepend_header(
        core_schema,
        schema_id="https://raw.githubusercontent.com/DataDog/schema/main/agent/datadog.yaml.schema.json",
        title="DataDog Agent configuration schema",
        description="The schema to validate the datadog.yaml configuration for the DataDog Agent",
    )
    sysprobe_schema = _prepend_header(
        sysprobe_schema,
        schema_id="https://raw.githubusercontent.com/DataDog/schema/main/agent/system-probe.yaml.schema.json",
        title="System Probe configuration schema",
        description="The schema to validate the system-probe.yaml configuration for the DataDog Agent",
    )

    # Split large sections out of the core schema into sibling files; the top
    # file references each via `$ref`. The Go embed and Python consumers
    # transparently merge these back at load time. system-probe is written
    # as a single file (no splitting).
    split_and_write_schema(core_schema, output_dir, CORE_SPLIT_SECTIONS, "core_schema")
    split_and_write_schema(sysprobe_schema, output_dir, SYSPROBE_SPLIT_SECTIONS, "system-probe_schema")

    print("Schema generation complete.")


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


@task
def hints(ctx):
    # Extract hints, dump them to a temporary directory for debugging purposes
    hints = extract_imperative_code_hints()
    hints_tmp_file = tempfile.NamedTemporaryFile(mode='w', suffix='.json', delete=False, delete_on_close=False)
    hints_tmp_file.file.write(json.dumps(hints))
    print(f"hints file = {hints_tmp_file.name}")


def extract_comments(ctx):
    # Extract hints object
    hints = extract_imperative_code_hints()

    # Collect comments per setting
    comment_assoc_map = {}

    for setting_group in hints:
        for setting in setting_group['settings']:
            (setting_name, _unused, comment) = setting
            if comment == '':
                continue
            comment_assoc_map[setting_name] = comment

    return comment_assoc_map


def filter(expect, filename):
    def comparator(othername):
        actual = filename == othername
        return actual == expect

    return comparator


@task
def codegen(ctx, keep_orig_order=False, check=False, fix=False, keeptmp=False):
    """
    Code generator for config schema

    keep_orig_order: If true, extract order from *_settings.go files, keep it the same
    check:           If true, validate whether codegen matches SETUP_INIT_DIR
    fix:             If true, copy the codegen files into SETUP_INIT_DIR
    keeptmp:         If true, don't delete the temporary folder
    """

    core_schema = resolve_schema(CORE_SCHEMA_MAIN_FILE)
    system_probe_schema = resolve_schema(SYSTEM_PROBE_SCHEMA_MAIN_FILE)
    hints = extract_imperative_code_hints()

    tmpdir = tempfile.mkdtemp()
    run_codegen(core_schema, filter(False, "system_probe_settings.go"), hints, keep_orig_order, tmpdir)
    run_codegen(system_probe_schema, filter(True, "system_probe_settings.go"), hints, keep_orig_order, tmpdir)

    display = not check and not fix

    if display:
        print("Codegen complete. Output dir: %s" % tmpdir)

    if check:
        # Compare tmpdir against SETUP_INIT_DIR, fail if different
        try:
            for file in os.listdir(tmpdir):
                ctx.run(f"diff {os.path.join(tmpdir, file)} {SETUP_INIT_DIR}/")
        except Failure as e:
            print(
                color_message(
                    "Codegen for configuration differs, fix this by running `dda inv schema.codegen --fix`", "yellow"
                )
            )
            raise Exit(code=1) from e

    if fix:
        # Fix any differences by copying the codegen results into SETUP_INIT_DIR
        ctx.run(f"cp {tmpdir}/*_settings.go {SETUP_INIT_DIR}/")

    if not keeptmp and not display:
        shutil.rmtree(tmpdir)
