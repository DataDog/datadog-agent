# Datadog Exporter

This package provides the Datadog exporter factory used inside the Datadog
Distribution of OpenTelemetry (DDOT) Collector. It builds on top of the
[`serializerexporter`](../serializerexporter), [`logsagentexporter`](../logsagentexporter),
and the trace agent component to send metrics, logs, and traces (plus APM
stats) to Datadog using the agent's internal pipelines.

## Configuration

This factory does not define its own configuration struct. It accepts the
same configuration as the upstream Datadog exporter in
[`opentelemetry-collector-contrib`](https://github.com/open-telemetry/opentelemetry-collector-contrib),
which is the `Config` type from
[`pkg/datadog/config`](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/pkg/datadog/config).

The corresponding configuration schema is published upstream at
[`pkg/datadog/config/config.schema.yaml`](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/pkg/datadog/config/config.schema.yaml).
That single file is the source of truth for every option this exporter
accepts (API, host metadata, metrics, traces, logs, retry, queue, etc.).
