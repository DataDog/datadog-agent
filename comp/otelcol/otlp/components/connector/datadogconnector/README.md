# Datadog Connector

This package provides the Datadog connector used inside the Datadog
Distribution of OpenTelemetry (DDOT) Collector. The Datadog connector derives
APM statistics (and, in the traces-to-traces pipeline, forwards traces) from
service traces so that trace-emitting services appear in Datadog APM.

The factory wires the connector to the Agent's tagger, hostname provider, and
(optionally) an existing stats `Concentrator`.

## Configuration

This package does not define its own configuration struct. It uses the
`ConnectorComponentConfig` type from
[`pkg/datadog/config`](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/pkg/datadog/config),
shared with the Datadog exporter.
