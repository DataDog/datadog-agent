#!/usr/bin/env python

import yaml
import sys
import re
import textwrap
import os


# Available Variables
#
# Most of those can be change by customers and are resolved at runtime when the Agent start. The following are the most
# likely values.
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
}

# Exception to the default schema. Thosee should be merged in the schema at some point

# Those are all the settings with custom env parser in the Agent.
custom_env_parsers = {
"apm_config.instrumentation.enabled_namespaces": "JSON array of strings",
"apm_config.instrumentation.disabled_namespaces": "JSON array of strings",
"apm_config.instrumentation.lib_versions": "JSON array of strings",
"apm_config.instrumentation.targets": "JSON array of strings",
"apm_config.features": "coma or space separated list of strings",
"apm_config.ignore_resources": "coma separated list of strings",
"apm_config.filter_tags.require": "space separated list of strings or JSON array of strings",
"apm_config.filter_tags.reject": "space separated list of strings or JSON array of strings",
"apm_config.filter_tags_regex.require": "space separated list of strings or JSON array of strings",
"apm_config.filter_tags_regex.reject": "space separated list of strings or JSON array of strings",
"apm_config.obfuscation.credit_cards.keep_values": "space separated list of strings or JSON array of strings",
"apm_config.replace_tags": "JSON object of string to string",
"apm_config.analyzed_spans": "coma separated list of key-value pairs",
"apm_config.peer_tags": "JSON array of strings",
"otelcollector.converter.features": "coma and space separated list of strings",
"dogstatsd_mapper_profiles": "JSON list of objects",
"process_config.custom_sensitive_words": "space separated list of strings or JSON array of strings",
"service_monitoring_config.http.replace_rules": "JSON object of string to string",
"service_monitoring_config.http_replace_rules": "JSON object of string to string",
"network_config.http_replace_rules": "JSON object of string to string",
}

