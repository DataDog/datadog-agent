# Inventory Payload

This package populates some of the agent-related fields in the `inventories` product in DataDog.

This is enabled by default but can be turned off using `inventories_enabled` config.

The payload is sent every 10min (see `inventories_max_interval` in the config) or whenever it's updated with at most 1
update every 5 minutes (see `inventories_min_interval`).

# Content

The package offers 3 methods to add data to the payload: `SetAgentMetadata`, `SetHostMetadata` and `SetCheckMetadata`.
As the name suggests, checks use `SetCheckMetadata` and each metadata is linked to a check ID. Everything agent-related
uses `SetAgentMetadata` and for host metadata `SetHostMetadata` is used.

Any part of the agent can add metadata to the inventory payload.

The current payload contains 3 sections `check_metadata`, `agent_metadata` and `host_metadata`. Those are not guaranteed
to be sent in one payload. For now they are but each will be sent in it's own payload at some point. The `hostname` and
`timestamp` field will always be present. This is why some field are duplicated like `agent_version`.

## Check metadata

`SetCheckMetadata` registers data per check instance. Metadata can include the check version, the version of the
monitored software, ... It depends on each check.

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
    - `last_updated` - **int**: timestamp of the last metadata update for this instance
    - `config.hash` - **string**: the instance ID for this instance (as shown in the status page).
    - `config.provider` - **string**: where the configuration came from for this instance (disk, docker labels, ...).
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
  - `feature_cws_enabled` - **bool**: True if the Cloud Workload Security is enabled (see: `runtime_security_config.enabled`
    config option).
  - `feature_process_enabled` - **bool**: True if the Process Agent has process collection enabled
     (see: `process_config.process_collection.enabled` config option).
  - `feature_processes_container_enabled` - **bool**: True if the Process Agent has container collection enabled
     (see: `process_config.container_collection.enabled`)
  - `feature_networks_enabled` - **bool**: True if the Network Performance Monitoring is enabled (see:
    `network_config.enabled` config option in `system-probe.yaml`).
  - `feature_networks_http_enabled` - **bool**: True if HTTP monitoring is enabled for Network Performance Monitoring (see: `network_config.enable_http_monitoring` config option in `system-proble.yaml`).
  - `feature_networks_https_enabled` - **bool**: True if HTTPS monitoring is enabled for Network Performance Monitoring (see: `network_config.enable_https_monitoring` config option in `system-proble.yaml`).
  - `feature_logs_enabled` - **bool**: True if the logs collection is enabled (see: `logs_enabled` config option).
  - `feature_cspm_enabled` - **bool**: True if the Cloud Security Posture Management is enabled (see:
    `compliance_config.enabled` config option).
  - `feature_apm_enabled` - **bool**: True if the APM Agent is enabled (see: `apm_config.enabled` config option).
  - `feature_otlp_enabled` - **bool**: True if the OTLP pipeline is enabled.
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
  - `python_version` - **string**: The Python version from the agent environment. `python -V` is used in `Gohai` for
    this. Unless the Agent environment has been modified this is the Python version from the OS not from the Agent. This
    is a relica from Agent V5 and is not useful for Agent V6 and V7 who ship their own Python.
  - `memory_total_kb` - **int**: the total memory size for the host in KiB.
  - `memory_swap_total_kb` - **int**: the `swap` memory size in KiB (Unix only).
  - `ip_address` - **string**: the IP address for the host.
  - `ipv6_address` - **string**: the IPV6 address for the host.
  - `mac_address` - **string**: the MAC address for the host.
  - `agent_version` - **string**: the version of the Agent that sent this payload.
  - `cloud_provider` - **string**: the name of the cloud provider detected by the Agent.

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
        "feature_apm_enabled": true,
        "feature_cspm_enabled": false,
        "feature_cws_enabled": false,
        "feature_logs_enabled": true,
        "feature_networks_enabled": false,
        "feature_process_enabled": false,
        "flavor": "agent",
        "hostname_source": "os",
        "install_method_installer_version": "",
        "install_method_tool": "undefined",
        "install_method_tool_version": "",
        "logs_transport": "HTTP",
    }
    "check_metadata": {
        "cpu": [
           {
               "config.hash": "cpu",
               "config.provider": "file",
               "last_updated": 1631281744506400319
           }
        ],
        "disk": [
            {
                "config.hash": "disk:e5dffb8bef24336f",
                "config.provider": "file",
                "last_updated": 1631281744506400319
            }
        ],
        "file_handle": [
            {
                "config.hash": "file_handle",
                "config.provider": "file",
                "last_updated": 1631281744506400319
            }
        ],
        "io": [
            {
                "config.hash": "io",
                "config.provider": "file",
                "last_updated": 1631281744506400319
            }
        ],
        "load": [
            {
                "config.hash": "load",
                "config.provider": "file",
                "last_updated": 1631281744506400319
            }
        ],
        "memory": [
            {
                "config.hash": "memory",
                "config.provider": "file",
                "last_updated": 1631281744506400319
            }
        ],
        "network": [
            {
                "config.hash": "network:d884b5186b651429",
                "config.provider": "file",
                "last_updated": 1631281744506400319
            }
        ],
        "ntp": [
            {
                "config.hash": "ntp:d884b5186b651429",
                "config.provider": "file",
                "last_updated": 1631281744506400319
            }
        ],
        "uptime": [
            {
                "config.hash": "uptime",
                "config.provider": "file",
                "last_updated": 1631281744506400319
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
        "python_version": "3.10.4",
        "memory_swap_total_kb": 10237948,
        "memory_total_kb": 12227556,
        "ip_address": "192.168.24.138",
        "ipv6_address": "fe80::1ff:fe23:4567:890a",
        "mac_address": "01:23:45:67:89:AB",
        "agent_version": "7.37.0-devel+git.198.68a5b69",
        "cloud_provider": "AWS"
    },
    "hostname": "my-host",
    "timestamp": 1631281754507358895
}
```
