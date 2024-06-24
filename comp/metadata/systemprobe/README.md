# System-probe metadata Payload

This package populates some of the system-probe related fields in the `inventories` product in DataDog. More specifically the
`system-probe` table.

This is enabled by default but can be turned off using `inventories_enabled` config.

The payload is sent every 10min (see `inventories_max_interval` in the config).

## System-probe Configuration

The agent configurations are scrubbed from any sensitive information (same logic than for the flare). The `Format`
section goes into more default about what configuration is sent.

Sending System-Probe configuration can be disabled using `inventories_configuration_enabled`.

# Format

The payload is a JSON dict with the following fields

- `hostname` - **string**: the hostname of the agent as shown on the status page.
- `timestamp` - **int**: the timestamp when the payload was created.
- `system_probe_metadata` - **dict of string to JSON type**:
  - `agent_version` - **string**: the version of the Agent sending this payload.
  - `full_configuration` - **string**: the current System-Probe configuration scrubbed, including all the defaults, as a YAML
    string.
  - `provided_configuration` - **string**: the current System-Probe configuration (scrubbed), without the defaults, as a YAML
    string. This includes the settings configured by the user (throuh the configuration file, the environment, CLI...).
  - `file_configuration` - **string**: the System-Probe configuration specified by the configuration file (scrubbed), as a YAML string.
    Only the settings written in the configuration file are included, and their value might not match what's applyed by the agent since they can be overriden by other sources.
  - `environment_variable_configuration` - **string**: the System-Probe configuration specified by the environment variables (scrubbed), as a YAML string.
    Only the settings written in the environment variables are included, and their value might not match what's applyed by the agent somce they can be overriden by other sources.
  - `agent_runtime_configuration` - **string**: the System-Probe configuration set by the agent itself (scrubbed), as a YAML string.
    Only the settings set by the agent itself are included, and their value might not match what's applyed by the agent since they can be overriden by other sources.
  - `remote_configuration` - **string**: the System-Probe configuration specified by the Remote Configuration (scrubbed), as a YAML string.
    Only the settings currently used by Remote Configuration are included, and their value might not match what's applyed by the agent since they can be overriden by other sources.
  - `cli_configuration` - **string**: the System-Probe configuration specified by the CLI (scrubbed), as a YAML string.
    Only the settings set in the CLI are included.
  - `source_local_configuration` - **string**: the System-Probe configuration synchronized from the local Agent process, as a YAML string.

("scrubbed" indicates that secrets are removed from the field value just as they are in logs)

## Example Payload

Here an example of an inventory payload:

```
{
    "system_probe_metadata": {
        "agent_version": "7.55.0",
        "full_configuration": "<entire yaml configuration for system-probe>",
        "provided_configuration": "system_probe_config:\n  sysprobe_socket: /tmp/sysprobe.sock",
        "file_configuration": "system_probe_config:\n  sysprobe_socket: /tmp/sysprobe.sock",
        "agent_runtime_configuration": "runtime_block_profile_rate: 5000",
        "environment_variable_configuration": "{}",
        "remote_configuration": "{}",
        "cli_configuration": "{}",
        "source_local_configuration": "{}"
    }
    "hostname": "my-host",
    "timestamp": 1631281754507358895
}
```
