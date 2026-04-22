"""
Schema generation tasks
"""

import os

import yaml
from invoke import task
from invoke.exceptions import Exit

from tasks.schema.fixes import fix_schema
from tasks.schema.template_parser import parse_template

SCHEMA_DIR = os.path.join("pkg", "config", "schema")
CORE_TEMPLATE = os.path.join("pkg", "config", "config_template.yaml")
SYSPROBE_TEMPLATE = os.path.join("pkg", "config", "system-probe_template.yaml")

_SCRIPTS_DIR = os.path.dirname(__file__)


def str_presenter(dumper, data):
    if "\n" in data:
        return dumper.represent_scalar('tag:yaml.org,2002:str', data, style='|')
    return dumper.represent_scalar('tag:yaml.org,2002:str', data)


yaml.add_representer(str, str_presenter)


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

    core = os.path.join(output_dir, "core_schema.yaml")
    sysprobe = os.path.join(output_dir, "system-probe_schema.yaml")

    # Step 1: Generate base schema using the agent binary.
    # The createschema command writes output files to the current directory,
    # so we cd into the output dir and use an absolute path for the binary.
    print("Generating base schema files...")
    agent_bin_abs = os.path.abspath(agent_bin)
    with ctx.cd(output_dir):
        core_schema = ctx.run(
            f"{agent_bin_abs} createschema --target core", env={"DD_CREATE_SCHEMA": "true"}, hide=True
        ).stdout
        sysprobe_schema = ctx.run(
            f"{agent_bin_abs} createschema --target system-probe", env={"DD_CREATE_SCHEMA": "true"}, hide=True
        ).stdout

    core_schema = yaml.safe_load(core_schema)
    sysprobe_schema = yaml.safe_load(sysprobe_schema)

    print("Enriching schemas with documentation from config_template.yaml...")
    core_schema = parse_template(CORE_TEMPLATE, core_schema)
    sysprobe_schema = parse_template(SYSPROBE_TEMPLATE, sysprobe_schema)

    print("Applying OS-specific fixes...")
    core_schema, sysprobe_schema = fix_schema(core_schema, sysprobe_schema)

    # adding header
    core_schema["$schema"] = "https://json-schema.org/draft/2020-12/schema"
    sysprobe_schema["$schema"] = "https://json-schema.org/draft/2020-12/schema"

    core_schema["$id"] = "https://raw.githubusercontent.com/DataDog/schema/main/agent/datadog.yaml.schema.json"
    core_schema["title"] = "DataDog Agent configuration schema"
    core_schema["description"] = "The schema to validate the datadog.yaml configuration for the DataDog Agent"

    sysprobe_schema["$id"] = "https://raw.githubusercontent.com/DataDog/schema/main/agent/system-probe.yaml.schema.json"
    sysprobe_schema["title"] = "System Probe configuration schema"
    sysprobe_schema["description"] = "The schema to validate the system-probe.yaml configuration for the DataDog Agent"

    with open(core, "w") as f:
        yaml.dump(core_schema, f, sort_keys=False)
    with open(sysprobe, "w") as f:
        yaml.dump(sysprobe_schema, f, sort_keys=False)

    print("Schema generation complete. Output files:")
    print(f"  {core}")
    print(f"  {sysprobe}")
