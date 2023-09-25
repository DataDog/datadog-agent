# Inventory Payload

This package populates some of the agent-related fields in the `inventories` product in DataDog.

This is enabled by default but can be turned off using `inventories_enabled` config.

The payload is sent every 10min (see `inventories_max_interval` in the config) or whenever it's updated with at most 1
update every 5 minutes (see `inventories_min_interval`).

# Content

The current payload contains 3 sections `check_metadata`, `agent_metadata`, and `host_metadata`.
Those are not guaranteed to be sent in one payload. If the final serialized payload is to big, each section is sent in a
different payload with `hostname` and `timestamp` always present. This is why some field are duplicated between section,
like `agent_version`.

The package offers 3 methods to the agent codebase to add data to the payload: `SetAgentMetadata`, `SetHostMetadata` and
`SetCheckMetadata`.
As the name suggests, checks use `SetCheckMetadata` and each metadata is linked to a check ID. Everything agent-related
uses `SetAgentMetadata` and for host metadata `SetHostMetadata` is used. Any part of the Agent can add metadata to the
inventory payload.

## Agent Configuration

`agent_metadata.full_configuration` and `agent_metadata.provided_configuration` are scrubbed from any sensitive
information (same logic than for the flare).

Sending Agent configuration can be disabled using `inventories_configuration_enabled`.

## Check metadata

`SetCheckMetadata` registers data per check instance. Metadata can include the check version, the version of the
monitored software, ... It depends on each check.

For every running check, no matter if it registered extra metadata or not, we send: name, ID, configuration,
configuration provider. Sending checks configuration can be disabled using `inventories_checks_configuration_enabled`.

## Agent metadata

`SetAgentMetadata` registers data about the agent itself.

## Host metadata

`host_metadata` contains metadata about the host to be displayed in the 'host' table of the inventories product. Some of
its content is duplicated from the V5 metadata payload as it's pulled from `gohai`.

# Format

The payload is a JSON dict with the following fields

- `hostname` - **string**: the hostname of the agent as shown on the status page.
- `timestamp` - **int**: the timestamp when the payload was created.
- `check_metadata` - **dict of string to list**: dictionary with check names as keys; values are a list of the metadata for each
  instance of that check.
  Each instance is composed of:
    - `config.hash` - **string**: the instance ID for this instance (as shown in the status page).
    - `config.provider` - **string**: where the configuration came from for this instance (disk, docker labels, ...).
    - `init_config` - **string**: the `init_config` part of the configuration for this check instance.
    - `instance_config` - **string**: the YAML configuration for this check instance
    - Any other metadata registered by the instance (instance version, version of the software monitored, ...).
