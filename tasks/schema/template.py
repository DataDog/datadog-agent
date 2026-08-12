"""
Config template generation tasks.
"""

import os
import sys
import textwrap

from invoke import task
from invoke.exceptions import Exit

from tasks.schema.merge_schema import resolve_schema

# Available Variables
#
# Most of those can be changed by customers and are resolved at runtime when the Agent starts. The following are the
# most likely values.
#
default_path = {
    "windows": {
        "${conf_path}": "c:/programdata/datadog",
        "${install_path}": "c:/program files/datadog/datadog agent",
        "${run_path}": "c:/programdata/datadog/run",
        "${log_path}": "c:/programdata/datadog/logs",
    },
    "linux": {
        "${conf_path}": "/etc/datadog-agent",
        "${install_path}": "/opt/datadog-agent",
        "${run_path}": "/opt/datadog-agent/run",
        "${log_path}": "/var/log/datadog",
    },
    "darwin": {
        "${conf_path}": "/opt/datadog-agent/etc",
        "${install_path}": "/opt/datadog-agent",
        "${run_path}": "/opt/datadog-agent/run",
        "${log_path}": "/opt/datadog-agent/logs",
    },
    "aix": {
        "${conf_path}": "/etc/datadog-agent",
        "${install_path}": "/opt/datadog-agent",
        "${run_path}": "/opt/datadog-agent/run",
        "${log_path}": "/var/log/datadog",
    },
}

# Maps env_parser schema values to their human-readable env var type descriptions.
# Used by _get_node_types_and_default to produce the @env line in config templates.
# The special value "json" is handled separately by _env_type_for_json().
_ENV_PARSER_ENV_TYPES = {
    "comma_separated": "comma-separated list of strings",
    "space_separated": "space-separated list of strings",
    "comma_and_space_separated": "comma and space separated list of strings",
    "comma_then_space_separated": "comma or space separated list of strings",
    "csv_comma_separated": "comma-separated list of strings",
    "traces_span": "comma-separated list of key-value pairs",
    "json_list_or_comma_separated": "comma-separated list of strings or JSON array of strings",
    "json_list_or_space_separated": "space-separated list of strings or JSON array of strings",
}


def _env_type_for_json(node):
    """Return the env var type description for a node whose env_parser is 'json'."""
    node_type = node.get("type")
    if node_type == "array":
        items_type = node.get("items", {}).get("type", "object")
        if items_type == "string":
            return "JSON array of strings"
        elif items_type == "number":
            return "JSON array of numbers"
        return "JSON list of objects"
    return "JSON object"


build_type_to_section = {
    "agent-py3": [
        "Common",
        "Agent",
        "CoreAgent",
        "Dogstatsd",
        "LogsAgent",
        "Logging",
        "DockerTagging",
        "KubernetesTagging",
        "ECS",
        "Containerd",
        "CRI",
        "TraceAgent",
        "Kubelet",
        "KubeApiServer",
        "PrivateActionRunner",
    ],
    "iot-agent": [
        "Common",
        "Agent",
        "Dogstatsd",
        "LogsAgent",
        "Logging",
    ],
    "system-probe": [
        "SystemProbe",
        "NetworkModule",
        "UniversalServiceMonitoringModule",
    ],
    "dogstatsd": [
        "Common",
        "Dogstatsd",
        "DockerTagging",
        "Logging",
        "KubernetesTagging",
        "ECS",
        "TraceAgent",
        "Kubelet",
    ],
    "dca": [
        "ClusterAgent",
        "Common",
        "Logging",
        "KubeApiServer",
        "ClusterChecks",
        "AdmissionController",
        "PrivateActionRunner",
    ],
    "dcacf": [
        "ClusterAgent",
        "Common",
        "Logging",
        "ClusterChecks",
        "CloudFoundry",
    ],
}

VALID_BUILD_TYPES = list(build_type_to_section.keys())
VALID_OS_TARGETS = list(default_path.keys())

# Canonical paths to the enriched schema files. Callers (tasks/agent.py,
# tasks/cluster_agent_helpers.py, tasks/dogstatsd.py) import these so the
# schema location is defined in exactly one place.
CORE_SCHEMA_FILE = "./pkg/config/schema/yaml/core_schema.yaml"
SYSPROBE_SCHEMA_FILE = "./pkg/config/schema/yaml/system-probe_schema.yaml"

# build_types that use the core schema vs the system-probe schema
_SYSPROBE_BUILD_TYPES = {"system-probe"}


def _is_node_section(node):
    return node.get("node_type", "") == "section"


def _should_render(build_type, node):
    for t in node.get("tags", []):
        if t.startswith("template_section:"):
            section = t.split(":")[1]
            return section in build_type_to_section[build_type]
    return True


def _filter_hidden_nodes(nodes, os_target):
    to_delete = []
    for name, node in nodes.items():
        if node.get("visibility", "") != "public":
            to_delete.append(name)
            continue

        for tag in node.get("tags", []):
            if tag.startswith("platform_only:"):
                platforms = tag.split(":")[1].split(",")
                if os_target not in platforms:
                    to_delete.append(name)

    for name in to_delete:
        del nodes[name]

    return nodes


