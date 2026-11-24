# Security-agent metadata Payload

This package populates some of the security-agent related fields in the `inventories` product in DataDog. More specifically the
`security-agent` table.

This is enabled by default but can be turned off using the `inventories_enabled` or `enable_metadata_collection`
config.

The payload is sent every 10min (see `inventories_max_interval` in the config).

## Security-agent Configuration

The agent configurations are scrubbed from any sensitive information (same logic as for the flare). The `Format`
section goes into more detail about what configuration is sent.

Sending Security-Agent configuration can be disabled using `inventories_configuration_enabled`. In that case, or if
the configuration can't be fetched from the security-agent process, `security_agent_metadata` only contains
`agent_version`. `full_configuration` is also omitted if it alone can't be fetched.

# Format

The payload is a JSON dict with the following fields

- `hostname` - **string**: the hostname of the agent as shown on the status page.
- `timestamp` - **int**: the timestamp when the payload was created.
- `security_agent_metadata` - **dict of string to JSON type**:
  - `agent_version` - **string**: the version of the Agent sending this payload.
  - `full_configuration` - **string**: the current Security-Agent configuration scrubbed, including all the defaults, as a YAML
    string.
  - `provided_configuration` - **string**: the current Security-Agent configuration (scrubbed), without the defaults, as a YAML
    string. This includes the settings configured by the user (through the configuration file, the environment, CLI...).
  - `file_configuration` - **string**: the Security-Agent configuration specified by the configuration file (scrubbed), as a YAML string.
    Only the settings written in the configuration file are included, and their value might not match what's applied by the agent since they can be overridden by other sources.
  - `environment_variable_configuration` - **string**: the Security-Agent configuration specified by the environment variables (scrubbed), as a YAML string.
    Only the settings written in the environment variables are included, and their value might not match what's applied by the agent since they can be overridden by other sources.
  - `agent_runtime_configuration` - **string**: the Security-Agent configuration set by the agent itself (scrubbed), as a YAML string.
    Only the settings set by the agent itself are included, and their value might not match what's applied by the agent since they can be overridden by other sources.
  - `remote_configuration` - **string**: the Security-Agent configuration specified by the Remote Configuration (scrubbed), as a YAML string.
    Only the settings currently used by Remote Configuration are included, and their value might not match what's applied by the agent since they can be overridden by other sources.
  - `fleet_policies_configuration` - **string**: the Security-Agent configuration specified by the Fleet Automation Policies (scrubbed), as a YAML string.
    Only the settings currently used by Fleet Automation Policies are included, and their value might not match what's applied by the agent since they can be overridden by other sources.
  - `cli_configuration` - **string**: the Security-Agent configuration specified by the CLI (scrubbed), as a YAML string.
    Only the settings set in the CLI are included.
  - `source_local_configuration` - **string**: the Security-Agent configuration synchronized from the local Agent process, as a YAML string.

("scrubbed" indicates that secrets are removed from the field value just as they are in logs)

## Example Payload

Here an example of an inventory payload:

```json
{
    "security_agent_metadata": {
        "agent_version": "7.75.0",
        "full_configuration": "api_key: '***************************aaaaa'\ncompliance_config:\n  enabled: true\n  host_benchmarks:\n    enabled: true\nlog_level: info\nruntime_security_config:\n  enabled: true\n  socket: /opt/datadog-agent/run/runtime-security.sock\nsite: datadoghq.com\n",
        "provided_configuration": "compliance_config:\n  enabled: true\nruntime_security_config:\n  enabled: true\n  socket: /opt/datadog-agent/run/runtime-security.sock\n",
        "file_configuration": "compliance_config:\n  enabled: true\nruntime_security_config:\n  enabled: true\n  socket: /opt/datadog-agent/run/runtime-security.sock\n",
        "agent_runtime_configuration": "runtime_block_profile_rate: 5000\n",
        "environment_variable_configuration": "{}\n",
        "remote_configuration": "{}\n",
        "fleet_policies_configuration": "{}\n",
        "cli_configuration": "{}\n",
        "source_local_configuration": "{}\n"
    },
    "hostname": "my-host",
    "timestamp": 1631281754507358895
}
```
