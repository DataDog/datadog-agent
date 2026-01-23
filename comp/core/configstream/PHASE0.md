# Config Stream: Phase 0 Implementation

**Status:** ✅ Complete  
**Date:** January 2026  
**Base:** Built on top of [PR #39877](https://patch-diff.githubusercontent.com/raw/DataDog/datadog-agent/pull/39877.diff)

## Overview

Phase 0 establishes the producer-side infrastructure for streaming configuration from core-agent to remote agents via RAR (Remote Agent Registry). This implementation builds on the foundation laid by PR #39877, adding missing pieces for production readiness.

## What PR #39877 Provided

PR #39877 implemented the core streaming infrastructure:

### 1. **Config Event Bus with Sequencing**
- Location: `pkg/config/nodetreemodel/config.go`
- Increments `sequenceID` on every config mutation
- Emits notifications via `OnUpdate` callback

### 2. **Config Stream Component**
- Location: `comp/core/configstream/impl/configstream.go`
- Snapshot generation with consistent point-in-time view
- Update fan-out to all subscribers via channels
- Discontinuity detection and automatic resync
- Non-blocking subscriber management (buffered channels)

### 3. **gRPC Service**
- Proto: `pkg/proto/datadog/model/v1/model.proto`
- Service: `AgentSecure.StreamConfigEvents` on IPC with mTLS
- Request/response messages defined

### 4. **RAR-Gated Authorization** 
- Location: `comp/core/configstream/server/server.go`
- Validates `session_id` from `ConfigStreamRequest`
- Uses `RemoteAgentRegistry.RefreshRemoteAgent()` to authorize

### 5. **Comprehensive Testing**
- Component tests in `configstream_test.go`
- Server tests with mock RAR
- Test client in `cmd/config-stream-client/`

## What We Added (Phase 0 Refinements)

### 1. **Origin Field Population**

**Problem:** Proto defined `origin` field in `ConfigSnapshot` and `ConfigUpdate`, but it was never populated.

**Solution:** Added `getConfigOrigin()` helper and populated origin in both snapshot and update creation.

```go
// Get the config origin (filename without path)
func (cs *configStream) getConfigOrigin() string {
    configFile := cs.config.ConfigFileUsed()
    if configFile == "" {
        return "core-agent" // Fallback
    }
    return filepath.Base(configFile)
}
```

**Changes:**
- `comp/core/configstream/impl/configstream.go:284-293` - Added `getConfigOrigin()` method
- `comp/core/configstream/impl/configstream.go:262` - Populate origin in snapshots
- `comp/core/configstream/impl/configstream.go:120` - Populate origin in updates

### 2. **Telemetry Metrics**

**Problem:** No observability into streaming behavior (subscribers, updates sent, discontinuities, etc.)

**Solution:** Added comprehensive telemetry metrics for monitoring.

**Metrics:**
- `configstream.subscribers` (Gauge) - Number of active subscribers
- `configstream.snapshots_sent` (Counter) - Snapshots sent (including resyncs)
- `configstream.updates_sent` (Counter) - Incremental updates sent
- `configstream.discontinuities` (Counter) - Gaps detected and resynced
- `configstream.dropped_updates` (Counter) - Updates dropped due to full channels

**Changes:**
- `comp/core/configstream/impl/configstream.go:38-53` - Added telemetry fields to `configStream` struct
- `comp/core/configstream/impl/configstream.go:58-68` - Initialize metrics in `NewComponent`
- `comp/core/configstream/impl/configstream.go:178-182` - Track subscriber additions
- `comp/core/configstream/impl/configstream.go:195-198` - Track subscriber removals
- `comp/core/configstream/impl/configstream.go:228-241` - Track updates, discontinuities, and drops

### 3. **Test Enhancements**

**Problem:** Tests didn't verify origin field or telemetry behavior.

**Solution:** Added assertions for origin field in existing tests.

**Changes:**
- `comp/core/configstream/impl/configstream_test.go:11` - Import telemetry noop component
- `comp/core/configstream/impl/configstream_test.go:316-333` - Updated test helper to include telemetry
- `comp/core/configstream/impl/configstream_test.go:65` - Assert origin field in snapshot test
- `comp/core/configstream/impl/configstream_test.go:130-132` - Assert origin field in update test

## Architecture

```
Remote Agent                      Core Agent
┌────────────────┐               ┌─────────────────────────┐
│ 1. Register    │──────────────>│ RAR: Get session_id     │
│    with RAR    │<──────────────│                         │
└────────────────┘               └─────────────────────────┘
        │
        │ session_id
        ▼
┌────────────────┐               ┌─────────────────────────┐
│ 2. Stream      │──────────────>│ Server: Validate RAR    │
│    ConfigEvents│  + session_id │   → Send snapshot       │
│                │<──────────────│   → Push updates        │
└────────────────┘               └─────────────────────────┘

Pull-to-Open                      Push-to-Deliver
(client connects)                 (server streams)
```

## Protocol Specification

**Request:**
```protobuf
message ConfigStreamRequest {
  string name = 1;        // Client identifier
  string session_id = 2;  // From RAR registration (required)
}
```

**Response Stream:**
```protobuf
message ConfigEvent {
  oneof event {
    ConfigSnapshot snapshot = 1;  // Always sent first
    ConfigUpdate update = 2;      // Incremental changes
  }
}

message ConfigSnapshot {
  string origin = 1;              // e.g., "datadog.yaml"
  int32 sequence_id = 2;          // Monotonic ID
  repeated ConfigSetting settings = 3;
}

message ConfigUpdate {
  string origin = 1;              // e.g., "datadog.yaml"
  int32 sequence_id = 2;          // Monotonic ID  
  ConfigSetting setting = 3;      // Single changed setting
}

message ConfigSetting {
  string source = 1;              // e.g., "file", "env-var", "remote-config"
  string key = 2;                 // Setting name
  google.protobuf.Value value = 3;
}
```

## Key Invariants

1. **Snapshot-first:** Every subscriber receives complete snapshot before updates
2. **Ordered delivery:** Sequence IDs are strictly increasing per subscriber
3. **Automatic resync:** Gaps trigger snapshot resend (logged as warnings)
4. **RAR-gated:** Only registered remote agents can subscribe
5. **Single source of truth:** Config resolved once in core-agent
6. **Origin tracking:** Every event identifies its config source file

## Files Modified

### Core Implementation
- ✅ `comp/core/configstream/impl/configstream.go` - Added origin + telemetry
- ✅ `comp/core/configstream/server/server.go` - (Already had RAR-gating from PR)

### Testing
- ✅ `comp/core/configstream/impl/configstream_test.go` - Added origin assertions + telemetry support

### No Changes Needed
- `pkg/proto/datadog/model/v1/model.proto` - (Already had origin field from PR)
- `comp/core/configstream/server/server.go` - (Already had RAR auth from PR)
- `comp/api/grpcserver/impl-agent/grpc.go` - (Already wired from PR)

## Testing

All tests passing:

```bash
# Run Phase 0 exit criteria tests
cd comp/core/configstream/impl
go test -tags test -v -run TestPhase0ExitCriteria

# Run all component tests (4 test suites, 10 test cases)
go test -tags test -v

# Run server tests (with RAR-gating)
cd ../server
go test -tags test -v
```

**Test Coverage:**
- ✅ Snapshot-first delivery with origin field
- ✅ Ordered sequence IDs
- ✅ Correct typed values (string, int, bool, float)
- ✅ Multiple subscribers (fan-out)
- ✅ Discontinuity detection and resync
- ✅ Config layering and unsets
- ✅ RAR authorization (session_id validation)
- ✅ Unsubscribe cleanup

## Configuration

```yaml
# datadog.yaml
remote_agent_registry:
  enabled: true
  recommended_refresh_interval: 30s
  idle_timeout: 60s

config_stream:
  sleep_interval: 10ms  # Backpressure on non-terminal errors
```

## Telemetry Usage

Monitor stream health with:

```promql
# Number of active subscribers
configstream_subscribers

# Rate of snapshots sent (including resyncs)
rate(configstream_snapshots_sent[5m])

# Rate of incremental updates
rate(configstream_updates_sent[5m])

# Discontinuities detected (should be rare)
rate(configstream_discontinuities[5m])

# Dropped updates (indicates slow consumers)
rate(configstream_dropped_updates[5m])
```

## Exit Criteria

| Criterion | Status |
|-----------|--------|
| Config event bus + sequencing | ✅ Complete (PR #39877) |
| Snapshot creation | ✅ Complete (PR #39877) |
| `AgentSecure.StreamConfigEvents` on IPC | ✅ Complete (PR #39877) |
| RAR-gated authorization | ✅ Complete (PR #39877) |
| Origin field populated | ✅ **Complete (This PR)** |
| Telemetry metrics | ✅ **Complete (This PR)** |
| Test client validation | ✅ Complete (PR #39877) |

## Limitations (Addressed in Phase 1+)

Phase 0 provides server-side streaming infrastructure. Future phases add:

1. **Phase 1:** Consumer library for remote agents
2. **Phase 2:** Integrate into one pilot agent (e.g., system-probe)
3. **Phase 3:** Migrate all remote agents
4. **Phase 4:** Remove legacy `configsync`

### What's Missing for Remote Agents

- **No consumer library:** Remote agents must implement gRPC client manually
- **No startup gating:** Can't block until config received
- **No change notifications:** Can't subscribe to specific keys
- **No `model.Reader` implementation:** Can't use familiar config interface

## Next: Phase 1 - Consumer Library

Implement `comp/core/configstreamconsumer` to provide:

```go
type Component interface {
    Start(ctx) error              // Initiates stream connection
    WaitReady(ctx) error          // Blocks until snapshot received
    Reader() model.Reader         // Config accessor (familiar interface)
    Subscribe() <-chan ChangeEvent // Change notifications
}
```

**Remote agents will use:**
```go
// In main()
consumer.Start(ctx)
consumer.WaitReady(ctx)  // Block startup until config ready

// In application code
cfg := consumer.Reader()
apiKey := cfg.GetString("api_key")

// For hot-reloading
changes, unsubscribe := consumer.Subscribe()
for change := range changes {
    log.Infof("Config changed: %s = %v", change.Key, change.NewValue)
}
```

---

**Phase 0 Status:** ✅ Complete  
**Foundation:** Server-side streaming infrastructure ready for consumer integration
