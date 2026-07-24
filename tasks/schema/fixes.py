#!/usr/bin/env python


"""
This script fixes the OS differences in the schema that are hard to capture automatically. This includes information
decided at runtime by the Agent, OS based description and more.
"""

import os
import re

# Directory holding the Go config setup package, relative to this file.
SETUP_DIR = os.path.normpath(os.path.join(os.path.dirname(__file__), "..", "..", "pkg", "config", "setup"))

# Some GetPlatformDefault map values are Go identifiers or function calls rather than literals. They are
# platform-independent constants resolved at build time; map each to the value it returns on the matching platform.
GO_VALUE_RESOLUTIONS = {
    "defaultpaths.GetDefaultReceiverSocket()": "/var/run/datadog/apm.socket",
    "defaultpaths.GetDefaultStatsdSocket()": "/var/run/datadog/dsd.socket",
    "DefaultRuntimePoliciesDir": "/etc/datadog-agent/runtime-security.d",
}

# Matches `"<config.key>", GetPlatformDefault(map[string]interface{}{ <body> })`, allowing the key and the
# GetPlatformDefault call to be on separate lines. The body stops at the first `})` which closes the map literal;
# `${...}` substitutions inside string values never produce a `})`, so this stays unambiguous.
PLATFORM_DEFAULT_RE = re.compile(
    r'"(?P<key>[\w.]+)"\s*,\s*GetPlatformDefault\(map\[string\]interface\{\}\{(?P<body>.*?)\}\)',
    re.DOTALL,
)

# Matches a single `"platform": value` entry inside a GetPlatformDefault map literal.
PLATFORM_ENTRY_RE = re.compile(r'^"(?P<platform>\w+)"\s*:\s*(?P<value>.+?),?$')

# A `generate_const:<name>` tag records that a setting's default value comes from the Go constant
# `<name>` declared in pkg/config/setup, so the constant can be generated from the schema.
GENERATE_CONST_PREFIX = "generate_const:"

generate_const_settings = {
    "dynamic_instrumentation.debug_info_disk_cache.dir": "defaultDynamicInstrumentationDebugInfoDir",
    "forwarder_apikey_validation_interval": "DefaultAPIKeyValidationInterval",
    "forwarder_recovery_interval": "DefaultForwarderRecoveryInterval",
    "logs_config.auditor_ttl": "DefaultAuditorTTL",
    "logs_config.max_message_size_bytes": "DefaultMaxMessageSizeBytes",
    "network_path.collector.e2e_queries": "DefaultNetworkPathStaticPathE2eQueries",
    "network_path.collector.max_ttl": "DefaultNetworkPathMaxTTL",
    "network_path.collector.timeout": "DefaultNetworkPathTimeout",
    "network_path.collector.traceroute_queries": "DefaultNetworkPathStaticPathTracerouteQueries",
    "security_agent.cmd_port": "DefaultSecurityAgentCmdPort",
    "security_agent.internal_profiling.site": "DefaultSite",
    "serializer_compressor_kind": "DefaultCompressorKind",
    "serializer_zstd_compressor_level": "DefaultZstdCompressionLevel",
    "service_monitoring_config.tls.istio.envoy_path": "defaultEnvoyPath",
    "site": "DefaultSite",
    "system_probe_config.btf_output_dir": "defaultBTFOutputDir",
    "system_probe_config.internal_profiling.site": "DefaultSite",
    "system_probe_config.max_conns_per_message": "defaultConnsMessageBatchSize",
    "system_probe_config.offset_guess_threshold": "defaultOffsetThreshold",
    "system_probe_config.runtime_compiler_output_dir": "defaultRuntimeCompilerOutputDir",
    "logs_config.compression_kind": "DefaultLogCompressionKind",
    "logs_config.zstd_compression_level": "DefaultZstdCompressionLevel",
    "logs_config.batch_wait": "DefaultBatchWait",
    "logs_config.batch_max_concurrent_send": "DefaultBatchMaxConcurrentSend",
    "logs_config.batch_max_content_size": "DefaultBatchMaxContentSize",
    "logs_config.batch_max_size": "DefaultBatchMaxSize",
    "logs_config.input_chan_size": "DefaultInputChanSize",
    "logs_config.sender_backoff_factor": "DefaultLogsSenderBackoffFactor",
    "logs_config.sender_backoff_base": "DefaultLogsSenderBackoffBase",
    "logs_config.sender_backoff_max": "DefaultLogsSenderBackoffMax",
    "logs_config.sender_recovery_interval": "DefaultForwarderRecoveryInterval",
}