<!-- NOTE: when modifying this list, please also update the constants in `inventories.go` -->
- `agent_metadata` - **dict of string to JSON type**:
  - `cloud_provider` - **string**: the name of the cloud provider detected by the Agent (omitted if no cloud is
    detected). Deprecated since `7.38.0`, for now this is duplicated in the `host_metadata` section and will soon be
    remove from `agent_metadata`.
  - `hostname_source` - **string**: the source for the agent hostname (see pkg/util/hostname/providers.go:GetWithProvider).
  - `agent_version` - **string**: the version of the Agent.
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
     (see: `language_detection,enabled` config option).
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
  - `system_probe_prebuilt_fallback_enabled` - **bool**: True if the System Probe will fallback to prebuilt when other options fail (see: `system_probe_config.allow_precompiled_fallback` config option in `system-probe.yaml`).
  - `system_probe_max_connections_per_message` - **int**: The maximum number of connections per message (see: `system_probe_config.max_conns_per_message` config option in `system-probe.yaml`).
  - `system_probe_track_tcp_4_connections` - **bool**: True if tracking TCPv4 connections is enabled in the System Probe (see: `network_config.collect_tcp_v4` config option in `system-probe.yaml`).
  - `system_probe_track_tcp_6_connections` - **bool**: True if tracking TCPv6 connections is enabled in the System Probe (see: `network_config.collect_tcp_v6` config option in `system-probe.yaml`).
  - `system_probe_track_udp_4_connections` - **bool**: True if tracking UDPv4 connections is enabled in the System Probe (see: `network_config.collect_udp_v4` config option in `system-probe.yaml`).
  - `system_probe_track_udp_6_connections` - **bool**: True if tracking UDPv6 connections is enabled in the System Probe (see: `network_config.collect_udp_v6` config option in `system-probe.yaml`).
  - `system_probe_protocol_classification_enabled` - **bool**: True if protocol classification is enabled in the System Probe (see: `network_config.enable_protocol_classification` config option in `system-probe.yaml`).
  - `system_probe_gateway_lookup_enabled` - **bool**: True if gateway lookup is enable in the System Probe (see: `network_config.enable_gateway_lookup` config option in `system-probe.yaml`).
  - `system_probe_root_namespace_enabled` - **bool**: True if the System Probe will run in the root namespace of the host (see: `network_config.enable_root_netns` config option in `system-probe.yaml`).
  - `feature_networks_http_enabled` - **bool**: True if HTTP monitoring is enabled for Network Performance Monitoring (see: `network_config.enable_http_monitoring` config option in `system-probe.yaml`).
  - `feature_networks_https_enabled` - **bool**: True if HTTPS monitoring is enabled for Universal Service Monitoring (see: `service_monitoring_config.tls.native.enabled` config option in `system-probe.yaml`).
  - `feature_remote_configuration_enabled` - **bool**: True if Remote Configuration is enabled (see: `remote_configuration.enabled` config option).
  - `feature_usm_enabled` - **bool**: True if Universal Service Monitoring is enabled (see: `service_monitoring_config.enabled` config option in `system-probe.yaml`)
  - `feature_usm_http2_enabled` - **bool**: True if HTTP2 monitoring is enabled for Universal Service Monitoring (see: `service_monitoring_config.enable_http2_monitoring` config option in `system-probe.yaml`).
  - `feature_usm_kafka_enabled` - **bool**: True if Kafka monitoring is enabled for Universal Service Monitoring (see: `data_streams_config.enabled` config option in `system-probe.yaml`)
  - `feature_usm_java_tls_enabled` - **bool**: True if HTTPS monitoring through java TLS is enabled for Universal Service Monitoring (see: `service_monitoring_config.enable_java_tls_support` config option in `system-probe.yaml`).
  - `feature_usm_go_tls_enabled` - **bool**: True if HTTPS monitoring through GoTLS is enabled for Universal Service Monitoring (see: `service_monitoring_config.enable_go_tls_support` config option in `system-probe.yaml`).
  - `feature_dynamic_instrumentation_enabled` - **bool**: True if dynamic instrumentation module is enabled.
  - `feature_enable_http_stats_by_status_code` - **bool**: True if HTTP stats aggregation should be by status code rather than status code family (see: `service_monitoring_config.enable_http_stats_by_status_code` config option in `system-probe.yaml`).
  - `feature_logs_enabled` - **bool**: True if the logs collection is enabled (see: `logs_enabled` config option).
  - `feature_cspm_enabled` - **bool**: True if the Cloud Security Posture Management is enabled (see:
    `compliance_config.enabled` config option).
  - `feature_apm_enabled` - **bool**: True if the APM Agent is enabled (see: `apm_config.enabled` config option).
  - `feature_otlp_enabled` - **bool**: True if the OTLP pipeline is enabled.
  - `feature_imdsv2_enabled` - **bool**: True if the IMDSv2 is enabled (see: `ec2_prefer_imdsv2` config option).
  - `full_configuration` - **string**: the current Agent configuration scrubbed, including all the defaults, as a YAML
    string.
  - `provided_configuration` - **string**: the current Agent configuration (scrubbed), without the defaults, as a YAML
    string. This includes the settings configured by the user (throuh the configuration file, the environment or CLI),
    as well as any settings explicitly set by the agent (for example the number of workers is dynamically set by the
    agent itself based on the load).
