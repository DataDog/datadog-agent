> **TL;DR:** `pkg/trace/semantics` provides a versioned, data-driven registry of semantic attribute equivalences across Datadog and OTel tracing conventions, so that lookup code can resolve concepts like "HTTP status code" through a canonical precedence chain without embedding attribute-name strings directly.

# pkg/trace/semantics

## Purpose

`semantics` provides a versioned, data-driven registry of semantic attribute equivalences
across tracing conventions (Datadog tracers and OpenTelemetry). It solves the problem that
the same observable concept — for example "HTTP status code" — is represented by different
attribute keys in different tracing conventions and across OTel semconv versions
(`http.status_code` in v1, `http.response.status_code` from v1.23+). The registry defines a
canonical `Concept` identifier for each such concept and an ordered list of attribute names to
check, so lookup code does not need to embed attribute-name strings directly.

## Key elements

### Concepts

**`Concept`** (type `string`) — An opaque identifier for a semantic concept, defined as
package-level constants grouped by category:

| Category | Example constants |
|---|---|
| Peer tags | `ConceptPeerService`, `ConceptPeerHostname`, `ConceptPeerDBName`, `ConceptPeerDBSystem`, `ConceptPeerMessagingSystem`, `ConceptPeerRPCService`, … |
| Stats aggregation | `ConceptHTTPStatusCode`, `ConceptHTTPMethod`, `ConceptHTTPRoute`, `ConceptGRPCStatusCode`, `ConceptSpanKind` |
| Service / resource identification | `ConceptServiceName`, `ConceptResourceName`, `ConceptOperationName`, `ConceptSpanType`, `ConceptDBSystem`, `ConceptRPCSystem`, `ConceptDeploymentEnv`, `ConceptContainerID`, `ConceptK8sPodUID` |
| Obfuscation | `ConceptDBQuery`, `ConceptMongoDBQuery`, `ConceptElasticsearchBody`, `ConceptRedisRawCommand`, `ConceptHTTPURL`, … |
| Normalization | `ConceptMessagingOperation`, `ConceptGraphQLOperationType`, `ConceptFaaSInvokedProvider`, `ConceptRPCMethod`, … |
| Sampling | `ConceptDDMeasured`, `ConceptDDTopLevel`, `ConceptSamplingPriority`, `ConceptOTelTraceID` |

### Registry and mappings

**`Registry`** (interface) — Three methods:
- `GetAttributePrecedence(concept)` — returns an ordered `[]TagInfo` slice; first entry has
  highest precedence.
- `GetAllEquivalences()` — returns a copy of the full concept→tags map.
- `Version()` — returns the semver string from `mappings.json`.

**`EmbeddedRegistry`** — The sole production implementation. Loads mappings at package init
time from `mappings.json`, which is embedded via `//go:embed`. The global instance is
accessed via `DefaultRegistry()`.

**`TagInfo`** — Describes one attribute entry in the precedence list:
```go
type TagInfo struct {
    Name     string    // attribute key, e.g. "http.response.status_code"
    Provider Provider  // "datadog" or "otel"
    Version  string    // semconv version that introduced this key, if applicable
    Type     ValueType // "string", "float64", or "int64"
}
```

**`mappings.json`** — Checked-in data file (embedded at compile time) that is the single
source of truth for attribute precedence. Editing this file is the correct way to add or
update semantic equivalences; no Go code changes are needed for mapping updates.

### Accessors

The `Accessor` interface (`GetString`, `GetInt64`, `GetFloat64`) decouples registry lookups
from the underlying data structure. Four implementations are provided:

| Type | Wraps | Notes |
|---|---|---|
| `StringMapAccessor` | `map[string]string` (DD span `Meta`) | `GetInt64`/`GetFloat64` always return not-found; use `"string"`-typed registry entries. |
| `MetricsMapAccessor` | `map[string]float64` (DD span `Metrics`) | `GetString` always returns `""`; converts exact-integer floats for `GetInt64`. |
| `DDSpanAccessor` | `meta` + `metrics` maps | Routes string lookups to `StringMapAccessor`, numeric to `MetricsMapAccessor`. |
| `DDSpanAccessorV1` | `*idx.InternalSpan` | Strictly typed; reads from the compressed internal span representation. |
| `PDataMapAccessor` | `pcommon.Map` (OTel) | Type-strict; returns not-found when the pdata type doesn't match. |
| `OTelSpanAccessor` | span attrs + resource attrs `pcommon.Map` | Primary (span) takes precedence over secondary (resource) for each getter. |

### Lookup functions

All lookup functions are generic over `Accessor`:

| Function | Returns | Description |
|---|---|---|
| `Lookup[A](r, accessor, concept)` | `(LookupResult, bool)` | Returns the first matching `TagInfo` and its string-formatted value. |
| `LookupString[A](r, accessor, concept)` | `string` | Convenience wrapper; returns `""` on miss. |
| `LookupInt64[A](r, accessor, concept)` | `(int64, bool)` | Tries typed getters first; falls back to string parsing. |
| `LookupFloat64[A](r, accessor, concept)` | `(float64, bool)` | Same pattern as `LookupInt64`. |

## Usage

### Creating an accessor and performing a lookup

```go
// OTel path (span attributes take precedence over resource attributes)
accessor := semantics.NewOTelSpanAccessor(otelspan.Attributes(), otelres.Attributes())
if code, ok := semantics.LookupInt64(semantics.DefaultRegistry(), accessor, semantics.ConceptHTTPStatusCode); ok {
    ddspan.Metrics[traceutil.TagStatusCode] = float64(code)
}

// DD span path
accessor := semantics.NewDDSpanAccessor(span.Meta, span.Metrics)
env := semantics.LookupString(semantics.DefaultRegistry(), accessor, semantics.ConceptDeploymentEnv)
```

### Where it is used

- **`pkg/trace/transform`** — All OTel-to-DD span conversions use `OTelSpanAccessor` and
  `DefaultRegistry()` to resolve HTTP status code, deployment env, service version, container
  ID, measured flag, and more.
- **`pkg/trace/otel/traceutil`** — `LookupSemanticStringWithAccessor` and related helpers
  delegate to this package for operation name, resource name, and span type derivation.
- **`pkg/trace/stats`** — Uses `LookupSemanticInt64` / `LookupSemanticString` for stats
  bucket key extraction (HTTP status, gRPC status, span kind).
- **`pkg/trace/agent`** (obfuscation) — Uses `NewStringMapAccessor` + `LookupString` to
  resolve obfuscation target attributes (e.g. `ConceptDBQuery`, `ConceptHTTPURL`) from already
  converted DD spans.

### Extending the registry

To add a new attribute equivalent for an existing concept, edit `mappings.json`. Insert a new
entry in the `fallbacks` array at the appropriate precedence position. No Go code changes are
required. To add a new concept, add both a `Concept` constant in `semantics.go` and a new
entry in `mappings.json`.