# Settings used by full agent but not by serverless

full_agent_only_paths = [
    "admission_controller",
    "allow_python_path_heuristics_failure",
    "appsec",
    "auto_team_tag_collection",
    "azure_hostname_style",
    "azure_metadata_api_version",
    "azure_metadata_timeout",
    "cel_workload_exclude",
    "check_sampler_allow_sketch_bucket_reset",
    "check_sampler_bucket_commits_count_expiry",
    "check_sampler_context_metrics",
    "check_sampler_expire_metrics",
    "check_sampler_stateful_metric_expiration_time",
    "checks_tag_cardinality",
    "clc_runner_enabled",
    "clc_runner_host",
    "clc_runner_id",
    "clc_runner_port",
    "clc_runner_remote_tagger_enabled",
    "clc_runner_server_readheader_timeout",
    "clc_runner_server_write_timeout",
    "cloud_foundry_bbs",
    "cloud_foundry_cc",
    "cloud_foundry_container_tagger",
    "cloud_foundry_garden",
    "cluster_agent",
    "cluster_checks",
    "cluster_name",
    "cluster_trust_chain",
    "collect_ccrid",
    "collect_ec2_instance_info",
    "collect_ec2_tags",
    "collect_ec2_tags_use_imds",
    "collect_gce_tags",
    "collect_gpu_tags",
    "compliance_config.check_interval",
    "compliance_config.check_max_events_per_run",
    "compliance_config.container_exclude",
    "compliance_config.container_include",
    "compliance_config.dir",
    "compliance_config.enabled",
    "compliance_config.exclude_pause_container",
    "compliance_config.host_benchmarks",
    "compliance_config.metrics",
    "compliance_config.opa",
    "compliance_config.run_in_system_probe",
    "compliance_config.xccdf",
    "container_image",
    "container_lifecycle",
    "csi",
    "data_plane",
    "disable_cluster_name_tag_key",
    "disable_unsafe_yaml",
    "disk_check",
    "djm_config",
    "docker_env_as_tags",
    "docker_labels_as_tags",
    "docker_query_timeout",
    "dogstatsd_host_socket_path",
    "dogstatsd_tag_cardinality",
    "ec2_imdsv2_transition_payload_enabled",
    "ec2_metadata_timeout",
    "ec2_metadata_token_lifetime",
    "ec2_prefer_imdsv2",
    "ec2_prioritize_instance_id_as_hostname",
    "ec2_use_dmi",
    "ec2_use_windows_prefix_detection",
    "ecs_agent_container_name",
    "ecs_agent_url",
    "ecs_collect_resource_tags_ec2",
    "ecs_deployment_mode",
    "ecs_metadata_retry_initial_interval",
    "ecs_metadata_retry_max_elapsed_time",
    "ecs_metadata_retry_timeout_factor",
    "ecs_metadata_timeout",
    "ecs_resource_tags_replace_colon",
    "ecs_task_cache_ttl",
    "ecs_task_collection_burst",
    "ecs_task_collection_enabled",
    "ecs_task_collection_rate",
    "enabled_rfc1123_compliant_cluster_name_tag",
    "exclude_ec2_tags",
    "exclude_gce_tags",
    "expvar_port",
    "external_metrics",
    "external_metrics_provider",
    "flare",
    "flare_provider_timeout",
    "gce_metadata_timeout",
    "gce_send_project_id_tag",
    "gpu",
    "ha_agent",
    "heroku_dyno",
    "host_aliases",
    "hostname_drift_initial_delay",
    "hostname_drift_recurring_interval",
    "hostname_force_config_as_canonical",
    "hostname_fqdn",
    "hostname_trust_uts_namespace",
    "hostprofiler",
    "hpa_configmap_name",
    "hpa_watcher_gc_period",
    "hpa_watcher_polling_freq",
    "ibm_metadata_timeout",
    "installer",
    "instrumentation_crd_controller",
    "integration_file_paths_allowlist",
    "integration_ignore_untrusted_file_params",
    "integration_security_excluded_checks",
    "integration_trusted_providers",
    "internal_profiling",
    "inventories_checks_configuration_enabled",
    "inventories_collect_cloud_provider_account_id",
    "inventories_configuration_enabled",
    "inventories_enabled",
    "inventories_first_run_delay",
    "inventories_max_interval",
    "inventories_min_interval",
    "iot_host",
    "jmx_check_period",
    "jmx_collection_timeout",
    "jmx_custom_jars",
    "jmx_java_tool_options",
    "jmx_max_ram_percentage",
    "jmx_max_restarts",
    "jmx_reconnection_thread_pool_size",
    "jmx_reconnection_timeout",
    "jmx_restart_interval",
    "jmx_statsd_client_buffer_size",
    "jmx_statsd_client_queue_size",
    "jmx_statsd_client_socket_timeout",
    "jmx_statsd_client_use_non_blocking",
    "jmx_statsd_telemetry_enabled",
    "jmx_telemetry_enabled",
    "jmx_thread_pool_size",
    "jmx_use_cgroup_memory_limit",
    "jmx_use_container_support",
    "kube_cache_sync_timeout_seconds",
    "kube_resources_namespace",
    "kubernetes_apiserver_ca_path",
    "kubernetes_apiserver_tls_verify",
    "kubernetes_event_collection_timeout",
    "kubernetes_informers_resync_period",
    "kubernetes_kubeconfig_path",
    "kubernetes_namespace_annotations_as_tags",
    "kubernetes_namespace_labels_as_tags",
    "kubernetes_node_annotations_as_host_aliases",
    "kubernetes_node_annotations_as_tags",
    "kubernetes_node_label_as_cluster_name",
    "kubernetes_node_labels_as_tags",
    "kubernetes_persistent_volume_claims_as_tags",
    "kubernetes_pod_annotations_as_tags",
    "kubernetes_pod_labels_as_tags",
    "kubernetes_resources_annotations_as_tags",
    "kubernetes_resources_labels_as_tags",
    "language_detection",
    "leader_election",
    "leader_election_default_resource",
    "leader_election_release_on_shutdown",
    "leader_lease_duration",
    "leader_lease_name",
    "metadata_endpoints_max_hostname_size",
    "metadata_ip_resolution_from_hostname",
    "metric_filterlist",
    "metric_filterlist_match_prefix",
    "metric_lookback",
    "metric_lookback.collection_interval",
    "metric_lookback.enabled",
    "metric_lookback.enabled_checks",
    "metric_tag_filterlist",
    "metric_tag_filterlist_adp_only",
    "metrics_port",
    "multi_secret_backends",
    "network_check",
    "network_devices",
    "network_path",
    "orchestrator_explorer",
    "otel_standalone",
    "otelcollector",
    "prioritize_go_check_loader",
    "prometheus_http_sd",
    "prometheus_scrape",
    "python3_linter_timeout",
    "python_lazy_loading",
    "remote_agent",
    "remote_tagger",
    "remote_updates",
    "reverse_dns_enrichment",
    "sbom",
    "secret_allowed_k8s_namespace",
    "secret_audit_file_max_size",
    "secret_backend_config",
    "secret_backend_remove_trailing_line_break",
    "secret_backend_type",
    "secret_image_to_handle",
    "secret_refresh_interval",
    "secret_refresh_on_api_key_failure_interval",
    "secret_refresh_scatter",
    "secret_scope_integration_to_their_k8s_namespace",
    "security_agent",
    "server_timeout",
    "shared_library_check",
    "snmp_listener",
    "statsd_metric_blocklist",
    "statsd_metric_blocklist_match_prefix",
    "synthetics",
    "system_tray",
    "trace_agent_host_socket_path",
    "use_diskv2_check",
    "use_networkv2_check",
    "vsock_addr",
    "windows_counter_init_failure_limit",
    "windows_counter_refresh_interval",
    "windows_use_pythonpath",
    "workloadmeta",
]