- `host_metadata` - **dict of string to JSON type**:
  - `cpu_cores` - **int**: the number of core for the host.
  - `cpu_logical_processors` - **int**:  the number of logical core for the host.
  - `cpu_vendor` - **string**: the CPU vendor.
  - `cpu_model` - **string**:  the CPU model.
  - `cpu_model_id` - **string**: the CPU model ID.
  - `cpu_family` - **string**: the CPU family.
  - `cpu_stepping` - **string**: the CPU stepping.
  - `cpu_frequency` - **number/float**: the CPU frequency.
  - `cpu_cache_size` - **int**: the CPU cache size in bytes (only fill on Linux only, 0 for Windows and OSX).
  - `cpu_architecture` - **string**: the hardware name, Linux only (ex "x86_64", "unknown", ...).
  - `kernel_name` - **string**: the kernel name (ex: "windows", "Linux", ...).
  - `kernel_release` - **string**:  the kernel release (ex: "10.0.20348", "4.15.0-1080-gcp", ...).
  - `kernel_version` - **string**:  the kernel version (Unix only, empty string on Windows).
  - `os` - **string**: the OS name description (ex: "GNU/Linux", "Windows Server 2022 Datacenter", ...).
  - `os_version` - **string**: the OS version (ex: "debian bookworm/sid", ...).
  - `memory_total_kb` - **int**: the total memory size for the host in KiB.
  - `memory_swap_total_kb` - **int**: the `swap` memory size in KiB (Unix only).
  - `ip_address` - **string**: the IP address for the host.
  - `ipv6_address` - **string**: the IPV6 address for the host.
  - `mac_address` - **string**: the MAC address for the host.
  - `agent_version` - **string**: the version of the Agent that sent this payload.
  - `cloud_provider` - **string**: the name of the cloud provider detected by the Agent.
  - `cloud_provider_source` - **string**: the data source used to know that the Agent is running on `cloud_provider`.
    This is different for each cloud provider. For now ony AWS is supported.
    Values on AWS:
    - `IMDSv2`: The Agent successfully contacted IMDSv2 metadata endpoint.
    - `IMDSv1`: The Agent successfully contacted IMDSv1 metadata endpoint.
    - `DMI`: The Agent successfully used DMI information to fetch the instance ID (only works on Unix EC2 Nitro instances).
    - `UUID`: The hypervisor or product UUID has the EC2 prefix. The Agent knows it's running on EC2 but don't know on
      which instance (see `hypervisor_guest_uuid` or `dmi_product_uuid`).
  - `cloud_provider_account_id` - **string**: The account/subscription ID from the cloud provider.
  - `cloud_provider_host_id` - **string**: the unique ID the cloud provider uses to reference this instance.
    This is different for each cloud provider (for now, ony AWS is supported).
    - On AWS: the instance ID returned by querying the IMDSv2 endpoint. An empty string is returned if we can't reach
      IMDSv2 (even if IMDSv1 is available).
  - `hypervisor_guest_uuid` - **string**: the hypervisor guest UUID (Unix only, empty string on Windows or if we can't
    read the data). On `ec2` instance this might start by "ec2". This was introduce in `7.41.0`/`6.41.0`.
  - `dmi_product_uuid` - **string**: the DMI product UUID (Unix only, empty string on Windows or if we can't read the
    data). On `ec2` instances this might start by "ec2". This was introduce in `7.41.0`/`6.41.0`.
  - `dmi_board_asset_tag` - **string**: the DMI board tag (Unix only, empty string on Windows or if we can't read the
    data). On `ec2` Nitro instance this contains the EC2 instance ID. This was introduce in `7.41.0`/`6.41.0`.
  - `dmi_board_vendor` - **string**: the DMI board vendor (Unix only, empty string on Windows or if we can't read the
    data). On `ec2` Nitro instance this might equal to "Amazon EC2". This was introduce in `7.41.0`/`6.41.0`.

