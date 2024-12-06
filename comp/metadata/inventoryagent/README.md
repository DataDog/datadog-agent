# Inventory Agent Payload

This package populates some of the agent-related fields in the `inventories` product in DataDog. More specifically the
`datadog-agent` table.

This is enabled by default but can be turned off using `inventories_enabled` config.

The payload is sent every 10min (see `inventories_max_interval` in the config) or whenever it's updated with at most 1
update every minute (see `inventories_min_interval`).

# Content

The `Set` method from the component allow the rest of the codebase to add any information to the payload.

## Agent Configuration

The agent configurations are scrubbed from any sensitive information (same logic than for the flare). The `Format`
section goes into more default about what configuration is sent.

Sending Agent configuration can be disabled using `inventories_configuration_enabled`.

# Format

The payload is a JSON dict with the following fields

- `hostname` - **string**: the hostname of the agent as shown on the status page.
- `uuid` - **string**: a unique identifier of the agent, used in case the hostname is empty.
- `timestamp` - **int**: the timestamp when the payload was created.
- `agent_metadata` - **dict of string to JSON type**:
  - `hostname_source` - **string**: the source for the agent hostname (see pkg/util/hostname/providers.go:GetWithProvider).
  - `agent_version` - **string**: the version of the Agent.
  - `agent_startup_time_ms` - **int**: the Agent startup timestamp (Unix milliseconds timestamp).
  - `flavor` - **string**: the flavor of the Agent. The Agent can be build under different flavor such as standalone
    dogstatsd, iot, serverless ... (see `pkg/util/flavor` package).
  - `config_apm_dd_url` - **string**: the configuration value `apm_config.dd_url` (scrubbed)
  - `config_dd_url` - **string**: the configuration value `dd_url` (scrubbed)
  - `config_site` - **string**: the configuration value `site` (scrubbed)
  - `config_logs_dd_url` - **string**: the configuration value `logs_config.logs_dd_url` (scrubbed)
  - `config_logs_socks5_proxy_address` - **string**: the configuration value `logs_config.socks5_proxy_address` (scrubbed)
  - `config_no_proxy` - **array of strings**: the configuration value `proxy.no_proxy` (scrubbed)
  - `config_process_dd_url` - **string**: the configuration value `process_config.process_dd_url` (scrubbed)
  - `config_proxy_http` - **string**: the configuration value `proxy.http` (scrubbed)
  - `config_proxy_https` - **string**: the configuration value `proxy.https` (scrubbed)
  - `config_eks_fargate` - **bool**: the configuration value `eks_fargate`
  - `install_method_tool` - **string**: the name of the tool used to install the agent (ie, Chef, Ansible, ...).
  - `install_method_tool_version` - **string**: the tool version used to install the agent (ie: Chef version, Ansible
    version, ...). This defaults to `"undefined"` when not installed through a tool (like when installed with apt, source
    build, ...).
  - `install_method_installer_version` - **string**:  The version of Datadog module (ex: the Chef Datadog package, the Datadog Ansible playbook, ...).
  - `logs_transport` - **string**:  The transport used to send logs to Datadog. Value is either `"HTTP"` or `"TCP"` when logs collection is
    enabled, otherwise the field is omitted.
  - `feature_fips_enabled` - **bool**: True if the Datadog Agent is in FIPS mode (see: `fips.enabled` config option).
  - `feature_cws_enabled` - **bool**: True if the Cloud Workload Security is enabled (see: `runtime_security_config.enabled`
    config option).
  - `feature_process_enabled` - **bool**: True if the Process Agent has process collection enabled
     (see: `process_config.process_collection.enabled` config option).
  - `feature_process_language_detection_enabled` - **bool**: True if process language detection is enabled
     (see: `language_detection.enabled` config option).
  - `feature_processes_container_enabled` - **bool**: True if the Process Agent has container collection enabled
     (see: `process_config.container_collection.enabled`)
  - `feature_networks_enabled` - **bool**: True if the Network Performance Monitoring is enabled (see:
    `network_config.enabled` config option in `system-probe.yaml`).
  - `feature_oom_kill_enabled` - **bool**: True if the OOM Kill check is enabled for System Probe (see: `system_probe_config.enable_oom_kill` config option in `system-probe.yaml`).
  - `feature_tcp_queue_length_enabled` - **bool**: True if TCP Queue Length check is enabled in System Probe (see: `system_probe_config.enable_tcp_queue_length` config option in `system-probe.yaml`).
  - `system_probe_telemetry_enabled` - **bool**: True if Telemetry is enabled in the System Probe (see: `system_probe_config.telemetry_enabled` config option in `system-probe.yaml`).
  - `system_probe_core_enabled` - **bool**: True if CO-RE is enabled in the System Probe (see: `system_probe_config.enable_co_re` config option in `system-probe.yaml`).
  - `system_probe_runtime_compilation_enabled` - **bool**: True if Runtime Compilation is enabled in the System Probe (see: `system_probe_config.enable_runtime_compiler` config option in `system-probe.yaml`).
  - `system_probe_kernel_headers_download_enabled` - **bool**: True if Kernel header downloading is enabled in the System Probe (see: `system_probe_config.enable_kernel_header_download` config option in `system-probe.yaml`).
  - `system_probe_prebuilt_fallback_enabled` - **bool**: True if the System Probe will fallback to prebuilt when other options fail (see: `system_probe_config.allow_prebuilt_fallback` config option in `system-probe.yaml`).
  - `system_probe_max_connections_per_message` - **int**: The maximum number of connections per message (see: `system_probe_config.max_conns_per_message` config option in `system-probe.yaml`).
  - `system_probe_track_tcp_4_connections` - **bool**: True if tracking TCPv4 connections is enabled in the System Probe (see: `network_config.collect_tcp_v4` config option in `system-probe.yaml`).
  - `system_probe_track_tcp_6_connections` - **bool**: True if tracking TCPv6 connections is enabled in the System Probe (see: `network_config.collect_tcp_v6` config option in `system-probe.yaml`).
  - `system_probe_track_udp_4_connections` - **bool**: True if tracking UDPv4 connections is enabled in the System Probe (see: `network_config.collect_udp_v4` config option in `system-probe.yaml`).
  - `system_probe_track_udp_6_connections` - **bool**: True if tracking UDPv6 connections is enabled in the System Probe (see: `network_config.collect_udp_v6` config option in `system-probe.yaml`).
  - `system_probe_protocol_classification_enabled` - **bool**: True if protocol classification is enabled in the System Probe (see: `network_config.enable_protocol_classification` config option in `system-probe.yaml`).
  - `system_probe_gateway_lookup_enabled` - **bool**: True if gateway lookup is enable in the System Probe (see: `network_config.enable_gateway_lookup` config option in `system-probe.yaml`).
  - `system_probe_root_namespace_enabled` - **bool**: True if the System Probe will run in the root namespace of the host (see: `network_config.enable_root_netns` config option in `system-probe.yaml`).
  - `feature_networks_http_enabled` - **bool**: True if HTTP monitoring is enabled for Network Performance Monitoring (see: `service_monitoring_config.enable_http_monitoring` config option in `system-probe.yaml`).
  - `feature_networks_https_enabled` - **bool**: True if HTTPS monitoring is enabled for Universal Service Monitoring (see: `service_monitoring_config.tls.native.enabled` config option in `system-probe.yaml`).
  - `feature_remote_configuration_enabled` - **bool**: True if Remote Configuration is enabled (see: `remote_configuration.enabled` config option).
  - `feature_usm_enabled` - **bool**: True if Universal Service Monitoring is enabled (see: `service_monitoring_config.enabled` config option in `system-probe.yaml`)
  - `feature_usm_http2_enabled` - **bool**: True if HTTP2 monitoring is enabled for Universal Service Monitoring (see: `service_monitoring_config.enable_http2_monitoring` config option in `system-probe.yaml`).
  - `feature_usm_kafka_enabled` - **bool**: True if Kafka monitoring is enabled for Universal Service Monitoring (see: `service_monitoring_config.enable_kafka_monitoring` config option in `system-probe.yaml`)
  - `feature_usm_postgres_enabled` - **bool**: True if Postgres monitoring is enabled for Universal Service Monitoring (see: `service_monitoring_config.enable_postgres_monitoring` config option in `system-probe.yaml`)
  - `feature_usm_redis_enabled` - **bool**: True if Redis monitoring is enabled for Universal Service Monitoring (see: `service_monitoring_config.enable_redis_monitoring` config option in `system-probe.yaml`)
  - `feature_usm_go_tls_enabled` - **bool**: True if HTTPS monitoring through GoTLS is enabled for Universal Service Monitoring (see: `service_monitoring_config.tls.go.enabled` config option in `system-probe.yaml`).
  - `feature_discovery_enabled` - **bool**: True if discovery module is enabled (see: `discovery.enabled` config option).
  - `feature_dynamic_instrumentation_enabled` - **bool**: True if dynamic instrumentation module is enabled (see: `dynamic_instrumentation.enabled` config option).
  - `feature_logs_enabled` - **bool**: True if the logs collection is enabled (see: `logs_enabled` config option).
  - `feature_cspm_enabled` - **bool**: True if the Cloud Security Posture Management is enabled (see:
    `compliance_config.enabled` config option).
  - `feature_cspm_host_benchmarks_enabled` - **bool**: True if host benchmarks are enabled (see:
    `compliance_config.host_benchmarks.enabled` config option).
  - `feature_apm_enabled` - **bool**: True if the APM Agent is enabled (see: `apm_config.enabled` config option).
  - `feature_otlp_enabled` - **bool**: True if the OTLP pipeline is enabled.
  - `feature_imdsv2_enabled` - **bool**: True if the IMDSv2 is enabled (see: `ec2_prefer_imdsv2` config option).
  - `feature_container_images_enabled` - **bool**: True if Container Images is enabled (see: `container_image.enabled` config option).
  - `feature_csm_vm_containers_enabled` - **bool**: True if VM Containers is enabled for Cloud Security Management (see: `sbom.enabled`, `container_image.enabled` and `sbom.container_image.enabled` config options).
  - `feature_csm_vm_hosts_enabled` - **bool**: True if VM Hosts is enabled for Cloud Security Management (see: `sbom.enable` and `sbom.host.enabled` config option).
  - `feature_cws_network_enabled` - **bool**: True if Network Monitoring is enabled for Cloud Workload Security (see: `event_monitoring_config.network.enabled` config option).
  - `feature_cws_remote_config_enabled` - **bool**: True if Remote Config is enabled for Cloud Workload Security (see: `runtime_security_config.remote_configuration.enabled` config option).
  - `feature_cws_security_profiles_enabled` - **bool**: True if Security Profiles is enabled for Cloud Workload Security (see: `runtime_security_config.activity_dump.enabled` config option).
  - `feature_usm_istio_enabled` - **bool**: True if Istio is enabled for Universal Service Monitoring (see: `service_monitoring_config.tls.istio.enabled` config option).
  - `feature_windows_crash_detection_enabled` - **bool**: True if Windows Crash Detection is enabled (see: `windows_crash_detection.enabled` config option).
  - `full_configuration` - **string**: the current Agent configuration scrubbed, including all the defaults, as a YAML
    string.
  - `provided_configuration` - **string**: the current Agent configuration (scrubbed), without the defaults, as a YAML
    string. This includes the settings configured by the user (throuh the configuration file, the environment or CLI),
    as well as any settings explicitly set by the agent (for example the number of workers is dynamically set by the
    agent itself based on the load).
  - `file_configuration` - **string**: the Agent configuration specified by the configuration file (scrubbed), as a YAML string.
    Only the settings written in the configuration file are included, and their value might not match what's applyed by the agent because they can be overriden by other sources.
  - `environment_variable_configuration` - **string**: the Agent configuration specified by the environment variables (scrubbed), as a YAML string.
    Only the settings written in the environment variables are included, and their value might not match what's applyed by the agent because they can be overriden by other sources.
  - `agent_runtime_configuration` - **string**: the Agent configuration set by the agent itself (scrubbed), as a YAML string.
    Only the settings set by the agent itself are included, and their value might not match what's applyed by the agent because they can be overriden by other sources.
  - `remote_configuration` - **string**: the Agent configuration specified by the Remote Configuration (scrubbed), as a YAML string.
    Only the settings currently used by Remote Configuration are included, and their value might not match what's applyed by the agent because they can be overriden by other sources.
  - `fleet_policies_configuration` - **string**: the Agent configuration specified by the Fleet Automation Policies (scrubbed), as a YAML string.
    Only the settings currently used by Fleet Automation Policies are included, and their value might not match what's applyed by the agent since they can be overriden by other sources.
  - `cli_configuration` - **string**: the Agent configuration specified by the CLI (scrubbed), as a YAML string.
    Only the settings set in the CLI are included, they cannot be overriden by any other sources.
  - `source_local_configuration` - **string**: the Agent configuration synchronized from the local Agent process, as a YAML string.
  - `ecs_fargate_task_arn` - **string**: if the Agent runs in ECS Fargate, contains the Agent's Task ARN. Else, is empty.
  - `ecs_fargate_cluster_name` - **string**: if the Agent runs in ECS Fargate, contains the Agent's cluster name. Else, is empty.
  - `fleet_policies_applied` -- **array of string**: The Fleet Policies that have been applied to the agent, if any. Is empty if no policy is applied.

