# Semantic Attribute Registry Change

This document describes the change from hardcoded attribute lookups to the centralized semantic attribute registry in the trace-agent.

## Overview

The trace-agent previously used hardcoded attribute key strings scattered across the codebase. This migration introduces a centralized `semantics` package that:

1. Maps canonical concept names to equivalent attribute keys across tracing conventions
2. Handles precedence-ordered fallback chains automatically
3. Supports multiple OTel semantic convention versions

## Migration Summary

### Functions Affected in `pkg/trace/otel/traceutil/otel_util.go`

| Function | Description |
|----------|-------------|
| `SpanKind2Type` | Determines span type (web, db, cache, http, custom) |
| `GetOTelSpanType` | Returns DD span type from OTel attributes |
| `GetOTelService` | Extracts service name |
| `GetOTelResourceV2` | Builds resource name from attributes |
| `GetOTelOperationNameV2` | Builds operation name from attributes |

### Attribute Mapping Changes

| Concept | Old (Hardcoded) | New (Semantic Registry) |
|---------|-----------------|-------------------------|
| Service Name | `semconv.ServiceNameKey` | `ConceptServiceName` → checks `service.name` |
| DB System | `semconv.DBSystemKey` | `ConceptDBSystem` → checks `db.system` |
| DB Statement | `semconv.DBStatementKey` | `ConceptDBStatement` → checks `db.statement`, `db.query.text` |
| HTTP Method | `"http.request.method"`, `semconv.HTTPMethodKey` | `ConceptHTTPMethod` → checks `http.request.method`, `http.method` |
| HTTP Route | `semconv.HTTPRouteKey` | `ConceptHTTPRoute` → checks `http.route` |
| Messaging Operation | `semconv.MessagingOperationKey` | `ConceptMessagingOperation` → checks `messaging.operation` |
| Messaging Destination | `semconv.MessagingDestinationKey`, `semconv117.MessagingDestinationNameKey` | `ConceptMessagingDest` → checks `messaging.destination`, `messaging.destination.name` |
| Messaging System | `semconv.MessagingSystemKey` | `ConceptMessagingSystem` → checks `messaging.system` |
| RPC Method | `semconv.RPCMethodKey` | `ConceptRPCMethod` → checks `rpc.method` |
| RPC Service | `semconv.RPCServiceKey` | `ConceptRPCService` → checks `rpc.service` |
| RPC System | `semconv.RPCSystemKey` | `ConceptRPCSystem` → checks `rpc.system` |
| GraphQL Op Type | `semconv117.GraphqlOperationTypeKey` | `ConceptGraphQLOperationType` → checks `graphql.operation.type` |
| GraphQL Op Name | `semconv117.GraphqlOperationNameKey` | `ConceptGraphQLOperationName` → checks `graphql.operation.name` |
| FaaS Provider | `semconv.FaaSInvokedProviderKey` | `ConceptFaaSInvokedProvider` → checks `faas.invoked_provider` |
| FaaS Name | `semconv.FaaSInvokedNameKey` | `ConceptFaaSInvokedName` → checks `faas.invoked_name` |
| FaaS Trigger | `semconv.FaaSTriggerKey` | `ConceptFaaSTrigger` → checks `faas.trigger` |
| Network Protocol | `"network.protocol.name"` | `ConceptNetworkProtocolName` → checks `network.protocol.name` |
| Span Type | `"span.type"` | `ConceptSpanType` → checks `span.type` |
| Resource Name | `"resource.name"` | `ConceptResourceName` → checks `resource.name` |
| Operation Name | `"operation.name"` | `ConceptOperationName` → checks `operation.name` |

## Behavioral Changes

### 1. DB Query Lookup Consolidation

**Before:** Two separate lookups for `db.statement` and `db.query.text`:
```go
if dbStatement := GetOTelAttrFromEitherMap(..., string(semconv.DBStatementKey)); dbStatement != "" {
    resName = dbStatement
    return
}
if dbQuery := GetOTelAttrFromEitherMap(..., string(semconv126.DBQueryTextKey)); dbQuery != "" {
    resName = dbQuery
    return
}
```

**After:** Single semantic lookup with built-in fallback:
```go
if dbStatement := LookupSemanticStringFromDualMaps(..., semantics.ConceptDBStatement, false); dbStatement != "" {
    resName = dbStatement
    return
}
```