("scrubbed" indicates that secrets are removed from the field value just as they are in logs)

## Example Payload

Here an example of an inventory payload:

```
{
    "agent_metadata": {
        "agent_version": "7.37.0-devel+git.198.68a5b69",
        "cloud_provider": "AWS",
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
        "provided_configuration": "api_key: \"***************************aaaaa\"\ncheck_runners: 4\ncmd.check.fullsketches: false\ncontainerd_namespace: []\ncontainerd_namespaces: []\npython_version: \"3\"\ntracemalloc_debug: false"
    }
    "check_metadata": {
        "cpu": [
            {
                "config.hash": "cpu",
                "config.provider": "file",
                "init_config": "",
                "instance_config: {}
            }
        ],
        "disk": [
            {
                "config.hash": "disk:e5dffb8bef24336f",
                "config.provider": "file",
                "init_config": "",
                "instance_config: {}
            }
        ],
        "file_handle": [
            {
                "config.hash": "file_handle",
                "config.provider": "file",
                "init_config": "",
                "instance_config: {}
            }
        ],
        "io": [
            {
                "config.hash": "io",
                "config.provider": "file",
                "init_config": "",
                "instance_config: {}
            }
        ],
        "load": [
            {
                "config.hash": "load",
                "config.provider": "file",
                "init_config": "",
                "instance_config: {}
            }
        ],
        "redisdb": [
            {
                "config.hash": "redisdb:6e5e79e5b724c83a",
                "config.provider": "container",
                "init_config": "test: 21",
                "instance_config": "host: localhost\nport: 6379\ntags:\n- docker_image:redis\n- image_name:redis\n- short_image:redis",
                "version.major": "7",
                "version.minor": "0",
                "version.patch": "2",
                "version.raw": "7.0.2",
                "version.scheme": "semver"
            },
            {
                "config.hash": "redisdb:c776ecdbdded09b8",
                "config.provider": "container",
                "init_config": "test: 21",
                "instance_config": "host: localhost\nport: 7379\ntags:\n- docker_image:redis\n- image_name:redis\n- short_image:redis",
                "version.major": "7",
                "version.minor": "0",
                "version.patch": "2",
                "version.raw": "7.0.2",
                "version.scheme": "semver"
            }
        ]
    },
    "host_metadata": {
        "cpu_architecture": "unknown",
        "cpu_cache_size": 9437184,
        "cpu_cores": 6,
        "cpu_family": "6",
        "cpu_frequency": 2208.007,
        "cpu_logical_processors": 6,
        "cpu_model": "Intel(R) Core(TM) i7-8750H CPU @ 2.20GHz",
        "cpu_model_id": "158",
        "cpu_stepping": "10",
        "cpu_vendor": "GenuineIntel",
        "kernel_name": "Linux",
        "kernel_release": "5.16.0-6-amd64",
        "kernel_version": "#1 SMP PREEMPT Debian 5.16.18-1 (2022-03-29)",
        "os": "GNU/Linux",
        "os_version": "debian bookworm/sid",
        "memory_swap_total_kb": 10237948,
        "memory_total_kb": 12227556,
        "ip_address": "192.168.24.138",
        "ipv6_address": "fe80::1ff:fe23:4567:890a",
        "mac_address": "01:23:45:67:89:AB",
        "agent_version": "7.37.0-devel+git.198.68a5b69",
        "cloud_provider": "AWS",
        "cloud_provider_source": "DMI",
        "cloud_provider_account_id": "aws_account_id",
        "cloud_provider_host_id": "i-abcedf",
        "hypervisor_guest_uuid": "ec24ce06-9ac4-42df-9c10-14772aeb06d7",
        "dmi_product_uuid": "ec24ce06-9ac4-42df-9c10-14772aeb06d7",
        "dmi_board_asset_tag": "i-abcedf",
        "dmi_board_vendor": "Amazon EC2"
    },
    "hostname": "my-host",
    "timestamp": 1631281754507358895
}
```