def _get_platform_version(data, os_target):
    if isinstance(data, str):
        return data

    if os_target in data:
        return data[os_target]
    elif os_target == "container" and "linux" in data:
        return data["linux"]

    return data["other"]


def _get_default_from_node(node, os_target):
    if "default" in node:
        default = node["default"]
    elif "platform_default" in node:
        default = _get_platform_version(node["platform_default"], os_target)
    else:
        return None

    if not isinstance(default, str):
        return default

    if "${" in default:
        for var, repl in default_path[os_target].items():
            default = default.replace(var, repl)

        if os_target == "windows":
            default = default.replace("/", "\\\\")

    return default


def _get_node_types_and_default(full_name, node, os_target):
    if _is_node_section(node):
        return "custom object", None, None

    default = _get_default_from_node(node, os_target)

    node_type = node.get("type")
    for tag in node.get("tags", []):
        if tag.startswith("golang_type:"):
            node_type = tag.split(":")[1]

    if node_type == "array":
        # some type are just arrays of unknown types (ex: dogstatsd_mapper_profiles)
        if "items" not in node:
            yaml_type, env_type = "list", "JSON list"
        elif node["items"]["type"] == "string":
            yaml_type, env_type = "list of strings", "space-separated list of strings"
        elif node["items"]["type"] == "object":
            yaml_type, env_type = "list of object", "JSON list of object"
        elif node["items"]["type"] in ["number", "integer"]:
            yaml_type, env_type = "list of integers", "JSON array of numbers or space-separated list of integers"
        else:
            raise Exception(f"unknown array of type: {node['items']['type']}")
    elif node_type in ["number", "integer", "int", "int64"]:
        yaml_type, env_type = "integer", "integer"
    elif node_type == '[]int':
        yaml_type, env_type = "list of integers", "JSON array of numbers or space-separated list of integers"
    elif node_type == "float64":
        return "float", "float", default
    elif node_type == "object":
        if node.get("additionalProperties", {}).get("type") == "string":
            yaml_type, env_type = "object", "JSON object of string to string"
        else:
            yaml_type, env_type = "object", "JSON object"
    elif node_type == 'map[string]interface{}':
        yaml_type, env_type = "object", "JSON object"
    else:
        yaml_type, env_type = node_type, node_type

    env_parser = node.get("env_parser")
    if env_parser == "json":
        env_type = _env_type_for_json(node)
    elif env_parser in _ENV_PARSER_ENV_TYPES:
        env_type = _ENV_PARSER_ENV_TYPES[env_parser]
    return yaml_type, env_type, default


def _print_default(default, one_liner, env_var):
    if isinstance(default, str):
        return f'"{default}"'
    if isinstance(default, bool):
        return str(default).lower()
    if isinstance(default, list):
        if len(default) == 0:
            return "[]"

        if one_liner:
            if env_var:
                default = "\"" + " ".join([str(x) for x in default]) + "\""
            else:
                if isinstance(default[0], int):
                    default = "[" + ", ".join([f"{x}" for x in default]) + "]"
                else:
                    default = "[" + ", ".join([f'"{x}"' for x in default]) + "]"
        else:
            line = ""
            for x in default:
                line += f"\n  - {_print_default(x, True, False)}"
            return line
    return f"{default}"


def _get_param_line(name, node_type, default):
    if name == "api_key":
        return f"@param {name} - {node_type} - required"

    line = f"@param {name} - {node_type} - optional"
    if default is not None:
        line += f" - default: {_print_default(default, True, False)}"
    return line


def _get_env_lines(node, full_name, node_type, default):
    if _is_node_section(node):
        return ""

    env_vars = node.get("env_vars", [])
    if not env_vars and "no-env" not in node.get("tags", []):
        env_vars = ["DD_" + full_name.replace(".", "_").upper()]

    res = ""
    for var in env_vars:
        if var.startswith("DD_PROCESS_AGENT_"):
            continue
        if full_name == "api_key":
            res += f"@env {var} - {node_type} - required"
        else:
            res += f"@env {var} - {node_type} - optional"
            if default is not None:
                res += f" - default: {_print_default(default, True, True)}"
        res += "\n"
    return res.rstrip()


def _get_example(node, indent_level, name, default):
    line = f"{name}:"

    if not _is_node_section(node):
        if name == "api_key" and indent_level == 0:
            # the API key is an exception to the format, we don't show the default
            pass
        else:
            line += " " + node.get("example", _print_default(default, False, False))

    if indent_level == 0:
        line = textwrap.indent(line, " ", lambda line: True)
    return line


def _render_block(full_name, doc, example, indent_level):
    prefix = ""
    if indent_level > 0:
        prefix = " " + "  " * indent_level

    doc = textwrap.indent(doc, "# ", lambda line: True)
    block = textwrap.indent(doc, prefix)
    block += "\n\n"

    if full_name == "api_key":
        block = textwrap.indent(block, "#", lambda line: True)
        block += example
    else:
        block += textwrap.indent(example, prefix)
        block = textwrap.indent(block, "#", lambda line: True)

    return block