# extra_tags

core_extra_tags = {
    "system_tray": ["platform_only:windows"],
    "sbom.container_image": ["platform_only:linux"],
    "network_path": ["platform_only:windows,linux"],
}

system_probe_extra_tags = {
    "windows_crash_detection": ["platform_only:windows"],
}

# fix env_parser
#
# Some settings use custom env var parsing logic that cannot be captured automatically
# by the schema generator. The env_parser field documents the parsing strategy.

core_env_parsers = {
    "apm_config.analyzed_spans": "traces_span",
    "apm_config.ignore_resources": "csv_comma_separated",
    "apm_config.features": "comma_then_space_separated",
    "apm_config.filter_tags.require": "json_list_or_space_separated",
    "apm_config.filter_tags.reject": "json_list_or_space_separated",
    "apm_config.filter_tags_regex.require": "json_list_or_space_separated",
    "apm_config.filter_tags_regex.reject": "json_list_or_space_separated",
    "apm_config.obfuscation.credit_cards.keep_values": "json_list_or_space_separated",
    "otelcollector.converter.features": "comma_and_space_separated",
    "process_config.custom_sensitive_words": "json_list_or_comma_separated",
}

# fix custom env vars
#
# Some env vars had handled manually by custom code instead of the config

core_extra_env = {
    "proxy.https": "@env DD_PROXY_HTTPS - string - optional - default: \"\"",
    "proxy.http": "@env DD_PROXY_HTTP - string - optional - default: \"\"",
    "proxy.no_proxy": "@env DD_PROXY_NO_PROXY - space-separated list of strings - optional - default: []",
}


