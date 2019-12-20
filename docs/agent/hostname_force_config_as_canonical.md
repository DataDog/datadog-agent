## Make config-provided hostname canonical

In v6 and v7 Agents, if `hostname` is set in `datadog.yaml` (or through the `DD_HOSTNAME` env var) and its value starts with `ip-` or `domu`, the hostname is not used in-app as the canonical hostname, even though it is a valid hostname.
More information about what a canonical hostname is can be found at [How does Datadog determine the Agent hostname?](https://docs.datadoghq.com/agent/faq/how-datadog-agent-determines-the-hostname/?tab=agentv6v7#agent-versions).

Starting with Agent v6.16.0 and v7.16.0, the Agent supports the config option `hostname_force_config_as_canonical` (default: `false`). When set to `true`, a configuration-provided hostname starting with `ip-` or `domu` will be accepted as the canonical hostname in-app.
It creates a new canonical hostname which receives new metrics. The old canonical hostname contains old metrics. 
You may contact the support team if you want to have the old metrics in the new canonical hostname.
