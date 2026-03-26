# pkg/trace/traceutil

## Purpose

Provides low-level utility functions for operating on traces and spans. It is intentionally kept lean: the package may only import `pkg/proto/pbgo/trace` (and its `idx` sub-package) plus the internal `log` package. This constraint prevents circular dependencies and keeps the package usable from anywhere in the trace pipeline.

The package also contains a `normalize` sub-package (`traceutil/normalize`) for string normalization and truncation.

## Key elements

### `ProcessedTrace` and `ProcessedTraceV1` (`processed_trace.go`)

The primary data carrier through the trace agent's processing pipeline.

```go
type ProcessedTrace struct {
    TraceChunk             *pb.TraceChunk
    Root                   *pb.Span
    TracerEnv              string
    AppVersion             string
    TracerHostname         string
    ClientDroppedP0sWeight float64
    GitCommitSha           string
    ImageTag               string
    Lang                   string
}
```

`ProcessedTraceV1` is the equivalent for the v1 ingestion path, using `idx.InternalTraceChunk` and `idx.InternalSpan`.

Both types expose a `Clone()` method that shallow-copies the struct and its `TraceChunk` and `Root` fields, making it safe to modify the clone's top-level fields without affecting the original. Individual spans within `TraceChunk.Spans` are **not** deep-copied — callers must not mutate span contents through a clone.

### Span attribute helpers (`span.go`)

Getters and setters for the well-known internal span metrics/meta keys:

| Function | Key accessed |
|---|---|
| `HasTopLevel` / `SetTopLevel` | `_top_level` |
| `HasTopLevelMetrics` / `HasTopLevelMetricsV1` | `_top_level`, `_dd.top_level` |
| `UpdateTracerTopLevel` / `UpdateTracerTopLevelV1` | syncs `_dd.top_level` → `_top_level` |
| `IsMeasured` / `SetMeasured` | `_dd.measured` |
| `IsPartialSnapshot` | `_dd.partial_version` |
| `SetMetric` / `GetMetric` | arbitrary float64 metric |
| `SetMeta` / `GetMeta` / `GetMetaDefault` | arbitrary string meta tag |
| `SetMetaStruct` / `GetMetaStruct` | msgpack-serialized structured metadata in `MetaStruct` |
| `GetTraceIDHigh` / `SetTraceIDHigh` / `HasTraceIDHigh` | `_dd.p.tid` (high 64 bits of 128-bit trace ID) |
| `CopyTraceID(dst, src)` | copies both low and high 64-bit trace ID parts |
| `UpgradeTraceID(dst, src)` | promotes a 64-bit trace ID to 128-bit when high bits arrive later |
| `SameTraceID(a, b)` | compares full 128-bit trace IDs, tolerating absent high bits |

### Trace-level utilities (`trace.go`)

```go
func GetRoot(t pb.Trace) *pb.Span
func GetRootV1(t *idx.InternalTraceChunk) *idx.InternalSpan
func GetEnv(root *pb.Span, t *pb.TraceChunk) string
func ChildrenMap(t pb.Trace) map[uint64][]*pb.Span
func ComputeTopLevel(trace pb.Trace)
func ComputeTopLevelV1(trace *idx.InternalTraceChunk)
```

`GetRoot` finds the root span by looking for a span with `ParentID == 0`; if none exists it identifies the span whose parent ID does not appear in the trace (the orphan root). `ComputeTopLevel` marks all entry-point spans: root spans, orphans, and spans whose parent belongs to a different service (local roots).

### Azure App Service (`azure.go`)

```go
func GetAppServicesTags() map[string]string
```

Reads Azure App Service environment variables (`WEBSITE_SITE_NAME`, `WEBSITE_OWNER_NAME`, etc.) and returns a map of `aas.*` tag key-value pairs. Detects the runtime (Node.js, Java, .NET, Python, PHP, Go, Container) and distinguishes between Web Apps and Function Apps.

### Constants (`constants.go`)

```go
const TagStatusCode = "http.status_code"
```

### Sub-package: `normalize`

#### `normalize.go`

Functions for cleaning up span fields to meet backend requirements:

| Function | Purpose |
|---|---|
| `NormalizeName(name string) (string, error)` | Enforces metric-name rules (alpha start, alphanumeric + `.` + `_`). Truncates at 100 chars. Falls back to `"unnamed_operation"`. |
| `NormalizeService(svc, lang string) (string, error)` | Tag-value normalization + length limit (100 chars). Falls back to `"unnamed-<lang>-service"`. |
| `NormalizePeerService(svc string) (string, error)` | Like `NormalizeService` but returns `""` on empty input. |
| `NormalizeTag(v string) string` | Full tag normalization (`key:value`), strips illegal characters, lowercases, max 200 chars. |
| `NormalizeTagValue(v string) string` | Same as `NormalizeTag` but for values only (first character may be a digit). |

Sentinel errors: `ErrEmpty`, `ErrTooLong`, `ErrInvalid`.

Constants: `DefaultSpanName = "unnamed_operation"`, `DefaultServiceName = "unnamed-service"`, `MaxNameLen = 100`, `MaxServiceLen = 100`, `MaxResourceLen = 5000`.

The normalization implementation uses lookup tables for O(1) ASCII character validation and avoids heap allocations on the fast path (already-valid ASCII strings are returned as-is).

#### `truncate.go`

```go
func TruncateUTF8(s string, limit int) string
```

Truncates `s` to at most `limit` bytes while preserving valid UTF-8 — it trims trailing bytes until the suffix is a complete code point. Used by normalization functions internally.

## Usage

`pkg/trace/agent` calls `ComputeTopLevel` on every incoming trace chunk so that downstream components (`pkg/trace/event`, `pkg/trace/stats`) can identify top-level spans without re-traversing the span tree. The `ProcessedTrace` struct is the standard value passed between agent stages (sampling, event extraction, obfuscation, stats, writing). Normalize functions are called by both the native Datadog trace path and the OTLP conversion path (`pkg/trace/otel/traceutil`) before spans are written.
