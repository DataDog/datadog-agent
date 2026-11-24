# Inventory Agent Payload

This package populates some of the agent-related fields in the `inventories` product in DataDog. More specifically the
`datadog-agent` table.

This is enabled by default but can be turned off using `inventories_enabled` config.

The payload is sent every 10min (see `inventories_max_interval` in the config) or whenever it's updated with at most 1
update every minute (see `inventories_min_interval`).

# Content

The `Set` method from the component allow the rest of the codebase to add any information to the payload.

## Shared payload code

`agentmetadata.NewData` and its `Init` method collect the common fields: versions, startup time, installation
information, hostname source, and infrastructure mode. `GetPayload` combines those fields with caller-supplied
metadata and the optional scrubbed configuration. The component calls `Init` only when `inventories_enabled` is true.

## Agent Configuration

The agent configurations are scrubbed from any sensitive information (same logic than for the flare). The `Format`
section goes into more detail about what configuration is sent.

Sending Agent configuration can be disabled using `inventories_configuration_enabled`.

# Format

The payload is a JSON dict with the following fields

- `hostname` - **string**: the hostname of the agent as shown on the status page.
- `uuid` - **string**: a unique identifier of the agent, used in case the hostname is empty.
- `timestamp` - **int**: the timestamp when the payload was created.
- `files_metadata` - **dict of string to object**: the scrubbed contents of the configuration files, keyed by their
  full file path. Only present when `inventories_configuration_enabled` is true. It contains at most 4 files: the main
  `datadog.yaml`, the `system-probe.yaml` in use, and the `security-agent.yaml` and `application_monitoring.yaml` files
  next to `datadog.yaml`. Files that can't be read are skipped and the field is omitted when no file is found. YAML
  setting order is preserved, while comments are omitted to avoid possibly outputting secrets. Each object contains:
  - `raw_config` - **string**: the scrubbed configuration file.
  - `hash` - **string**: the SHA-256 hash of `raw_config`.
  - `secrets` - **array of objects** (omitted if empty): the secret declarations in the file (see
    [Secret declarations in configuration files](#secret-declarations-in-configuration-files)).
    - `kind` - **string**: `enc` or `raw`.
    - `path` - **string**: the dotted path of the setting, relative to the file.
    - `handle` - **string** (omitted for `raw`): the secret handle used in `ENC[]`.
- `agent_metadata` - **dict of string to JSON type**:
  - `hostname_source` - **string**: the source for the agent hostname (see pkg/util/hostname/providers.go:GetWithProvider).
    Omitted when the provider is unknown or when the hostname comes from Fargate.
  - `agent_version` - **string**: the version of the Agent.
  - `package_version` - **string**: the version of the Agent package.
  - `agent_startup_time_ms` - **int**: the Agent startup timestamp (Unix milliseconds timestamp).
  - `flavor` - **string**: the flavor of the Agent. The Agent can be build under different flavor such as standalone
    dogstatsd, iot, serverless ... (see `pkg/util/flavor` package).
  - `fips_mode` - **bool**: True if the Datadog Agent is in FIPS mode.
  - `config_apm_dd_url` - **string**: the configuration value `apm_config.apm_dd_url` (scrubbed)
  - `config_dd_url` - **string**: the configuration value `dd_url` (scrubbed)
  - `config_site` - **string**: the configuration value `site` (scrubbed)
  - `config_logs_dd_url` - **string**: the configuration value `logs_config.logs_dd_url` (scrubbed)
  - `config_logs_socks5_proxy_address` - **string**: the configuration value `logs_config.socks5_proxy_address` (scrubbed)
  - `config_no_proxy` - **array of strings**: the configuration value `proxy.no_proxy` (scrubbed)
  - `config_process_dd_url` - **string**: the configuration value `process_config.process_dd_url` (scrubbed)
  - `config_proxy_http` - **string**: the configuration value `proxy.http` (scrubbed)
  - `config_proxy_https` - **string**: the configuration value `proxy.https` (scrubbed)
  - `config_eks_fargate` - **bool**: the configuration value `eks_fargate`
  - `eks_fargate_cluster_name` - **string**: the configuration value `cluster_name`. Only set when `eks_fargate` is true.
  - `diagnostics` - **dict of string to JSON type**: the result of series of connectivity checks performed on the agent.
    Not sent when `inventories_diagnostics_enabled` is false.
    - `connectivity` - **array of objects**: one entry per connectivity check:
      - `status` - **string**: `success` or `failure`.
      - `description` - **string**: what was checked.
      - `error` - **string** (omitted if empty): the error raised by the check.
      - `metadata` - **dict of string to string** (omitted if empty): extra information about the check (ex: the
        endpoint).
  - `enc_info` - **dict**: the paths of secret settings, their handles, scrubbed leafs and the hash of the resolved
    secrets (scrubbed). Omitted when `metadata_skip_secrets` is true.
    - `paths` - **array of strings**: the paths of settings that contain secrets
    - `handles` - **array of strings**: the handles that correspond to these paths
    - `scrubbed` - **array of strings**: the keys of non-default settings whose values were scrubbed, either because the key is known to be sensitive (ex: `snmp_traps.users.0.authKey`) or because the value looks like a credential (ex: a raw API key). Keys already covered by `paths` are omitted
    - `hash_secret_id` - **dict of string to string**: for each handle with a resolved value, the hex encoded SHA-256
      hash of the secret value. The hashes are scrubbed from flares and from the `show-metadata` output.
  - `install_method_tool` - **string**: the name of the tool used to install the agent (ie, Chef, Ansible, ...).
  - `install_method_tool_version` - **string**: the tool version used to install the agent (ie: Chef version, Ansible
    version, ...). This defaults to `"undefined"` when not installed through a tool (like when installed with apt, source
    build, ...).
  - `install_method_installer_version` - **string**:  The version of Datadog module (ex: the Chef Datadog package, the Datadog Ansible playbook, ...).
  - `logs_transport` - **string**:  The transport used to send logs to Datadog. Value is either `"HTTP"` or `"TCP"` when logs collection is
    enabled, otherwise the field is omitted.
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
  - `feature_network_path_remote_config_enabled` - **bool**: True if Remote Config is enabled for Network Path (see:
    `network_path.remote_config.enabled` config option).
  - `feature_network_path_connections_monitoring_enabled` - **bool**: True if connections monitoring is enabled for Network Path (see:
    `network_path.connections_monitoring.enabled` config option).
  - `feature_traceroute_enabled` - **bool**: True if the Traceroute module is enabled in the System Probe (see: `traceroute.enabled` config option in `system-probe.yaml`).
  - `feature_oom_kill_enabled` - **bool**: True if the OOM Kill check is enabled for System Probe (see: `system_probe_config.enable_oom_kill` config option in `system-probe.yaml`).
  - `feature_synthetics_collector_enabled` - **bool**: True if Synthetics Test module is enabled.
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
  - `feature_networks_http_enabled` - **bool**: True if HTTP monitoring is enabled for Universal Service Monitoring (see: `service_monitoring_config.http.enabled` config option in `system-probe.yaml`).
  - `feature_networks_https_enabled` - **bool**: True if HTTPS monitoring is enabled for Universal Service Monitoring (see: `service_monitoring_config.tls.native.enabled` config option in `system-probe.yaml`).
  - `feature_remote_configuration_enabled` - **bool**: True if Remote Configuration is enabled (see: `remote_configuration.enabled` config option).
  - `feature_remote_updates_enabled` - **bool**: True if Remote Updates is enabled (see: `remote_updates` config option).
  - `feature_infra_enabled` - **bool**: True if infrastructure metrics (series) payloads are enabled (see: `enable_payloads.series` config option).
  - `feature_autoscaling_cluster_enabled` - **bool**: True if Kubernetes cluster autoscaling is enabled (see: `autoscaling.cluster.enabled` config option).
  - `feature_usm_enabled` - **bool**: True if Universal Service Monitoring is enabled (see: `service_monitoring_config.enabled` config option in `system-probe.yaml`)
  - `feature_usm_http2_enabled` - **bool**: True if HTTP2 monitoring is enabled for Universal Service Monitoring (see: `service_monitoring_config.http2.enabled` config option in `system-probe.yaml`).
  - `feature_usm_kafka_enabled` - **bool**: True if Kafka monitoring is enabled for Universal Service Monitoring (see: `service_monitoring_config.kafka.enabled` config option in `system-probe.yaml`)
  - `feature_usm_postgres_enabled` - **bool**: True if Postgres monitoring is enabled for Universal Service Monitoring (see: `service_monitoring_config.postgres.enabled` config option in `system-probe.yaml`)
  - `feature_usm_redis_enabled` - **bool**: True if Redis monitoring is enabled for Universal Service Monitoring (see: `service_monitoring_config.redis.enabled` config option in `system-probe.yaml`)
  - `feature_usm_go_tls_enabled` - **bool**: True if HTTPS monitoring through GoTLS is enabled for Universal Service Monitoring (see: `service_monitoring_config.tls.go.enabled` config option in `system-probe.yaml`).
  - `feature_discovery_enabled` - **bool**: True if discovery module is enabled (see: `discovery.enabled` config option).
  - `feature_discovery_service_map_enabled` - **bool**: True if the discovery service map mode is enabled (see:
    `discovery.service_map.enabled` config option in `system-probe.yaml`). When true, `feature_usm_enabled`,
    `feature_networks_http_enabled`, `feature_networks_https_enabled`, `feature_usm_istio_enabled` and
    `feature_usm_go_tls_enabled` are reported as false.
  - `feature_gpu_monitoring_enabled` - **bool**: True if GPU monitoring is enabled (see: `gpu.enabled` config option in
    `datadog.yaml` or `gpu_monitoring.enabled` config option in `system-probe.yaml`).
  - `feature_dynamic_instrumentation_enabled` - **bool**: True if dynamic instrumentation module is enabled (see: `dynamic_instrumentation.enabled` config option).
  - `feature_logs_enabled` - **bool**: True if the logs collection is enabled (see: `logs_enabled` config option).
  - `feature_private_action_runner_enabled` - **bool**: True if the Private Action Runner is enabled in split deployment mode (see: `private_action_runner.enabled` and `private_action_runner.split_enabled` config options).
  - `feature_cspm_enabled` - **bool**: True if the Cloud Security Posture Management is enabled (see:
    `compliance_config.enabled` config option).
  - `feature_cspm_host_benchmarks_enabled` - **bool**: True if host benchmarks are enabled (see:
    `compliance_config.enabled` and `compliance_config.host_benchmarks.enabled` config options).
  - `feature_apm_enabled` - **bool**: True if the APM Agent is enabled (see: `apm_config.enabled` config option).
  - `feature_otlp_enabled` - **bool**: True if the OTLP pipeline is enabled. Only set when the OTLP collector
    component runs.
  - `feature_imdsv2_enabled` - **bool**: True if the IMDSv2 is enabled (see: `ec2_prefer_imdsv2` config option).
  - `feature_container_images_enabled` - **bool**: True if Container Images is enabled (see: `container_image.enabled` config option).
  - `feature_csm_vm_containers_enabled` - **bool**: True if VM Containers is enabled for Cloud Security Management (see: `sbom.enabled`, `container_image.enabled` and `sbom.container_image.enabled` config options).
  - `feature_csm_vm_hosts_enabled` - **bool**: True if VM Hosts is enabled for Cloud Security Management (see: `sbom.enabled` and `sbom.host.enabled` config options).
  - `feature_cws_network_enabled` - **bool**: True if Network Monitoring is enabled for Cloud Workload Security (see: `event_monitoring_config.network.enabled` config option).
  - `feature_cws_remote_config_enabled` - **bool**: True if Remote Config is enabled for Cloud Workload Security (see: `runtime_security_config.remote_configuration.enabled` config option).
  - `feature_cws_security_profiles_enabled` - **bool**: True if Security Profiles is enabled for Cloud Workload Security (see: `runtime_security_config.activity_dump.enabled` config option).
  - `feature_usm_istio_enabled` - **bool**: True if Istio is enabled for Universal Service Monitoring (see: `service_monitoring_config.tls.istio.enabled` config option).
  - `feature_windows_crash_detection_enabled` - **bool**: True if Windows Crash Detection is enabled (see: `windows_crash_detection.enabled` config option).
  - `feature_auto_instrumentation_enabled` - **bool**: True if APM Auto-Instrumentation is installed and enabled.
  - `full_configuration` - **string**: the current Agent configuration scrubbed, including all the defaults, as a YAML
    string.
  - `provided_configuration` - **string**: the current Agent configuration (scrubbed), without the defaults, as a YAML
    string. This includes the settings configured by the user (throuh the configuration file, the environment or CLI),
    as well as any settings explicitly set by the agent (for example the number of workers is dynamically set by the
    agent itself based on the load).
  - `file_configuration` - **string**: the Agent configuration specified by the configuration file (scrubbed), as a YAML string.
    Only the settings written in the configuration file are included, and their value might not match what's applied by the agent because they can be overridden by other sources.
  - `environment_variable_configuration` - **string**: the Agent configuration specified by the environment variables (scrubbed), as a YAML string.
    Only the settings written in the environment variables are included, and their value might not match what's applied by the agent because they can be overridden by other sources.
  - `agent_runtime_configuration` - **string**: the Agent configuration set by the agent itself (scrubbed), as a YAML string.
    Only the settings set by the agent itself are included, and their value might not match what's applied by the agent because they can be overridden by other sources.
  - `remote_configuration` - **string**: the Agent configuration specified by the Remote Configuration (scrubbed), as a YAML string.
    Only the settings currently used by Remote Configuration are included, and their value might not match what's applied by the agent because they can be overridden by other sources.
  - `fleet_policies_configuration` - **string**: the Agent configuration specified by the Fleet Automation Policies (scrubbed), as a YAML string.
    Only the settings currently used by Fleet Automation Policies are included, and their value might not match what's applied by the agent since they can be overridden by other sources.
  - `cli_configuration` - **string**: the Agent configuration specified by the CLI (scrubbed), as a YAML string.
    Only the settings set in the CLI are included, they cannot be overridden by any other sources.
  - `source_local_configuration` - **string**: the Agent configuration synchronized from the local Agent process, as a YAML string.
  - `ecs_fargate_task_arn` - **string**: if the Agent runs in ECS Fargate, contains the Agent's Task ARN. Else, is empty.
  - `ecs_fargate_cluster_name` - **string**: if the Agent runs in ECS Fargate, contains the Agent's cluster name. Else, is empty.
  - `fleet_policies_applied` -- **array of string**: The Fleet Policies that have been applied to the agent, if any. Is empty if no policy is applied.
  - `config_id` -- **string**: the Fleet Config ID, the configuration value `config_id`.
  - `auto_instrumentation_modes` -- **array of string**: The injection types enabled for APM Auto-Instrumentation.
  - `infrastructure_mode` -- **string**: The monitoring mode the agent is configured in, each mode offers different
    amount of feature (default is `full`; other values include `end_user_device`, `basic`, `cloud_cost_only`, and `none`).
  - `application_monitoring_config` -- **string**: the scrubbed content of `application_monitoring.yaml`. The value is
    stored as bytes, so it is currently sent as a base64-encoded YAML string. `null` when the file doesn't exist or is
    empty.
  - `application_monitoring_config_fleet` -- **string**: same as `application_monitoring_config` for the Fleet
    Automation managed `managed/datadog-agent/stable/application_monitoring.yaml` file (base64-encoded YAML string,
    `null` when the file doesn't exist or is empty).

("scrubbed" indicates that secrets are removed from the field value just as they are in logs)

## Example Payload

Here an example of an inventory payload (`application_monitoring_config` is the base64 encoding of
`apm_configuration_default:\n  DD_PROFILING_ENABLED: true\n`):

```json
{
    "hostname": "my-host",
    "uuid": "00000000-0000-0000-0000-000000000001",
    "timestamp": 1763650000507358895,
    "agent_metadata": {
        "hostname_source": "os",
        "agent_version": "7.75.0",
        "package_version": "7.75.0-1",
        "agent_startup_time_ms": 1763650000123,
        "flavor": "agent",
        "fips_mode": false,
        "config_apm_dd_url": "",
        "config_dd_url": "",
        "config_site": "datadoghq.com",
        "config_logs_dd_url": "",
        "config_logs_socks5_proxy_address": "",
        "config_no_proxy": [
            "http://some-no-proxy"
        ],
        "config_process_dd_url": "",
        "config_proxy_http": "",
        "config_proxy_https": "http://localhost:9999",
        "config_eks_fargate": true,
        "eks_fargate_cluster_name": "my-eks-cluster",
        "diagnostics": {
            "connectivity": [
                {
                    "status": "success",
                    "description": "Ping to Datadog intake",
                    "metadata": {
                        "endpoint": "https://intake.profile.datadoghq.com/api/v2/profile"
                    }
                },
                {
                    "status": "failure",
                    "description": "Ping to Datadog intake",
                    "error": "context deadline exceeded",
                    "metadata": {
                        "endpoint": "https://process.datadoghq.com/api/v1/collector"
                    }
                }
            ]
        },
        "enc_info": {
            "paths": [
                "api_key",
                "password"
            ],
            "handles": [
                "name",
                "pswd"
            ],
            "scrubbed": [
                "client_secret"
            ],
            "hash_secret_id": {
                "name": "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
                "pswd": "60303ae22b998861bce3b28f33eec1be758a213c86c93c076dbe9f558c11c752"
            }
        },
        "install_method_tool": "install_script",
        "install_method_tool_version": "install_script",
        "install_method_installer_version": "install_script-1.40.0",
        "logs_transport": "HTTP",
        "feature_cws_enabled": false,
        "feature_process_enabled": true,
        "feature_process_language_detection_enabled": false,
        "feature_processes_container_enabled": true,
        "feature_networks_enabled": true,
        "feature_network_path_remote_config_enabled": false,
        "feature_network_path_connections_monitoring_enabled": false,
        "feature_traceroute_enabled": false,
        "feature_oom_kill_enabled": false,
        "feature_synthetics_collector_enabled": false,
        "feature_tcp_queue_length_enabled": false,
        "system_probe_telemetry_enabled": false,
        "system_probe_core_enabled": true,
        "system_probe_runtime_compilation_enabled": false,
        "system_probe_kernel_headers_download_enabled": false,
        "system_probe_prebuilt_fallback_enabled": true,
        "system_probe_track_tcp_4_connections": true,
        "system_probe_track_tcp_6_connections": true,
        "system_probe_track_udp_4_connections": true,
        "system_probe_track_udp_6_connections": true,
        "system_probe_protocol_classification_enabled": true,
        "system_probe_gateway_lookup_enabled": true,
        "system_probe_root_namespace_enabled": true,
        "feature_networks_http_enabled": false,
        "feature_networks_https_enabled": false,
        "feature_remote_configuration_enabled": true,
        "feature_remote_updates_enabled": false,
        "feature_infra_enabled": true,
        "feature_autoscaling_cluster_enabled": false,
        "feature_usm_enabled": false,
        "feature_usm_http2_enabled": false,
        "feature_usm_kafka_enabled": false,
        "feature_usm_postgres_enabled": false,
        "feature_usm_redis_enabled": false,
        "feature_usm_go_tls_enabled": false,
        "feature_discovery_enabled": true,
        "feature_discovery_service_map_enabled": false,
        "feature_gpu_monitoring_enabled": false,
        "feature_dynamic_instrumentation_enabled": false,
        "feature_logs_enabled": true,
        "feature_private_action_runner_enabled": false,
        "feature_cspm_enabled": false,
        "feature_cspm_host_benchmarks_enabled": false,
        "feature_apm_enabled": true,
        "feature_otlp_enabled": false,
        "feature_imdsv2_enabled": true,
        "feature_container_images_enabled": true,
        "feature_csm_vm_containers_enabled": false,
        "feature_csm_vm_hosts_enabled": false,
        "feature_cws_network_enabled": false,
        "feature_cws_remote_config_enabled": false,
        "feature_cws_security_profiles_enabled": false,
        "feature_usm_istio_enabled": false,
        "feature_windows_crash_detection_enabled": false,
        "feature_auto_instrumentation_enabled": true,
        "system_probe_max_connections_per_message": 600,
        "full_configuration": "<entire yaml configuration for the agent>",
        "provided_configuration": "api_key: \"***************************aaaaa\"\ncheck_runners: 4\nlog_level: \"warn\"",
        "file_configuration": "check_runners: 4",
        "environment_variable_configuration": "api_key: \"***************************aaaaa\"",
        "agent_runtime_configuration": "runtime_block_profile_rate: 5000",
        "remote_configuration": "log_level: \"debug\"",
        "fleet_policies_configuration": "{}",
        "cli_configuration": "log_level: \"warn\"",
        "source_local_configuration": "{}",
        "ecs_fargate_task_arn": "",
        "ecs_fargate_cluster_name": "",
        "fleet_policies_applied": [
            "my-policy"
        ],
        "config_id": "my-config",
        "auto_instrumentation_modes": [
            "host",
            "docker"
        ],
        "infrastructure_mode": "full",
        "application_monitoring_config": "YXBtX2NvbmZpZ3VyYXRpb25fZGVmYXVsdDoKICBERF9QUk9GSUxJTkdfRU5BQkxFRDogdHJ1ZQo=",
        "application_monitoring_config_fleet": null
    },
    "files_metadata": {
        "/etc/datadog-agent/datadog.yaml": {
            "raw_config": "api_key: '***************************aaaa'\nsite: datadoghq.com\nlogs_config:\n  api_key: ENC[logs_api_key]\n",
            "hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
            "secrets": [
                {
                    "path": "api_key",
                    "kind": "raw"
                },
                {
                    "path": "logs_config.api_key",
                    "kind": "enc",
                    "handle": "logs_api_key"
                }
            ]
        }
    }
}
```

### Secret declarations in configuration files

This metadata is intended to support Fleet Automation for VM-based Agents and only
covers configuration files reported in `files_metadata`. Secrets supplied only
through other configuration sources, such as environment variables or Kubernetes
Autodiscovery annotations, are not included. It is not a complete inventory of
secrets in the effective configuration.

Each `files_metadata[filename].secrets` array describes the secret declarations in
that file. Entries have `kind` (`enc` or `raw`), `path` (a dotted path relative to the file,
including list indexes), and, for ENC declarations, `handle`. For example,
`{"kind":"enc","path":"instances.0.password","handle":"db_password"}`.
The filename and path together identify a location, including when two files
configure the same integration. Values and hashes are never included in these entries.
Raw references identify the field replaced by a scrubbing rule. When a rule
replaces a list or mapping, the reference identifies that whole field (for example,
`snmp_traps.community_strings`), rather than reconstructing its original leaves.
ENC references always retain their original indexed paths and handles, even inside
a replaced container. Repeated handles retain one entry per location. These entries describe file
contents; `agent_metadata.enc_info` continues reporting resolved handles and hashes.
`metadata_skip_secrets` suppresses these entries as well as `enc_info`.
