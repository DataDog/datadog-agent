# Datadog Connector

This package provides the Datadog connector factory used inside the Datadog
Distribution of OpenTelemetry (DDOT) Collector. The Datadog connector derives
APM statistics (and, in the traces-to-traces pipeline, forwards traces) from
service traces so that trace-emitting services appear in Datadog APM.

It is a local copy of the upstream
[`connector/datadogconnector`](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/connector/datadogconnector)
factory together with the connector implementation from
[`pkg/datadog/apmstats`](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/pkg/datadog/apmstats),
vendored here so the embedded collector no longer depends on contrib for the
connector logic. The factory wires the connector to the Agent's tagger,
hostname provider, and (optionally) an existing stats `Concentrator`. Config and
feature-gate definitions are still shared with upstream via the `pkg/datadog`
module.

## Configuration

This factory does not define its own configuration struct. It accepts the same
configuration as the upstream Datadog connector, which is the
`ConnectorComponentConfig` type from
[`pkg/datadog/config`](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/pkg/datadog/config).
