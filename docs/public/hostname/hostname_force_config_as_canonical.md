# Config-provided hostname starting with `ip-` or `domu`

## Description of the issue

In v6 and v7 Agents, if `hostname` is set in `datadog.yaml` (or through the `DD_HOSTNAME` env var) and its value starts with `ip-` or `domu`, the hostname is not used in-app as the canonical hostname, even if it is a valid hostname.
More information about what a canonical hostname is can be found at [How does Datadog determine the Agent hostname?](https://docs.datadoghq.com/agent/faq/how-datadog-agent-determines-the-hostname/?tab=agentv6v7#agent-versions).

To know if your Agents are affected, starting with v6.16.0 and v7.16.0, the Agent logs the following warning if it detects a situation where the config-provided hostname is a valid hostname but will not be accepted as the canonical hostname in-app:
`Hostname '<HOSTNAME>' defined in configuration are not used as the in-app hostname. For more information: https://dtdg.co/agent-hostname-force-config-as-canonical`

If this warning is logged, you have the following options:

- If you are satisfied with the in-app hostname: unset the configured `hostname` from `datadog.yaml` (or the `DD_HOSTNAME` env var) and restart the Agent; or
- If you are not satisfied with the in-app hostname, and want the configured hostname to appear as the in-app hostname, follow the instructions below

## Allowing Agent in-app hostnames to start with `ip-` or `domu`

Starting with Agent v6.16.0 and v7.16.0, the Agent supports the config option `hostname_force_config_as_canonical` (default: `false`). When set to `true`, a configuration-provided hostname starting with `ip-` or `domu` is accepted as the canonical hostname in-app:

- For new hosts, enabling this option works immediately.
- For hosts that already report to Datadog, after enabling this option, contact Datadog support at support@datadoghq.com so that the in-app hostname can be changed to your configuration-provided hostname.
