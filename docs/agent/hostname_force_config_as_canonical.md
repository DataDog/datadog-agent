# Make config-provided hostname canonical

In v6 and v7 Agents, if `hostname` is set in `datadog.yaml` (or through the `DD_HOSTNAME` env var) and its value starts with `ip-` or `domu`, the hostname is not used in-app as the canonical hostname, even though it is a valid hostname.
More information about what a canonical hostname is can be found at [How does Datadog determine the Agent hostname?](https://docs.datadoghq.com/agent/faq/how-datadog-agent-determines-the-hostname/?tab=agentv6v7#agent-versions).

Starting with Agent v6.16.0 and v7.16.0, the Agent supports the config option `hostname_force_config_as_canonical` (default: `false`). When set to `true`, a configuration-provided hostname starting with `ip-` or `domu` will be accepted as the canonical hostname in-app.

Starting with v6.16.0 and v7.16.0, the Agent logs the following warning if it detects a situation where the config-provided hostname is a valid hostname but will not be accepted as the canonical hostname in-app:
`Hostname '<HOSTNAME>' defined in configuration will not be used as the in-app hostname. For more information: https://dtdg.co/agent-hostname-config-as-canonical`

## Impact of enabling hostname_force_config_as_canonical

The hostname remains the same in-app until contacting the support team to runs the command that clears the existing host aliases.
This command let the config-provided hostname be accepted as the canonical hostname in-app.