The `ConceptDBStatement` mapping in `mappings.json` handles both:
```json
"db.statement": {
  "fallbacks": [
    {"name": "db.statement", "provider": "otel"},
    {"name": "db.query.text", "provider": "otel", "version": "1.24.0"}
  ]
}
```

### 2. HTTP Method Lookup Consolidation

**Before:** Multiple keys passed to single function:
```go
GetOTelAttrFromEitherMap(sattr, rattr, false, "http.request.method", string(semconv.HTTPMethodKey))
```

**After:** Semantic lookup with registry-defined precedence:
```go
LookupSemanticStringFromDualMaps(sattr, rattr, semantics.ConceptHTTPMethod, false)
```

The `ConceptHTTPMethod` mapping handles version differences:
```json
"http.method": {
  "fallbacks": [
    {"name": "http.request.method", "provider": "otel", "version": "1.21.0"},
    {"name": "http.method", "provider": "otel"}
  ]
}
```

### 3. Messaging Destination Consolidation

**Before:** Multiple semconv versions:
```go
GetOTelAttrFromEitherMap(..., string(semconv.MessagingDestinationKey), string(semconv117.MessagingDestinationNameKey))
```

**After:** Single concept with version-aware fallbacks:
```go
LookupSemanticStringFromDualMaps(..., semantics.ConceptMessagingDest, false)
```

## No Breaking Changes

The migration preserves existing behavior:

1. **Precedence order** - Attribute fallback order matches previous hardcoded order
2. **Normalization** - The `shouldNormalize` parameter is passed through unchanged
3. **V1 vs V2 logic** - `SpanKind2Type` (V1) still has resource-first precedence; `GetOTelSpanType` (V2) has span-first precedence

## Known Inconsistencies in Existing Hardcoded Fallbacks

The original codebase has **inconsistent precedence** between "old" and "new" semconv attribute keys. The semantic registry preserves these inconsistencies to avoid breaking changes, but they should be addressed in a future cleanup.

### 1. HTTP Status Code Precedence Inconsistency

Two functions handle `http.status_code` vs `http.response.status_code` with **opposite** precedence:

| Function | File | Precedence | Notes |
|----------|------|------------|-------|
| `GetOTelStatusCode` | `transform.go` | OLD first (`http.status_code`) | Migrated to semantic lookup |
| `Status2Error` | `transform.go` | NEW first (`http.response.status_code`) | **NOT migrated** |

**Current `mappings.json` configuration:**
```json
"http.status_code": {
  "fallbacks": [
    {"name": "http.status_code", ...},           // OLD first
    {"name": "http.response.status_code", ...}   // NEW second
  ]
}
```

**Impact:** If `Status2Error` is migrated to use the semantic lookup, test `TestFallbackInconsistency_Status2ErrorHTTPCodePrecedence` will fail because it expects NEW first behavior.

### 2. DB Statement vs Query Text Precedence

**Current behavior:** OLD first (`db.statement` before `db.query.text`)

This is inconsistent with HTTP method precedence which uses NEW first.

### 3. Messaging Destination Precedence

**Current behavior:** OLD first (`messaging.destination` before `messaging.destination.name`)

This is inconsistent with HTTP method precedence which uses NEW first.

### 4. Span vs Resource Attribute Precedence Inconsistency

Two helper functions have **opposite** precedence for span vs resource attributes:

| Function | Precedence | Used By |
|----------|------------|---------|
| `GetOTelAttrFromEitherMap` | Span first (map1) | `GetOTelSpanType`, `GetOTelResourceV2`, etc. |
| `GetOTelAttrValInResAndSpanAttrs` | Resource first | `SpanKind2Type` (V1 logic) |

The semantic registry uses span-first precedence by default. Functions using resource-first precedence explicitly pass arguments in the opposite order.

## Functions Not Yet Migrated to Semantic Lookup

### `pkg/trace/transform/transform.go`

The following functions still use hardcoded lookups:

| Line | Function/Usage | Current Lookup | Concept | Why Not Migrated |
|------|----------------|----------------|---------|------------------|
| 83 | Span type | `GetOTelAttrValInResAndSpanAttrs(..., "span.type")` | `ConceptSpanType` | Phase 2 scope |
| 98 | Measured flag | `GetOTelAttrFromEitherMap(..., "_dd.measured")` | `ConceptDDMeasured` | Phase 2 scope |
| 105 | Peer tags | `GetOTelAttrFromEitherMap(..., peerTagKey)` | Various peer concepts | Dynamic keys |
| 162 | Environment | `GetOTelAttrFromEitherMap(..., DeploymentEnvironmentNameKey, DeploymentEnvironmentKey)` | `ConceptDeploymentEnv` | Phase 2 scope |
| 170 | Hostname | `GetOTelAttrValInResAndSpanAttrs(..., "_dd.hostname")` | N/A | DD-specific |
| 190 | Version | `GetOTelAttrFromEitherMap(..., ServiceVersionKey)` | `ConceptServiceVersion` | Phase 2 scope |
| 195 | Container ID | `GetOTelAttrFromEitherMap(..., ContainerIDKey)` | `ConceptContainerID` | Phase 2 scope |
| 203 | Container ID fallback | `GetOTelAttrFromEitherMap(..., ContainerIDKey, K8SPodUIDKey)` | `ConceptContainerID` | Phase 2 scope |
| 334 | DB namespace | `GetOTelAttrValInResAndSpanAttrs(..., DBNamespaceKey)` | N/A | No concept yet |
| 567 | HTTP status (Status2Error) | `GetFirstFromMap(..., "http.response.status_code", "http.status_code")` | `ConceptHTTPStatusCode` | **Precedence conflict** |

### Why These Functions Were Not Migrated

**1. Incremental Rollout Strategy**

The semantic library migration was implemented in phases:
- **Phase 1 (this PR):** Core span conversion in `otel_util.go` - resource name, operation name, span type, service name
- **Phase 2 (future):** Additional lookups in `transform.go` - environment, version, container ID, etc.

This phased approach allows:
- Easier code review and testing
- Lower risk of introducing regressions
- Ability to validate the semantic library before broader adoption

**2. `Status2Error` Precedence Conflict**

The `Status2Error` function cannot be migrated without a breaking change because it uses **opposite precedence** from `mappings.json`:

| Source | Precedence |
|--------|------------|
| `Status2Error` hardcoded | `http.response.status_code` first (NEW) |
| `mappings.json` | `http.status_code` first (OLD) |

Migrating would change runtime behavior and fail `TestFallbackInconsistency_Status2ErrorHTTPCodePrecedence`.

**3. Dynamic Peer Tag Keys**

Line 105 iterates over dynamic `peerTagKey` values from configuration. This requires a different approach - either:
- Map each peer tag key to its concept at runtime
- Keep the dynamic lookup but use the registry for precedence

**4. No Concept Defined**

Some attributes like `_dd.hostname` and `db.namespace` don't have concepts defined in `semantics.go` yet because they're either:
- Datadog-specific with no fallbacks needed
- Not yet part of the semantic model

### Migration Risk: `Status2Error`

The `Status2Error` function (line 567) uses **NEW first** precedence:
```go
GetFirstFromMap(metaMap, "http.response.status_code", "http.status_code")
```

But `mappings.json` has **OLD first** for `http.status_code`. Migrating this function would change behavior and fail existing tests.

**Recommended resolution:** Either:
1. Update `mappings.json` to use NEW first for `http.status_code` (breaking change for `GetOTelStatusCode`)
2. Create a separate concept `ConceptHTTPStatusCodeNewFirst` with reversed precedence
3. Leave `Status2Error` unmigrated and document the inconsistency

### `pkg/trace/api/otlp.go`

This file has significant hardcoded lookups that were not migrated:

