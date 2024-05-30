# Inventory Agent Payload

This package populates some of the otel-agent related fields in the `inventories` product in DataDog. More specifically the
`datadog-otel-agent` table.

This is enabled by default if otel_enabled is set to true, and can be turned off using `inventories_enabled` config.

The payload is sent every 10min (see `inventories_max_interval` in the config) or whenever it's updated with at most 1
update every minute (see `inventories_min_interval`).

# Content

## Agent Configuration

The otel-agent configurations are scrubbed from any sensitive information (same logic than for the flare).
This include the following:
`otel_customer_configuration`
`otel_environment_configuration`
`otel_runtime_override_configuration`
`otel_runtime_configuration`

Sending Agent configuration can be disabled using `inventories_configuration_enabled`.

# Format

The payload is a JSON dict with the following fields

- `hostname` - **string**: the hostname of the otel-agent as shown on the status page.
- `uuid` - **string**: a unique identifier of the otel-agent, used in case the hostname is empty.
- `timestamp` - **int**: the timestamp when the payload was created.
- `otel-agent_metadata` - **dict of string to JSON type**:
  - `otel_agent_version` - **string**: the version of the OTel Agent in use.
  - `otel_agent_command` - **string**: the command used to launch the OTel Agent.
  - `otel_agent_description` - **string**: the internal description provided by the OTel Agent.
  - `otel_enabled` - **boolean**: describes if the OTel Agent has been enabled in the Agent configuration.
  - `otel_customer_configuration` - **string**: OTel Collector configuration provided by the customer.
  - `otel_environment_configuration` - **string**: OTel Collector environment variables defined.
  - `otel_runtime_override_configuration` - **string**: OTel Collector configuration overrides introduced by DD.
  - `otel_runtime_configuration` - **string**: full compiled OTel Collector configuration executing at runtime.


("scrubbed" indicates that secrets are removed from the field value just as they are in logs)

## Example Payload

Here an example of an inventory payload:

```
{
    "hostname": "COMP-GQ7WQN6HYC",
    "otel_metadata": {
        "otel_agent_command": "otelcol",
        "otel_agent_description": "foo bar",
        "otel_agent_version": "1.0.0",
        "otel_customer_configuration": "\nreceivers:\n  prometheus:\n    config:\n      scrape_configs:\n        - job_name: \"otelcol\"\n          scrape_interval: 10s\n          static_configs:\n            -
targets: [\"0.0.0.0:8888\"]\n          metric_relabel_configs:\n            - source_labels: [__name__]\n              regex: \".*grpc_io.*\"\n              action: drop\n  otlp:\n    protocols:\n      grpc:\n
http:\nexporters:\n  datadog:\n    api:\n      key: $DD_API_KEY\nservice:\n  pipelines:\n    traces:\n      receivers: [otlp]\n      exporters: [datadog]\n    metrics:\n      receivers: [otlp, prometheus]\n
exporters: [datadog]\n    logs:\n      receivers: [otlp]\n      exporters: [datadog]\"",
        "otel_enabled": true,
        "otel_environment_configuration": "",
        "otel_runtime_configuration": "\nreceivers:\n  prometheus:\n    config:\n      scrape_configs:\n        - job_name: \"otelcol\"\n          scrape_interval: 10s\n          static_configs:\n            -
targets: [\"0.0.0.0:8888\"]\n          metric_relabel_configs:\n            - source_labels: [__name__]\n              regex: \".*grpc_io.*\"\n              action: drop\n  otlp:\n    protocols:\n      grpc:\n
http:\nexporters:\n  datadog:\n    api:\n      key: $DD_API_KEY\nprocessors:\n  tagenrich:\n  batch:\n    timeout: 10s\nconnectors:\n  datadog/connector:\n\tcompute_stats_by_span_kind:
true\n\tpeer_tags_aggregation: true\nservice:\n  pipelines:\n    traces:\n      receivers: [otlp]\n      processors: [batch,tagenrich]\n      exporters: [datadog/connector,datadog]\n    metrics:\n      receivers:
[otlp, prometheus,datadog/connector]\n      processors: [batch,tagenrich]\n      exporters: [datadog]\n    logs:\n      receivers: [otlp]\n      processors: [batch,tagenrich]\n      exporters: [datadog]\"",
        "otel_runtime_override_configuration": ""
    },
    "timestamp": 1716985696922603000,
    "uuid": "eee7bdc9-93ce-5938-91c3-7643d7ba7674"
}
```
