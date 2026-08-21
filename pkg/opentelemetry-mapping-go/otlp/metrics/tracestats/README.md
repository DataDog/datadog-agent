# Trace stats mapping

This package remaps the delta OTLP histogram `traces.span.sdk.metrics.duration` into an APM stats payload. Its main source is Datadog SDKs running in OTel mode, where client-side trace stats are computed before head sampling and exported through OTLP.

| OTLP input | Datadog stats output |
| --- | --- |
| Histogram count, sum, and buckets | `Hits`, `Duration`, and either `ok_sparse_sketch` or `error_sparse_sketch` |
| `service.name` | `Service` (a data-point value overrides the resource value) |
| `deployment.environment.name` | `Env` |
| `service.version` | `Version` |
| `telemetry.sdk.language` | Payload `Languages` |
| `span.name` | `Resource` |
| `span.kind` | `SpanKind` |
| `status.code` | `Errors` and `HasErrors`; accepts OTel error code `2` and case-insensitive `ERROR` / `STATUS_CODE_ERROR` |
| `http.response.status_code` | `HttpStatusCode` |
| `rpc.response.status_code` | `GrpcStatusCode` |
| `datadog.operation.name`, or OTel semantic attributes as a fallback | `Name` |
| `datadog.span.top_level` / `datadog.is_trace_root` | `TopLevelHits` / `IsTraceRoot` |
| `datadog.span.type`, `datadog.origin`, and other additional metric attributes | Normalized `OtherTags`; origin also sets `Synthetics` when applicable |
| `datadog.peer_tags` | `PeerTags` |
| Base service | `BaseService` exists in the protobuf but is not populated |

Only delta histograms with the exact SDK metric name are remapped. Invalid data points are skipped independently. The package owns the stats protobuf definitions and serializes the complete `OTLPIntakeStatsPayload` with protobuf.
