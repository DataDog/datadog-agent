# Datadog Trace Agent

Datadog documentation:<br>
- [docs.datadoghq.com/tracing/send_traces/#datadog-agent][1]

## Building the trace agent

Run `invoke trace-agent.build`.

## Running the trace agent

With the datadog agent already running, run `./bin/trace-agent/trace-agent`. If needed, pass your configuration file with `-config <path-to-yaml-file>`.

[1]: https://docs.datadoghq.com/tracing/send_traces/#datadog-agent
