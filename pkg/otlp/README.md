# package `otlp`

This package is responsible for handling [telemetry signals](https://github.com/open-telemetry/opentelemetry-specification/blob/v1.6.1/specification/glossary.md#signals) from external software in the [OTLP](https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/otlp.md) format on any supported protocol (gRPC/protobuf, HTTP/JSON, HTTP/protobuf...).

Any telemetry signal sent via OTLP to the Agent must be sent to the endpoint defined by this package on the core Agent first, to support the [single endpoint configuration](https://github.com/open-telemetry/opentelemetry-specification/blob/v1.6.1/specification/protocol/exporter.md#configuration-options) of OTLP exporters defined by the OpenTelemetry specification. Telemetry signals may be forwarded to other agents internally after intake.

