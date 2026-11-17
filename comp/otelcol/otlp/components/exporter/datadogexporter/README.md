# Datadog Exporter

This package provides the Datadog exporter factory used inside the Datadog
Distribution of OpenTelemetry (DDOT) Collector. It builds on top of the
[`serializerexporter`](../serializerexporter), [`logsagentexporter`](../logsagentexporter),
and the trace agent component to send metrics, logs, and traces (plus APM
stats) to Datadog using the agent's internal pipelines.

## Configuration

The DDOT configuration adapter is defined in `datadogconfig`. It follows the upstream [Datadog exporter configuration](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/exporter/datadogexporter). Its experimental DI configuration uses the shared contrib `pkg/datadog/di.Config` type.

## Experimental DI export

Enable `exporters.datadog.dynamic_instrumentation.enabled` to send named OTLP DI events through the shared contrib DI module. The exporter owns delivery, credentials, queueing and retries. The Datadog extension only serves RC and metadata. RC and DI export can be enabled independently.

The shared DI module owns event classification, native payload conversion and HTTP delivery. The DDOT binding only supplies configuration and lifecycle hooks inside the existing logs queue. It does not discover or call an extension sender.

The draft pins the shared module to a Verified commit in the [upstream DI experiment](https://github.com/open-telemetry/opentelemetry-collector-contrib/pull/53248), using an immutable fork replacement. Replace that pin with an upstream module release before shipping. Collector core dependencies stay at the DDOT baseline.