# Setting declared with BindEnv() don't have a type or a default but some of them are still listed in the config
# example. Until the team migrate to BindEnvAndSetDefault we use the following list pulled from the config template.
#
# It's is highly possible that the information from the template are no longer accurate. Migrating to
# BindEnvAndSetDefault is still a priority.
type_exception = {
        "api_key": ("string", "string", ""),
        "site": ("string", "string", "datadoghq.com"),
        "dd_url": ("string", "string", "https://app.datadoghq.com"),
        "procfs_path": ("string", "string", None),
        "logs_config.logs_dd_url": ("string", "string", ""),
        "logs_config.processing_rules": ("list of custom objects", "list of custom objects", ""),
        "apm_config.env": ("string", "string", "none"),
        "apm_config.apm_non_local_traffic": ("boolean", "boolean", False),
        "apm_config.apm_dd_url": ("string", "string", None),
        "apm_config.max_traces_per_second": ("integer", "integer", 10),
        "apm_config.target_traces_per_second": ("integer", "integer", 10),
        "apm_config.errors_per_second": ("integer", "integer", 10),
        "apm_config.max_events_per_second": ("integer", "integer", 200),
        "apm_config.max_memory": ("integer", "integer", 500000000),
        "apm_config.max_cpu_percent": ("integer", "integer", 50),
        "apm_config.replace_tags": ("list of objects", "list of objects", None),
        "apm_config.ignore_resources": ("list of strings", "coma separated list of strings", []),
        "apm_config.log_file": ("string", "string", None),
        "apm_config.connection_limit": ("integer", "integer", 2000),
        "apm_config.peer_tags": ("list of strings", "list of strings", []),
        "apm_config.additional_endpoints": ("object", "object", {}),
        "apm_config.trace_buffer": ("integer", "integer", 0),
        "apm_config.probabilistic_sampler.enabled": ("boolean", "boolean", False),
        "apm_config.probabilistic_sampler.sampling_percentage": ("float", "float", 0),
        "apm_config.probabilistic_sampler.hash_seed": ("integer", "integer", 0),
        "apm_config.profiling_receiver_timeout": ("integer", "integer", 5),
        "apm_config.internal_profiling.enabled": ("boolean", "boolean", False),
        "process_config.enabled": ("boolean", "boolean", False),
        "process_config.intervals.container": ("integer", "integer", 10),
        "process_config.intervals.container_realtime": ("integer", "integer", 2),
        "process_config.intervals.process": ("integer", "integer", 10),
        "process_config.intervals.process_realtime": ("integer", "integer", 2),
        "process_config.blacklist_patterns": ("list of strings", "space separated list of strings", []),
        "process_config.dd_agent_env": ("string", "string", ""),
        "process_config.scrub_args": ("boolean", "boolean", True),
        "process_config.custom_sensitive_words": ("list of strings", "space separated list of strings", []),
        "network_path.collector.filters": ("list of custom objects", "list of custom objects", None),
        "bind_host": ("string", "string", "localhost"),
        "dogstatsd_mapper_profiles": ("list of custom object", "list of custom object", None),
        "metadata_providers": ("list of custom object", "list of custom object", None),
        "config_providers": ("list of custom object", "list of custom object", None),
        "container_cgroup_root": ("string", "string", "/host/sys/fs/cgroup/"),
        "container_proc_root": ("string", "string", "/host/proc"),
        "listeners": ("list of key:value elements", "list of key:value elements", None),
        "admission_controller.pod_owners_cache_validity": ("integer", "integer", 10),
        "admission_controller.auto_instrumentation.init_resources.cpu": ("string", "string", None),
        "admission_controller.auto_instrumentation.init_resources.memory": ("string", "string", None),
        "admission_controller.auto_instrumentation.init_security_context": ("json", "json", None),
        "cluster_name": ("string", "string", None),
        "prometheus_scrape.checks": ("custom object", "custom object", None),
        "network_config.enabled": ("boolean", "boolean", False),
        "network_devices.autodiscovery.workers": ("integer", "integer", 2),
        "network_devices.autodiscovery.discovery_interval": ("integer", "integer", 3600),
        "network_devices.autodiscovery.discovery_allowed_failures": ("integer", "integer", 3),
        "network_devices.autodiscovery.loader": ("string", "string", "python"),
        "network_devices.autodiscovery.min_collection_interval": ("integer", "integer", 15),
        "network_devices.autodiscovery.use_device_id_as_hostname": ("boolean", "boolean", False),
        "network_devices.autodiscovery.collect_topology": ("boolean", "boolean", True),
        "network_devices.autodiscovery.collect_vpn": ("boolean", "boolean", None),
        "network_devices.autodiscovery.ping.enabled": ("boolean", "boolean", None),
        "network_devices.autodiscovery.ping.timeout": ("integer", "integer", None),
        "network_devices.autodiscovery.ping.count": ("integer", "integer", None),
        "network_devices.autodiscovery.ping.interval": ("integer", "integer", None),
        "network_devices.autodiscovery.ping.linux.use_raw_socket": ("boolean", "boolean", None),
        "network_devices.autodiscovery.use_deduplication": ("boolean", "boolean", None),
        "network_devices.autodiscovery.configs": ("list", "string", None),
        "network_devices.snmp_traps.users": ("list of custom objects", "list of custom objects", None),
        "network_devices.netflow.listeners": ("custom object", "custom object", None),
        "network_devices.netflow.stop_timeout": ("integer", "integer", 5),
        "reverse_dns_enrichment.workers": ("integer", "integer", 10),
        "reverse_dns_enrichment.chan_size": ("integer", "integer", 5000),
        "reverse_dns_enrichment.cache.max_size": ("integer", "integer", 1000000),
        "reverse_dns_enrichment.rate_limiter.limit_per_sec": ("integer", "integer", 1000),
        "reverse_dns_enrichment.rate_limiter.limit_throttled_per_sec": ("integer", "integer", 1),
        "reverse_dns_enrichment.rate_limiter.throttle_error_threshold": ("integer", "integer", 10),
        "reverse_dns_enrichment.rate_limiter.recovery_intervals": ("integer", "integer", 5),
        "otlp_config.receiver.protocols.grpc.endpoint": ("string", "string", "0.0.0.0:4317"),
        "otlp_config.receiver.protocols.grpc.transport": ("string", "string", "tmp"),
        "otlp_config.receiver.protocols.grpc.max_recv_msg_size_mib": ("integer", "integer", 4),
        "otlp_config.receiver.protocols.http.endpoint": ("string", "string", "0.0.0.0:4138"),
        "otlp_config.metrics.resource_attributes_as_tags": ("boolean", "boolean", False),
        "otlp_config.metrics.tag_cardinality": ("string", "string", "low"),
        "otlp_config.metrics.delta_ttl": ("integer", "integer", 3600),
        "otlp_config.metrics.histograms.mode": ("string", "string", "distributions"),
        "otlp_config.metrics.histograms.send_count_sum_metrics": ("boolean", "boolean", False),
        "otlp_config.metrics.histograms.send_aggregation_metrics": ("boolean", "boolean", False),
        "otlp_config.metrics.sums.cumulative_monotonic_mode": ("string", "string", "to_delta"),
        "otlp_config.metrics.sums.initial_cumulative_monotonic_value": ("string", "string", "auto"),
        "otlp_config.metrics.summaries.mode": ("string", "string", "gauges"),
        "otlp_config.debug.verbosity": ("string", "string", "normal"),
}

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
        ],
    "dcacf": [
        "ClusterAgent",
        "Common",
        "Logging",
        "ClusterChecks",
        "CloudFoundry",
        ],
    "security-agent": [
        "SecurityAgent",
        ],
}

