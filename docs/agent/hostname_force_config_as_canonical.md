Agent hostname config as canonical 

If `hostname` is set in `datadog.yaml` and the value starts with `ip-` or `domu`, this hostname is not used as a canonical hostname.
More information about what a canonical hostname is can be found at [How does Datadog determine the Agent hostname?](https://docs.datadoghq.com/agent/faq/how-datadog-agent-determines-the-hostname/?tab=agentv6v7#agent-versions).

To use an hostname starting with `ip-` or `domu` as a canonical hostname, set the config `hostname_force_config_as_canonical` at `true` in datadog.yaml.
It creates a new canonical hostname which receives new metrics. The old canonical hostname contains old metrics. 
You may contact the support team if you want to have the old metrics in the new canonical hostname.