def _render_node(full_name, name, node, indent_level, os_target):
    yaml_node_type, env_node_type, default = _get_node_types_and_default(full_name, node, os_target)

    param_line = _get_param_line(name, yaml_node_type, default)
    env_lines = _get_env_lines(node, full_name, env_node_type, default)
    example = _get_example(node, indent_level, name, default)

    doc = "\n".join([x for x in [param_line, env_lines, _get_platform_version(node["description"], os_target)] if x])
    return _render_block(full_name, doc, example, indent_level) + "\n\n"


def _get_header(node):
    title = node.get("title")
    if title is None:
        return ""
    outline = "#" * (len(title) + 6)
    return f"{outline}\n## {title} ##\n{outline}\n\n"


def _render(build_type, os_target, previous_path, name, node, indent_level):
    if not _should_render(build_type, node):
        return ""

    full_name = previous_path + "." + name if previous_path != "" else name

    template = _render_node(full_name, name, node, indent_level, os_target)

    child_nodes = _filter_hidden_nodes(node.get("properties", {}), os_target)
    for child_name, child in child_nodes.items():
        template += _render(build_type, os_target, full_name, child_name, child, indent_level + 1)

    header = _get_header(node)
    return header + template


def generate_template(schema_file, dest, build_type, os_target):
    schema = resolve_schema(schema_file)

    config_template = ""
    child_nodes = _filter_hidden_nodes(schema.get("properties", {}), os_target)
    for child_name, child in child_nodes.items():
        config_template += _render(build_type, os_target, "", child_name, child, 0)

    final_render = [line.strip() for line in config_template.strip().split("\n")]
    with open(dest, "w") as f:
        f.write("\n".join(final_render) + "\n")


@task(
    help={
        "schema": "Path to the enriched schema YAML file (mandatory).",
        "build_type": f"Build type to generate the template for. One of: {', '.join(VALID_BUILD_TYPES)}",
        "os_target": f"Target OS. One of: {', '.join(VALID_OS_TARGETS)}",
        "output": "Path to the output file.",
    }
)
def template(ctx, schema, build_type, os_target, output):
    """
    Generate a config template for a specific build type and OS from an enriched schema file.
    """
    if build_type not in VALID_BUILD_TYPES:
        raise Exit(f"Invalid build_type '{build_type}'. Must be one of: {', '.join(VALID_BUILD_TYPES)}", code=1)

    if os_target not in VALID_OS_TARGETS:
        raise Exit(f"Invalid os_target '{os_target}'. Must be one of: {', '.join(VALID_OS_TARGETS)}", code=1)

    if not os.path.isfile(schema):
        raise Exit(f"Schema file not found: {schema}", code=1)

    generate_template(schema, output, build_type, os_target)
    print(f"Template written to {output}")


@task(
    help={
        "core_schema": "Path to the enriched core agent schema YAML file (mandatory).",
        "sysprobe_schema": "Path to the enriched system-probe schema YAML file (mandatory).",
        "output_dir": "Directory where all generated templates will be written.",
    }
)
def template_all(ctx, core_schema, sysprobe_schema, output_dir):
    """
    Generate all config templates (all build types x all OS targets) from the enriched schema files.
    """
    for path, label in [(core_schema, "core_schema"), (sysprobe_schema, "sysprobe_schema")]:
        if not os.path.isfile(path):
            raise Exit(f"Schema file not found for {label}: {path}", code=1)

    os.makedirs(output_dir, exist_ok=True)

    schema_for_build_type = {
        build_type: sysprobe_schema if build_type in _SYSPROBE_BUILD_TYPES else core_schema
        for build_type in VALID_BUILD_TYPES
    }

    for build_type, schema in schema_for_build_type.items():
        for os_target in VALID_OS_TARGETS:
            dest = os.path.join(output_dir, f"{build_type}_{os_target}.yaml")
            generate_template(schema, dest, build_type, os_target)
            print(f"  {dest}")


def main(argv):
    """CLI entry point for `bazel run //tasks/schema:schema_template` and direct
    `python -m` invocation. Mirrors the `template` invoke task, minus the
    Context dependency, so it can be called from a Bazel py_binary
    without going through the full invoke task collection."""
    if len(argv) != 5:
        print(
            f"usage: {argv[0]} <schema.yaml> <build_type> <os_target> <output.yaml>",
            file=sys.stderr,
        )
        return 2
    _, schema, build_type, os_target, output = argv

    if build_type not in VALID_BUILD_TYPES:
        print(
            f"Invalid build_type '{build_type}'. Must be one of: {', '.join(VALID_BUILD_TYPES)}",
            file=sys.stderr,
        )
        return 1
    if os_target not in VALID_OS_TARGETS:
        print(
            f"Invalid os_target '{os_target}'. Must be one of: {', '.join(VALID_OS_TARGETS)}",
            file=sys.stderr,
        )
        return 1
    if not os.path.isfile(schema):
        print(f"Schema file not found: {schema}", file=sys.stderr)
        return 1

    generate_template(schema, output, build_type, os_target)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