def is_node_section(node):
    return node.get("node_type", "") == "section"

def should_render(build_type, node):
    for t in node["tags"]:
        if t.startswith("template_section:"):
            section = t.split(":")[1]
            return section in build_type_to_section[build_type]
    return True

def filter_hidden_nodes(nodes, os_target):
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

def order_items(nodes):
    res = []
    for name, node in nodes.items():
        tags = node.get("tags", [])
        for tag in tags:
            if tag.startswith("template_section_order:"):
                template_order = tag.split(":")[1]
                break
        else:
            print(f"error: {name} is public but has no template order")
            continue

        res.append((int(template_order), (name, node)))

    return [x[1] for x in sorted(res, key=lambda x: x[0])]

def get_platform_version(data, os_target):
    if isinstance(data, str):
        return data

    if os_target in data:
        return data[os_target]
    elif os_target == "container" and "linux" in data:
        return data["linux"]

    return data["other"]

def get_default_from_node(node, os_target):
    if "default" in node:
        default = node["default"]
    elif "platform_default" in node:
        default = get_platform_version(node["platform_default"], os_target)
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

def get_node_types_and_default(full_name, node, os_target):
    if is_node_section(node):
        return "custom object", None, None

    default = get_default_from_node(node, os_target)

    node_type = node.get("type")
    if node_type is None:
        return type_exception[full_name]

    for tag in node.get("tags", []):
        if tag.startswith("golang_type:"):
            node_type = tag.split(":")[1]

    if node_type == "array":
        if node["items"]["type"] == "string":
            yaml_type, env_type = "list of strings", "space separated list of strings"
        elif node["items"]["type"] == "object":
            yaml_type, env_type = "list of object", "JSON list of object"
        elif node["items"]["type"] == "number":
            yaml_type, env_type = "list of integers", "space separated list of integers"
        else:
            raise(Exception(f"unknown array of type: {node["items"]["type"]}"))
    elif node_type == "number":
        yaml_type, env_type = "integer", "integer"
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

    env_type = custom_env_parsers.get(full_name, env_type)
    return yaml_type, env_type, default

def print_default(default, one_liner, env_var):
    if type(default) is str:
        return f"\"{default}\""
    if type(default) is bool:
        return str(default).lower()
    if type(default) is list:
        if len(default) == 0:
            return "[]"

        if one_liner:
            if env_var:
                default = "'"+" ".join([str(x) for x in default])+"'"
            else:
                if type(default[0]) is int:
                    default = "["+", ".join([f"{x}" for x in default])+"]"
                else:
                    default = "["+", ".join([f"\"{x}\"" for x in default])+"]"
        else:
            line = ""
            for x in default:
                line += f"\n  - {print_default(x, True, False)}"
            return line
    return f"{default}"

def get_param_line(name, node_type, default):
    if name == "api_key":
        # API key is the exception: it's the only required field
        return f"@param {name} - {node_type} - required"

    line = f"@param {name} - {node_type} - optional"
    if default is not None:
        line += f" - default: {print_default(default, True, False)}"
    return line