def fetch_node(root, key):
    curr = root
    for k in key.split("."):
        curr = curr["properties"][k]
    return curr


def try_fetch_node(root, key):
    try:
        return fetch_node(root, key)
    except KeyError:
        return None


def _strip_go_comment(line):
    """Strip a trailing `// ...` Go comment, ignoring `//` inside string or raw-string literals."""
    in_str = None  # '"' for interpreted strings, '`' for raw strings
    i = 0
    while i < len(line):
        c = line[i]
        if in_str == '"' and c == '\\':
            i += 2
            continue
        if in_str:
            if c == in_str:
                in_str = None
        elif c in '"`':
            in_str = c
        elif c == '/' and line[i + 1 : i + 2] == '/':
            return line[:i]
        i += 1
    return line


def _parse_go_value(value):
    """Resolve a single Go map value (literal, identifier, or call) to its Python equivalent."""
    value = value.strip()
    if value in ("true", "false"):
        return value == "true"
    if re.fullmatch(r"-?\d+", value):
        return int(value)
    if value.startswith('"') and value.endswith('"'):
        # Interpreted string literal. The values used here only ever escape backslashes and quotes.
        return value[1:-1].replace('\\\\', '\\').replace('\\"', '"')
    if value.startswith('`') and value.endswith('`'):
        # Raw string literal: content is taken verbatim.
        return value[1:-1]
    if value in GO_VALUE_RESOLUTIONS:
        return GO_VALUE_RESOLUTIONS[value]
    raise RuntimeError(f"cannot resolve Go GetPlatformDefault value {value!r}; add it to GO_VALUE_RESOLUTIONS")


def parse_platform_defaults():
    """Parse the GetPlatformDefault calls in pkg/config/setup into a {config_key: {platform: value}} mapping."""
    platform_defaults = {}
    for fname in sorted(os.listdir(SETUP_DIR)):
        if not fname.endswith(".go") or fname.endswith("_test.go"):
            continue
        with open(os.path.join(SETUP_DIR, fname)) as f:
            content = f.read()

        for match in PLATFORM_DEFAULT_RE.finditer(content):
            key = match.group("key")
            values = {}
            for line in match.group("body").splitlines():
                line = _strip_go_comment(line).strip()
                if not line:
                    continue
                entry = PLATFORM_ENTRY_RE.match(line)
                if entry:
                    values[entry.group("platform")] = _parse_go_value(entry.group("value"))
            if values:
                platform_defaults[key] = values

    return platform_defaults


def fix_defaults(core_schema, sysprobe_schema):
    # Platform-specific defaults pulled from the GetPlatformDefault calls in pkg/config/setup. A setting can live in
    # the core schema, the system-probe schema, or both (some are duplicated), so apply to whichever schemas have it.
    for key, values in parse_platform_defaults().items():
        for schema in (core_schema, sysprobe_schema):
            node = try_fetch_node(schema, key)
            if node is None:
                continue
            if "example" in node:
                del node["example"]
            if "default" in node:
                del node["default"]
            node["platform_default"] = values

    return core_schema, sysprobe_schema


def fix_tags(core_schema, sysprobe_schema):
    for schema, new_tags in [[core_schema, core_extra_tags], [sysprobe_schema, system_probe_extra_tags]]:
        for key, tags in new_tags.items():
            node = fetch_node(schema, key)
            node["tags"] = sorted(set(node.get("tags", []) + tags))
    return core_schema, sysprobe_schema


