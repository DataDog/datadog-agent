# Windows Installation Guide

Welcome to the Datadog IoT installer for Windows.

Installation instructions are available on the dedicated [docs website page](https://docs.datadoghq.com/agent/basic_agent_usage/windows/?tab=agentv6#installation)

## Additional Configuration Options

Most command line options are documented on the [docs website](https://docs.datadoghq.com/agent/basic_agent_usage/windows/?tab=agentv6#command-line). For completeness, the options that should only be used upon request from Datadog Support are listed below:
* DD_URL="_string_"
  * Sets the **dd_url** variable in datadog.yaml to _string_.
* LOGS_DD_URL="_string_"
  * Sets the **logs_dd_url** variable in the **logs_config** section in datadog.yaml to _string_ . _string_ has to be of the form `<endpoint>:<format>`, for example `agent-intake.logs.datadoghq.com:443`.
* PROCESS_DD_URL="_string_"
  * Sets the **process_dd_url** variable in the **process_config** section in datadog.yaml to _string_.
* TRACE_DD_URL="_string_"
  * Sets the **apm_dd_url** variable in the **apm_config** section in datadog.yaml to _string_.
