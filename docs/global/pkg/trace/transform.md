# pkg/trace/transform

## Purpose

`transform` converts OpenTelemetry (OTLP) spans and resources into Datadog APM span format
(`pb.Span`) and provides obfuscation helpers for sensitive span attributes. It is the primary
translation layer between the OTLP ingestion path and the internal Datadog trace representation.

## Key elements

### Span conversion

**`OtelSpanToDDSpan(otelspan, otelres, lib, conf)`** — Full conversion of an OTel span to a
`*pb.Span`. In addition to the fields produced by the minimal variant, it copies all span and
resource attributes into `Meta`/`Metrics`, marshals events and links to JSON, sets OTel-specific
metadata tags (`otel.trace_id`, `otel.status_code`, `otel.library.name`/`.version`), resolves
`error.msg`/`error.type`/`error.stack` from the status and exception events, and handles
`db.namespace` → `db.name` mapping.

**`OtelSpanToDDSpanMinimal(otelspan, otelres, lib, isTopLevel, topLevelByKind, conf, peerTagKeys)`**
— Lightweight variant that populates only the fields required for APM stats computation
(service, name, resource, type, IDs, timestamps, error, span kind, HTTP status code,
top-level/measured markers, peer tags). Used by `pkg/trace/stats` to avoid the overhead of full
attribute copying for concentrator inputs.

**`OperationAndResourceNameV2Enabled(conf)`** — Returns true when the v2 operation/resource name
logic should be used (i.e. `SpanNameAsResourceName` is off, no `SpanNameRemappings` are
configured, and the `disable_operation_and_resource_name_logic_v2` feature flag is absent).
Callers use this to choose between the v1 and v2 name resolution paths.

### Attribute mapping helpers

**`GetDDKeyForOTLPAttribute(k)`** — Maps an OTLP HTTP attribute key to its Datadog equivalent
(via `attributes.HTTPMappings`). Handles `http.request.header.*` prefix rewriting. Returns
`""` for Datadog APM convention keys (`service.name`, `operation.name`, `resource.name`,
`span.type`) because those are handled by dedicated logic elsewhere.

**`SetMetaOTLP(s, k, v)` / `SetMetaOTLPIfEmpty(s, k, v)`** — Set a string attribute on a span,
routing well-known Datadog APM keys to the corresponding struct fields (`s.Name`, `s.Service`,
`s.Resource`, `s.Type`) and `analytics.event` to `Metrics[KeySamplingRateEventExtraction]`.
The `IfEmpty` variant skips fields that are already populated.

**`SetMetricOTLP(s, k, v)` / `SetMetricOTLPIfEmpty(s, k, v)`** — Set a numeric attribute,
mapping `sampling.priority` to `_sampling_priority_v1`.

### Resource/metadata extraction helpers

| Function | Description |
|---|---|
| `GetOTelEnv(span, res)` | Reads `deployment.environment` (with OTel/DD fallbacks via the semantics registry). |
| `GetOTelHostname(span, res, tr, fallback)` | Resolves the DD hostname from resource attributes using the OTel attribute translator; returns `""` in serverless environments. |
| `GetOTelVersion(span, res)` | Reads `service.version`. |
| `GetOTelContainerID(span, res)` | Reads `container.id`. |
| `GetOTelContainerOrPodID(span, res)` | Reads `container.id`, falling back to `k8s.pod.uid` for backward compatibility. |
| `GetOTelStatusCode(span, res)` | Returns the HTTP status code as `uint32`, or 0 if absent/negative. |
| `GetOTelContainerTags(rattrs, tagKeys)` | Builds a list of normalized `key:value` container tags from resource attributes using `attributes.ContainerMappings`. |

### Event and link marshalling

**`MarshalEvents(events)`** — Serialises OTel `SpanEventSlice` to a compact JSON string
stored in `span.Meta["events"]`. Errors in individual attribute serialisation are handled
gracefully (value replaced with `"redacted"`).

**`MarshalLinks(links)`** — Serialises OTel `SpanLinkSlice` to a compact JSON string stored
in `span.Meta["_dd.span_links"]`.

**`TagSpanIfContainsExceptionEvent(otelspan, ddspan)`** — Sets
`_dd.span_events.has_exception = "true"` when any span event is named `"exception"`.

### Error conversion

**`Status2Error(status, events, metaMap)`** — Returns `1` if the OTel status is `Error`,
populating `error.msg` from the status message or HTTP status code text if not already set.
Returns `0` otherwise.

### Obfuscation (`obfuscate.go`)

Obfuscation functions accept an `*obfuscate.Obfuscator` (from `pkg/obfuscate`) and a `*pb.Span`.

| Function | Obfuscates |
|---|---|
| `ObfuscateSQLSpan(o, span)` | `span.Resource` and `sql.query` tag; also handles OTel `db.statement` / `db.query.text` attributes, reusing the same obfuscated value when they match the resource. Sets `sql.tables` from obfuscation metadata. |
| `ObfuscateRedisSpan(o, span, removeAllArgs)` | `redis.raw_command` tag. |
| `ObfuscateValkeySpan(o, span, removeAllArgs)` | `valkey.raw_command` tag. |

Constants for tag names (`TagRedisRawCommand`, `TagSQLQuery`, `TagHTTPURL`, etc.) are exported
so callers outside the package can reference the same string literals.

## Usage

The primary consumer is the OTLP receiver in `pkg/trace/api/otlp.go`:

```go
// Full conversion for ingestion
ddspan := transform.OtelSpanToDDSpan(otelspan, otelres, libspans.Scope(), o.conf)
```

The minimal conversion is used by the stats concentrator input builder in
`pkg/trace/stats/otel.go`:

```go
ddSpan := transform.OtelSpanToDDSpanMinimal(
    otelspan, otelres, scopeByID[spanID], isTop, topLevelByKind, conf, peerTagKeys,
)
```

Obfuscation functions are called from `pkg/trace/agent` after spans are received, using the
agent-level `Obfuscator` instance. The `IfEmpty` setter variants are used during attribute
iteration to ensure that explicitly set Datadog convention values are never overwritten by OTel
attribute copies.
