> **TL;DR:** `pkg/trace/otel` provides the OTel-to-Datadog conversion utilities for ingesting OTLP traces — mapping span attributes to Datadog fields, detecting top-level spans, converting trace/span IDs, and bridging OTLP payloads into the APM stats concentrator pipeline.

# pkg/trace/otel

## Purpose

Provides utilities for ingesting and converting OpenTelemetry (OTel/OTLP) traces into Datadog's internal trace representation. The package is a Go module (`go.mod`) separate from the main `pkg/trace` module, allowing the OTel Collector's Datadog exporter to import it independently.

The package is organized into three sub-packages:

| Sub-package | Responsibility |
|---|---|
| `otel/traceutil` | Semantic conversion — mapping OTel span attributes to Datadog span fields (service, resource, operation name, type, top-level detection). |
| `otel/stats` | Stats pipeline bridge — converting OTLP traces into `stats.Input` values consumed by the Concentrator for APM stats computation. |
| `otel/integration` | Integration tests that exercise the full OTLP ingestion path end-to-end (no production code, tests only). |

## Key elements

### `otel/traceutil`

#### Span indexing and top-level detection

```go
func IndexOTelSpans(traces ptrace.Traces) (
    map[pcommon.SpanID]ptrace.Span,
    map[pcommon.SpanID]pcommon.Resource,
    map[pcommon.SpanID]pcommon.InstrumentationScope,
)
func GetTopLevelOTelSpans(spanByID, resByID, topLevelByKind) map[pcommon.SpanID]struct{}
```

`IndexOTelSpans` flattens the three-level OTLP hierarchy (resource spans → scope spans → spans) into flat maps keyed by span ID. Spans with an empty trace ID or span ID are skipped. `GetTopLevelOTelSpans` identifies entry-point spans: root spans (no parent), orphan spans (parent not in the same payload), or — when `topLevelByKind` is true — Server and Consumer kind spans regardless of parent presence.

#### Semantic field resolution

These functions translate OTel semantic conventions to Datadog span fields. V1 variants use the legacy `GetOTelAttrValInResAndSpanAttrs` attribute lookup; V2 variants use the semantics registry (`pkg/trace/semantics`) with multi-version fallback chains.

| Function | Returned Datadog field |
|---|---|
| `GetOTelService` / `GetOTelServiceWithAccessor` | `span.service` |
| `GetOTelResourceV1` / `GetOTelResourceV2` / `GetOTelResourceV2WithAccessor` | `span.resource` |
| `GetOTelOperationNameV1` / `GetOTelOperationNameV2` / `GetOTelOperationNameV2WithAccessor` | `span.name` (operation name) |
| `SpanKind2Type` / `GetOTelSpanType` / `GetOTelSpanTypeWithAccessor` | `span.type` (`"web"`, `"db"`, `"cache"`, `"http"`, `"custom"`) |

`GetOTelOperationNameV2` applies rich protocol-aware naming (e.g. `http.server.request`, `redis.query`, `aws.s3.request`) based on span kind and semantic attributes.

Functions that end in `WithAccessor` accept a pre-created `semantics.Accessor` to avoid repeated allocation in tight loops where the same span and resource attribute maps are accessed many times.

#### Attribute helpers

```go
func GetOTelAttrVal(attrs pcommon.Map, normalize bool, keys ...string) string
func GetOTelAttrFromEitherMap(map1, map2 pcommon.Map, normalize bool, keys ...string) string
func GetOTelAttrValInResAndSpanAttrs(span ptrace.Span, res pcommon.Resource, normalize bool, keys ...string) string
func GetOTelContainerTags(rattrs pcommon.Map, tagKeys []string) []string
```

`GetOTelContainerTags` produces normalized `key:value` container tag strings from OTel resource attributes using `pkg/opentelemetry-mapping-go/otlp/attributes`.

#### ID conversion

```go
func OTelTraceIDToUint64(b [16]byte) uint64   // uses the low 64 bits
func OTelSpanIDToUint64(b [8]byte) uint64
func OTelSpanKindName(k ptrace.SpanKind) string
```

OTel trace IDs are 128-bit; Datadog uses 64-bit. `OTelTraceIDToUint64` extracts the lower 8 bytes (big-endian), matching the convention used by Datadog tracers for 128-bit ID propagation.

### `otel/stats`

```go
func OTLPTracesToConcentratorInputs(
    traces ptrace.Traces,
    conf *config.AgentConfig,
    containerTagKeys, peerTagKeys []string,
) []stats.Input

func OTLPTracesToConcentratorInputsWithObfuscation(
    traces ptrace.Traces,
    conf *config.AgentConfig,
    containerTagKeys, peerTagKeys []string,
    obfuscator *obfuscate.Obfuscator,
) []stats.Input
```

Thin wrappers that delegate to `pkg/trace/stats.OTLPTracesToConcentratorInputsWithObfuscation`. They convert OTLP spans into the minimal `stats.Input` structs required by the Concentrator's `Add()` method. The `WithObfuscation` variant applies SQL/Redis/etc. obfuscation before stats bucketing so that resource names in APM stats match what would appear in the Datadog UI.

## Usage

`pkg/trace/api` (`otlp.go`) uses `otel/traceutil` to convert incoming OTLP payloads into `pb.TracerPayload` before handing them off to the agent pipeline:

```go
import oteltraceutil "github.com/DataDog/datadog-agent/pkg/trace/otel/traceutil"

svc := oteltraceutil.GetOTelService(span, res, true)
resName := oteltraceutil.GetOTelResourceV2(span, res)
opName := oteltraceutil.GetOTelOperationNameV2(span, res)
```

`pkg/trace/stats` (`otel.go`) and the OTel Collector Datadog exporter use `otel/stats` to compute APM stats directly from OTLP data without full trace conversion:

```go
import otelstats "github.com/DataDog/datadog-agent/pkg/trace/otel/stats"

inputs := otelstats.OTLPTracesToConcentratorInputs(traces, conf, containerTagKeys, peerTagKeys)
for _, inp := range inputs {
    concentrator.Add(inp)
}
```