| Line | Usage | Current Lookup | Concept | Why Not Migrated |
|------|-------|----------------|---------|------------------|
| 298 | Container ID | `GetOTelAttrVal(..., ContainerIDKey)` | `ConceptContainerID` | Phase 2 scope |
| 300 | Container ID fallback | `GetOTelAttrVal(..., ContainerIDKey, K8SPodUIDKey)` | `ConceptContainerID` | Phase 2 scope |
| 304 | Environment | `GetOTelAttrVal(..., DeploymentEnvironmentNameKey, DeploymentEnvironmentKey)` | `ConceptDeploymentEnv` | Phase 2 scope |
| 354 | SDK Language | `GetOTelAttrVal(..., TelemetrySDKLanguageKey)` | N/A | No concept needed |
| 358 | SDK Version | `GetOTelAttrVal(..., TelemetrySDKVersionKey)` | N/A | No concept needed |
| 369 | Stats computed | `GetOTelAttrVal(..., keyStatsComputed)` | N/A | DD-specific |
| 412 | Environment | `GetFirstFromMap(..., DeploymentEnvironmentNameKey, DeploymentEnvironmentKey)` | `ConceptDeploymentEnv` | Phase 2 scope |
| 417 | Container ID | `GetFirstFromMap(..., ContainerIDKey, K8SPodUIDKey)` | `ConceptContainerID` | Phase 2 scope |
| 459 | Container ID | `GetFirstFromMap(..., ContainerIDKey, K8SPodUIDKey)` | `ConceptContainerID` | Phase 2 scope |
| 670 | Environment | `GetFirstFromMap(..., DeploymentEnvironmentNameKey, DeploymentEnvironmentKey)` | `ConceptDeploymentEnv` | Phase 2 scope |
| 677 | DB namespace | `GetOTelAttrValInResAndSpanAttrs(..., DBNamespaceKey)` | N/A | No concept yet |
| 740 | HTTP method | `GetFirstFromMap(..., "http.request.method", "http.method")` | `ConceptHTTPMethod` | Phase 2 scope |
| 742 | HTTP route | `GetFirstFromMap(..., HTTPRouteKey, "grpc.path")` | `ConceptHTTPRoute` | Phase 2 scope |
| 748 | Messaging dest | `GetFirstFromMap(..., MessagingDestinationKey, MessagingDestinationNameKey)` | `ConceptMessagingDest` | Phase 2 scope |

### `pkg/trace/otel/traceutil/otel_util.go` (V1 Functions)

The V1 functions were intentionally **not migrated** to preserve backwards compatibility:

| Line | Function | Current Lookup | Why Not Migrated |
|------|----------|----------------|------------------|
| 274-299 | `GetOTelResourceV1` | Multiple `GetOTelAttrValInResAndSpanAttrs` calls | **V1 uses resource-first precedence** |
| 493 | `GetOTelOperationNameV1` | `GetOTelAttrValInResAndSpanAttrs(..., "operation.name")` | V1 compatibility |
| 540 | Container tags loop | `GetOTelAttrVal(rattrs, false, key)` | Dynamic keys from config |

**Note:** The V1 functions (`GetOTelResourceV1`, `GetOTelOperationNameV1`) use `GetOTelAttrValInResAndSpanAttrs` which has **resource-first** precedence, opposite of the V2 functions. These were kept as-is to maintain backwards compatibility for users who haven't migrated to V2.

## Tests Documenting Inconsistencies

The following tests explicitly document these inconsistencies:

| Test | File | Documents |
|------|------|-----------|
| `TestFallbackInconsistency_ResourceVsSpanPrecedence` | `otel_util_test.go` | Span vs resource precedence |
| `TestFallbackInconsistency_MessagingDestinationPrecedence` | `otel_util_test.go` | OLD first for messaging |
| `TestFallbackInconsistency_DBStatementVsQueryText` | `otel_util_test.go` | OLD first for DB |
| `TestFallbackInconsistency_HTTPMethodPrecedence` | `otel_util_test.go` | NEW first for HTTP method |
| `TestFallbackInconsistency_Status2ErrorHTTPCodePrecedence` | `transform_test.go` | NEW first in Status2Error |

## Future Work

The semantic registry enables future migrations:
- Stats aggregation attribute lookups
- Obfuscation attribute detection
- Sampling decision attributes
- Peer tag extraction for APM features

### Recommended Future Cleanup

1. **Standardize precedence policy** - Decide on either "newer semconv first" or "older semconv first" across all concepts
2. **Migrate remaining functions** - Convert all hardcoded lookups in:
   - `transform.go` - Environment, version, container ID, Status2Error
   - `api/otlp.go` - Container ID, environment, HTTP method/route, messaging
   - `otel_util.go` - V1 functions (if V1 deprecation is planned)
3. **Update tests** - After standardizing, update `TestFallbackInconsistency_*` tests to verify consistent behavior
4. **Document breaking changes** - Any precedence changes should be documented in release notes
5. **Deprecate V1 functions** - Once V2 is stable, consider deprecating `GetOTelResourceV1` and `GetOTelOperationNameV1`

