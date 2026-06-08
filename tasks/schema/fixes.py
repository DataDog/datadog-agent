#!/usr/bin/env python


"""
This script fixes the OS differences in the schema that are hard to capture automatically. This includes information
decided at runtime by the Agent, OS based description and more.
"""

# Available Variables
#
# Most of those can be change by customers and are resolved at runtime when the Agent start. The following are the most
# likely values.
#
# conf_path:
#     linux: /etc/datadog-agent/
#     windows: programdata Dir, likely c:\programdata\datadog
#     darwin: /opt/datadog-agent/etc/
# install_path:
#     linux: /opt/datadog-agent/
#     windows: likely c:\Program Files\Datadog\Datadog Agent
#     darwin: /opt/datadog-agent
# run_path:
#    linux: {install_path}/run
#    darwin: /opt/datadog-agent/run
#    windows: {conf_path}\run
# log_path:
#     linux: /var/log/datadog
#     darwin: /opt/datadog-agent/logs
#     windows: c:\programdata\datadog\logs

core_defaults = {
    "api_key": "",
    "container_cgroup_root": "/host/sys/fs/cgroup/",
    "container_proc_root": "/host/proc",
    "sbom.cache_directory": "${run_path}/sbom-agent",
    "agent_ipc.socket_path": "${run_path}/agent_ipc.socket",
    "logs_config.open_files_limit": {
        "darwin": 200,
        "other": 500,
    },
    "use_networkv2_check": {"darwin": False, "other": True},
    "network_check.use_core_loader": {"darwin": False, "other": True},
    "kubernetes_kubelet_podresources_socket": {
        "windows": "\\\\.\\pipe\\kubelet-pod-resources",
        "other": "/var/lib/kubelet/pod-resources/kubelet.sock",
    },
    "kubernetes_kubelet_deviceplugins_socketdir": {
        "windows": "\\\\.\\pipe\\kubelet-device-plugins",
        "other": "/var/lib/kubelet/device-plugins",
    },
    "logs_config.run_path": "${run_path}",
    "run_path": "${run_path}",
    "process_config.dd_agent_bin": {
        "linux": "${install_path}/bin/agent/agent",
        "darwin": "${install_path}/bin/agent/agent",
        "windows": "${install_path}/bin/agent.exe",
    },
    "confd_path": "${conf_path}/conf.d",
    "shared_library_check.library_folder_path": "${conf_path}/checks.d",
    "additional_checksd": "${conf_path}/checks.d",
    "GUI_port": {
        "linux": -1,
        "other": 5002,
    },
    "security_agent.log_file": "${log_path}/security-agent.log",
    "process_config.log_file": "${log_path}/process-agent.log",
    "private_action_runner.log_file": "${log_path}/private-action-runner.log",
    "dogstatsd_socket": {
        "linux": "/var/run/datadog/dsd.socket",
        "other": "",
    },
    "apm_config.receiver_socket": {
        "linux": "/var/run/datadog/apm.socket",
        "other": "",
    },
    "logs_config.streaming.streamlogs_log_file": "${log_path}/streamlogs_info/streamlogs.log",
    "system_tray.log_file": "${conf_path}/logs/ddtray.log",
    # setting duplicated for some reasons between system-probe and core-agent config
    "runtime_security_config.socket": {
        "windows": "localhost:3335",
        "other": "${install_path}/run/runtime-security.sock",
    },
}

sysprobe_defaults = {
    "log_file": "${log_path}/system-probe.log",
    "system_probe_config.bpf_dir": "${install_path}/embedded/share/system-probe/ebpf",
    "system_probe_config.process_service_inference.enabled": {
        "windows": True,
        "other": False,
    },
    "system_probe_config.sysprobe_socket": {
        "linux": "${run_path}/sysprobe.sock",
        "darwin": "/opt/datadog-agent/run/sysprobe.sock",
        "windows": "\\\\.\\pipe\\dd_system_probe",
    },
    "runtime_security_config.security_profile.dir": "${run_path}/runtime-security/profiles",
    "runtime_security_config.activity_dump.local_storage.output_directory": "${run_path}/runtime-security/profiles",
    "runtime_security_config.policies.dir": {
        "windows": "${conf_path}/runtime-security.d",
        "other": "/etc/datadog-agent/runtime-security.d",  # hardcoded for both, probably doesn't work on darwin
    },
    # setting duplicated for some reasons between system-probe and core-agent config
    "runtime_security_config.socket": {
        "windows": "localhost:3335",
        "other": "${install_path}/run/runtime-security.sock",
    },
    "network_config.direct_send": {
        "linux": True,
        "other": False,
    },
    "discovery.enabled": {
        "linux": True,
        "other": False,
    },
    "discovery.use_system_probe_lite": {
        "linux": True,
        "other": False,
    },
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


def fix_defaults(core_schema, sysprobe_schema):
    for schema, custom_defaults in [[core_schema, core_defaults], [sysprobe_schema, sysprobe_defaults]]:
        for key, default in custom_defaults.items():
            node = fetch_node(schema, key)

            if "example" in node:
                del node["example"]

            if isinstance(default, str):
                node["default"] = default
            elif isinstance(default, dict):
                if "default" in node:
                    del node["default"]
                node["platform_default"] = default
            else:
                raise RuntimeError(f"unknown custom default type {type(default)} for {key}")
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


def fix_full_agent_only(core_schema, sysprobe_schema):
    for key in full_agent_only_paths:
        node = fetch_node(core_schema, key)
        if "tags" not in node:
            node["tags"] = []
        node["tags"].append("full-agent-only:true")
    return core_schema, sysprobe_schema


def fix_schema(core_schema, sysprobe_schema):
    core_schema, sysprobe_schema = fix_defaults(core_schema, sysprobe_schema)
    core_schema, sysprobe_schema = fix_full_agent_only(core_schema, sysprobe_schema)
    core_schema, sysprobe_schema = fix_tags(core_schema, sysprobe_schema)
    core_schema, sysprobe_schema = fix_missing_env_doc(core_schema, sysprobe_schema)
    core_schema, sysprobe_schema = fix_env_parsers(core_schema, sysprobe_schema)

    # special edge case for api_key
    core_schema["properties"]["api_key"]["type"] = "string"

    return core_schema, sysprobe_schema
