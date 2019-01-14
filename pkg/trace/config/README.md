# Agent Configuration

The trace-agent sources configuration from the following locations:

1. The Datadog Agent configuration file, provided to the `-config` command line flag (default: `/etc/datadog/datadog.conf`)
2. Environment variables: See full list below

Environment variables will override settings defined in configuration files.

## File configuration

Refer to the [Datadog Agent example configuration](https://github.com/DataDog/dd-agent/blob/master/datadog.conf.example) to see all available options.


## Environment variables
We allow overriding a subset of configuration values from the environment. These
can be useful when running the agent in a Docker container or in other situations
where env vars are preferrable to static files

- `DD_APM_ENABLED` - overrides `[Main] apm_enabled`
- `DD_HOSTNAME` - overrides `[Main] hostname`
- `DD_API_KEY` - overrides `[Main] api_key`
- `DD_DOGSTATSD_PORT` - overrides `[Main] dogstatsd_port`
- `DD_BIND_HOST` - overrides `[Main] bind_host`
- `DD_APM_NON_LOCAL_TRAFFIC` - overrides `[Main] non_local_traffic`
- `DD_LOG_LEVEL` - overrides `[Main] log_level`
- `DD_RECEIVER_PORT` - overrides `[trace.receiver] receiver_port`
- `DD_IGNORE_RESOURCE` - overrides `[trace.ignore] resource`