("scrubbed" indicates that secrets are removed from the field value just as they are in logs)

## Example Payload

Here an example of an inventory payload:

```
{
    "agent_metadata": {
        "agent_version": "7.37.0-devel+git.198.68a5b69",
        "config_apm_dd_url": "",
        "config_dd_url": "",
        "config_logs_dd_url": "",
        "config_logs_socks5_proxy_address": "",
        "config_no_proxy": [
            "http://some-no-proxy"
        ],
        "config_process_dd_url": "",
        "config_proxy_http": "",
        "config_proxy_https": "http://localhost:9999",
        "config_site": "",
        "feature_imdsv2_enabled": false,
        "feature_apm_enabled": true,
        "feature_cspm_enabled": false,
        "feature_cws_enabled": false,
        "feature_logs_enabled": true,
        "feature_networks_enabled": false,
        "feature_process_enabled": false,
        "feature_remote_configuration_enabled": false,
        "flavor": "agent",
        "hostname_source": "os",
        "install_method_installer_version": "",
        "install_method_tool": "undefined",
        "install_method_tool_version": "",
        "logs_transport": "HTTP",
        "full_configuration": "<entire yaml configuration for the agent>",
        "provided_configuration": "api_key: \"***************************aaaaa\"\ncheck_runners: 4\ncontainerd_namespace: []\ncontainerd_namespaces: []\npython_version: \"3\"\ntracemalloc_debug: false\nlog_level: \"warn\"",
        "file_configuration": "check_runners: 4\ncontainerd_namespace: []\ncontainerd_namespaces: []\npython_version: \"3\"\ntracemalloc_debug: false",
        "agent_runtime_configuration": "runtime_block_profile_rate: 5000",
        "environment_variable_configuration": "api_key: \"***************************aaaaa\"",
        "remote_configuration": "log_level: \"debug\"",
        "cli_configuration": "log_level: \"warn\"",
        "source_local_configuration": ""
    }
    "hostname": "my-host",
    "timestamp": 1631281754507358895
}
```