def fix_env_parsers(core_schema, sysprobe_schema):
    for key, parser in core_env_parsers.items():
        node = fetch_node(core_schema, key)
        node["env_parser"] = parser
    return core_schema, sysprobe_schema


def fix_missing_env_doc(core_schema, sysprobe_schema):
    # no extra env for sysprobe
    for schema, env_lines in [[core_schema, core_extra_env]]:
        for key, line in env_lines.items():
            node = fetch_node(schema, key)
            node["description"] = line + "\n" + node.get("description", "")
    return core_schema, sysprobe_schema


def fix_generate_const(core_schema, sysprobe_schema):
    # Tag each setting whose default is sourced from a pkg/config/setup constant with
    # `generate_const:<name>`. A setting can live in the core schema, the system-probe schema, or both,
    # so apply the tag to whichever schemas have it.
    for key, const in generate_const_settings.items():
        tag = GENERATE_CONST_PREFIX + const
        for schema in (core_schema, sysprobe_schema):
            node = try_fetch_node(schema, key)
            if node is None:
                continue
            node["tags"] = sorted(set(node.get("tags", []) + [tag]))
    return core_schema, sysprobe_schema


def fix_full_agent_only(core_schema, sysprobe_schema):
    for key in full_agent_only_paths:
        node = try_fetch_node(core_schema, key)
        if not node:
            continue
        if "tags" not in node:
            node["tags"] = []
        node["tags"].append("full-agent-only:true")
    return core_schema, sysprobe_schema


# A `template_section:<value>` tag records which config template a node belongs to.
TEMPLATE_SECTION_PREFIX = "template_section:"


def _template_section_values(node):
    """Return the set of `template_section:<value>` values carried by *node*'s tags."""
    return {
        tag[len(TEMPLATE_SECTION_PREFIX) :]
        for tag in node.get("tags", [])
        if isinstance(tag, str) and tag.startswith(TEMPLATE_SECTION_PREFIX)
    }


def _strip_redundant_template_section_tags(setting, parent_section_values):
    """Drop `template_section:<value>` tags from *setting* whose value is already
    carried by its parent section, then remove the `tags` list if it becomes empty."""
    tags = setting.get("tags")
    if not isinstance(tags, list):
        return
    kept = [
        tag
        for tag in tags
        if not (
            isinstance(tag, str)
            and tag.startswith(TEMPLATE_SECTION_PREFIX)
            and tag[len(TEMPLATE_SECTION_PREFIX) :] in parent_section_values
        )
    ]
    if kept:
        setting["tags"] = kept
    else:
        del setting["tags"]


def _clean_template_section_tags(section):
    """Recursively clean redundant `template_section` tags under *section*.

    A direct setting child's `template_section:<value>` tag is redundant when the
    enclosing section already carries the same value, so it is removed."""
    parent_values = _template_section_values(section)
    props = section.get("properties")
    if not isinstance(props, dict):
        return
    for child in props.values():
        if not isinstance(child, dict):
            continue
        if child.get("node_type") == "setting":
            _strip_redundant_template_section_tags(child, parent_values)
        elif child.get("node_type") == "section":
            _clean_template_section_tags(child)


def fix_redundant_template_section_tags(core_schema, sysprobe_schema):
    # A setting that repeats its parent section's `template_section:<value>` tag adds no
    # information — it is already implied by the enclosing section. Remove those redundant
    # tags, and drop any `tags` list left empty as a result.
    for schema in (core_schema, sysprobe_schema):
        _clean_template_section_tags(schema)
    return core_schema, sysprobe_schema


def fix_schema(core_schema, sysprobe_schema):
    core_schema, sysprobe_schema = fix_defaults(core_schema, sysprobe_schema)
    core_schema, sysprobe_schema = fix_generate_const(core_schema, sysprobe_schema)
    core_schema, sysprobe_schema = fix_full_agent_only(core_schema, sysprobe_schema)
    core_schema, sysprobe_schema = fix_tags(core_schema, sysprobe_schema)
    core_schema, sysprobe_schema = fix_missing_env_doc(core_schema, sysprobe_schema)
    core_schema, sysprobe_schema = fix_env_parsers(core_schema, sysprobe_schema)
    core_schema, sysprobe_schema = fix_redundant_template_section_tags(core_schema, sysprobe_schema)

    # special edge case for api_key
    core_schema["properties"]["api_key"]["type"] = "string"

    return core_schema, sysprobe_schema
