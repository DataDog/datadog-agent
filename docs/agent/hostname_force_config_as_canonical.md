## Make config-provided hostname canonical

In v6 and v7 Agents, if `hostname` is set in `datadog.yaml` (or through the `DD_HOSTNAME` env var) and its value starts with `ip-` or `domu`, the hostname is not used in-app as the canonical hostname, even though it is a valid hostname.
More information about what a canonical hostname is can be found at [How does Datadog determine the Agent hostname?](https://docs.datadoghq.com/agent/faq/how-datadog-agent-determines-the-hostname/?tab=agentv6v7#agent-versions).

To use an hostname starting with `ip-` or `domu` as a canonical hostname, set the config `hostname_force_config_as_canonical` at `true` in datadog.yaml.
It creates a new canonical hostname which receives new metrics. The old canonical hostname contains old metrics. 
You may contact the support team if you want to have the old metrics in the new canonical hostname.