def get_env_lines(node, full_name, node_type, default):
    if is_node_section(node):
        return ""

    env_vars = node.get("env_vars", [])
    # generate the default env vars
    if not env_vars and "no-env" not in node.get("tags", []):
        env_vars = ["DD_"+ full_name.replace(".", "_").upper()]

    res = ""
    for var in env_vars:
        if full_name == "api_key":
            # API key is the exception: it's the only required field
            res += f"@env {var} - {node_type} - required"
        else:
            res += f"@env {var} - {node_type} - optional"
            if default is not None:
                res += f" - default: {print_default(default, True, True)}"
        res += "\n"
    return res.rstrip()

def get_example(node, indent_level, name, default):
    line = f"{name}:"
    if not is_node_section(node):
        line += " " + node.get("example", print_default(default, False, False))

    # edge case for the top level to match the current template
    if indent_level == 0:
        line = textwrap.indent(line, " ", lambda line: True)
    return line

def render_block(full_name, doc, example, indent_level):
    prefix = ""
    if indent_level > 0:
        prefix = " " + "  "*indent_level

    doc = textwrap.indent(doc, "# ", lambda line: True)
    block = textwrap.indent(doc, prefix)
    block += "\n\n"

    if full_name == "api_key":
        # API key is the exception: it's not commented out
        block = textwrap.indent(block, "#", lambda line: True)
        block += example
    else:
        block += textwrap.indent(example, prefix)
        block = textwrap.indent(block, "#", lambda line: True)

    return block

def render_node(full_name, name, node, indent_level, os_target):
    yaml_node_type, env_node_type, default = get_node_types_and_default(full_name, node, os_target)

    param_line = get_param_line(name, yaml_node_type, default)
    env_lines = get_env_lines(node, full_name, env_node_type, default)
    example = get_example(node, indent_level, name, default)

    doc = "\n".join([x for x in [param_line, env_lines, get_platform_version(node["description"], os_target)] if x])
    return render_block(full_name, doc, example, indent_level)+"\n\n"

def get_header(node):
    title = node.get("title")
    if title is None:
        return ""
    outline = "#"*(len(title)+6)
    return f"{outline}\n## {title} ##\n{outline}\n\n"

def render(build_type, os_target, previous_path, name, node, indent_level):
    if not should_render(build_type, node):
        return ""

    full_name = previous_path+"."+name if previous_path != "" else name

    template = render_node(full_name, name, node, indent_level, os_target)

    child_nodes = filter_hidden_nodes(node.get("properties", {}), os_target)
    for child_name, child in order_items(child_nodes):
        template += render(build_type, os_target, full_name, child_name, child, indent_level+1)

    header = get_header(node)
    return header+template

def generate_template(schema_file, dest, build_type, os_target):
    with open(schema_file, "r") as f:
        schema = yaml.safe_load(f)

    config_template =""
    child_nodes = filter_hidden_nodes(schema.get("properties", {}), os_target)
    for child_name, child in order_items(child_nodes):
        config_template += render(build_type, os_target, "", child_name, child, 0)

    with open(dest, "w") as f:
        for line in config_template.split("\n"):
            line = line.strip()
            f.write(line)
            f.write("\n")

def generate_all(core_schema, sysprobe_schema, dest_folder):
    for build_type, schema in {
            "agent-py3": core_schema,
            "iot-agent": core_schema,
            "dogstatsd": core_schema,
            "dca": core_schema,
            "dcacf": core_schema,
            "security-agent": core_schema,
            "system-probe": sysprobe_schema,
        }.items():
        for os_target in ["windows", "darwin", "linux"]:
            dest = os.path.join(dest_folder, build_type + "_" + os_target + ".yaml")
            generate_template(schema, dest, build_type, os_target)

if __name__ == "__main__":

    if len(sys.argv) == 4:
        generate_all(sys.argv[1], sys.argv[2], sys.argv[3])
    elif len(sys.argv) == 5:
        generate_template(sys.argv[1], sys.argv[2],sys.argv[3], sys.argv[4])
    else:
        print(f"Usage: {sys.argv[0]} <yaml schema> <output file> <build_type> <os_target>")
        print(f"or to generate all possible file: {sys.argv[0]} <core schema> <system-probe schema> <output folder>")
        sys.exit(1)
