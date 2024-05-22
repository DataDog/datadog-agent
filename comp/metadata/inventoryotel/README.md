# Inventory Agent Payload

This package populates some of the otel-agent-related fields in the `inventories` product in DataDog. More specifically the
`datadog-otel-agent` table.

This is enabled by default but can be turned off using `inventories_enabled` config.

The payload is sent every 10min (see `inventories_max_interval` in the config) or whenever it's updated with at most 1
update every minute (see `inventories_min_interval`).

# Content

The `Set` method from the component allow the rest of the codebase to add any information to the payload.

## Agent Configuration

The otel-agent configurations are scrubbed from any sensitive information (same logic than for the flare).
This include the following:
`otel_provided_configuration`
`otel_runtime_configuration`

Sending Agent configuration can be disabled using `inventories_configuration_enabled`.

# Format

The payload is a JSON dict with the following fields

- `hostname` - **string**: the hostname of the otel-agent as shown on the status page.
- `uuid` - **string**: a unique identifier of the otel-agent, used in case the hostname is empty.
- `timestamp` - **int**: the timestamp when the payload was created.
- `otel-agent_metadata` - **dict of string to JSON type**:
  - `otel-agent_version` - **string**: the version of the Agent.
  - `install_method_tool` - **string**: the name of the tool used to install the otel-agent (ie, Chef, Ansible, ...).
  - `provided_configuration` - **string**: the current Agent configuration (scrubbed), without the defaults, as a YAML
    string. This includes the settings configured by the user (throuh the configuration file, the environment or CLI),
    as well as any settings explicitly set by the otel-agent (for example the number of workers is dynamically set by the
    otel-agent itself based on the load).
  - `file_configuration` - **string**: the Agent configuration specified by the configuration file (scrubbed), as a YAML string.
    Only the settings written in the configuration file are included, and their value might not match what's applyed by the otel-agent because they can be overriden by other sources.
  - `environment_variable_configuration` - **string**: the Agent configuration specified by the environment variables (scrubbed), as a YAML string.
    Only the settings written in the environment variables are included, and their value might not match what's applyed by the otel-agent because they can be overriden by other sources.
  - `otel-agent_runtime_configuration` - **string**: the Agent configuration set by the otel-agent itself (scrubbed), as a YAML string.
    Only the settings set by the otel-agent itself are included, and their value might not match what's applyed by the otel-agent because they can be overriden by other sources.
  - `remote_configuration` - **string**: the Agent configuration specified by the Remote Configuration (scrubbed), as a YAML string.
    Only the settings currently used by Remote Configuration are included, and their value might not match what's applyed by the otel-agent because they can be overriden by other sources.
  - `cli_configuration` - **string**: the Agent configuration specified by the CLI (scrubbed), as a YAML string.
    Only the settings set in the CLI are included, they cannot be overriden by any other sources.
  - `source_local_configuration` - **string**: the Agent configuration synchronized from the local Agent process, as a YAML string.

("scrubbed" indicates that secrets are removed from the field value just as they are in logs)

## Example Payload

Here an example of an inventory payload:

```
{
    "otel-agent_metadata": {
        "otel-agent_version": "7.37.0-devel+git.198.68a5b69",
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
        "flavor": "otel-agent",
        "hostname_source": "os",
        "install_method_installer_version": "",
        "install_method_tool": "undefined",
        "install_method_tool_version": "",
        "logs_transport": "HTTP",
        "full_configuration": "<entire yaml configuration for the otel-agent>",
        "provided_configuration": "api_key: \"***************************aaaaa\"\ncheck_runners: 4\ncmd.check.fullsketches: false\ncontainerd_namespace: []\ncontainerd_namespaces: []\npython_version: \"3\"\ntracemalloc_debug: false\nlog_level: \"warn\"",
        "file_configuration": "check_runners: 4\ncmd.check.fullsketches: false\ncontainerd_namespace: []\ncontainerd_namespaces: []\npython_version: \"3\"\ntracemalloc_debug: false",
        "environment_variable_configuration": "api_key: \"***************************aaaaa\"",
        "remote_configuration": "log_level: \"debug\"",
        "cli_configuration": "log_level: \"warn\""
    }
    "hostname": "my-host",
    "timestamp": 1631281754507358895
}
```
