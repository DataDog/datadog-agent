# Inventory Payload

This package populates some of the agent-related fields in the `inventories` product in DataDog.

This is enabled by default but can be turned off using `inventories_enabled` config.

The payload is sent every 10min (see `inventories_max_interval` in the config) or whenever it's updated with at most 1
update every 5 minutes (see `inventories_min_interval`).

# Content

The package offers 2 method to add data to the payload: `SetAgentMetadata` and `SetCheckMetadata`. As the name suggests,
checks use `SetCheckMetadata` and each metadata is linked to a check ID. Everything agent-related uses the other one.

Any part of the agent can add metadata to the inventory payload.

## Check metadata

`SetCheckMetadata` registers data per check instance. Metadata can include the check version, the version of the
monitored software, ... It depends on each check.

## Agent metadata

`SetAgentMetadata` registers data about the agent itself.

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
  - `cloud_provider` - **string**: the name of the cloud provider detected by the Agent (omitted if no cloud is detected).
  - `hostname_source` - **string**: the source for the agent hostname (see pkg/util/hostname.go:GetHostnameData).
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
  - `feature_process_enabled` - **bool**: True if the Process Agent is enabled (see: `process_config.enabled` config
    option).
  - `feature_networks_enabled` - **bool**: True if the Network Performance Monitoring is enabled (see:
    `network_config.enabled` config option in `system-probe.yaml`).
  - `feature_logs_enabled` - **bool**: True if the logs collection is enabled (see: `logs_enabled` config option).
  - `feature_cspm_enabled` - **bool**: True if the Cloud Security Posture Management is enabled (see:
    `compliance_config.enabled` config option).
  - `feature_apm_enabled` - **bool**: True if the APM Agent is enabled (see: `apm_config.enabled` config option).

("scrubbed" indicates that secrets are removed from the field value just as they are in logs)

## Example Payload

Here an example of an inventory payload:

```
{
   "agent_metadata": {
      "agent_version": "7.32.0-devel+git.146.7bd17a1",
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
    "hostname": "my-host",
    "timestamp": 1631281754507358895
}
```
