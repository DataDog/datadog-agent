# Config Stream: Phase 0 Implementation

**Status:** ✅ Complete  
**Date:** January 2026

## Overview

Phase 0 establishes the producer-side infrastructure for streaming configuration from core-agent to remote agents via RAR (Remote Agent Registry).

## What Was Implemented

### 1. Config Event Bus with Sequencing

**Location:** `pkg/config/nodetreemodel/config.go`

Every config mutation increments `sequenceID` and emits notifications:

```go
c.sequenceID++
// Notify all receivers with (key, source, oldValue, newValue, sequenceID)
for _, receiver := range receivers {
    receiver(key, source, previousValue, newValue, c.sequenceID)
}
```

### 2. Config Stream Component

**Location:** `comp/core/configstream/impl/configstream.go`

- **Snapshot generation:** Consistent point-in-time view of all config
- **Update fan-out:** Broadcasts to all subscribers
- **Discontinuity detection:** Automatically resyncs clients that fall behind
- **Non-blocking:** Buffered channels (100 events), slow consumers don't block others

### 3. gRPC Service with RAR-Gating

**Proto:** `pkg/proto/datadog/model/v1/model.proto`

```protobuf
message ConfigStreamRequest {
  string name = 1;
  string session_id = 2;  // Required: from RAR registration
}

message ConfigEvent {
  oneof event {
    ConfigSnapshot snapshot = 1;  // Always sent first
    ConfigUpdate update = 2;      // Incremental changes
  }
}
```

**Service:** `AgentSecure.StreamConfigEvents` on IPC with mTLS

### 4. RAR-Gated Authorization

**Location:** `comp/core/configstream/server/server.go`

```go
func (s *Server) StreamConfigEvents(req *pb.ConfigStreamRequest, stream) error {
    // 1. Validate session_id is provided
    if req.SessionId == "" {
        return status.Error(codes.Unauthenticated, "session_id required")
    }
    
    // 2. Check RAR registry
    if !s.registry.RefreshRemoteAgent(req.SessionId) {
        return status.Errorf(codes.PermissionDenied, "session_id not found")
    }
    
    // 3. Stream config events
    eventsCh, unsubscribe := s.comp.Subscribe(req)
    defer unsubscribe()
    // ... send snapshot + updates
}
```

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

## Key Invariants

1. **Snapshot-first:** Every subscriber receives complete snapshot before updates
2. **Ordered delivery:** Sequence IDs are strictly increasing
3. **Automatic resync:** Gaps trigger snapshot resend
4. **RAR-gated:** Only registered remote agents can subscribe
5. **Single source of truth:** Config resolved once in core-agent

## Files Changed

### Core Implementation
- `pkg/config/nodetreemodel/config.go` - Sequence ID tracking
- `comp/core/configstream/impl/configstream.go` - Stream component
- `comp/core/configstream/server/server.go` - gRPC server with RAR auth
- `comp/api/grpcserver/impl-agent/grpc.go` - Server wiring

### Protocol
- `pkg/proto/datadog/model/v1/model.proto` - Added `session_id` field
- `pkg/proto/pbgo/core/model.pb.go` - Generated

### Testing
- `comp/core/configstream/impl/configstream_test.go` - Component tests
- `comp/core/configstream/server/server_test.go` - Server tests with mock RAR
- `cmd/config-stream-client/` - Test client

## Testing

All tests passing:

```bash
# Component tests (7 test cases)
cd comp/core/configstream/impl
go test -tags test -v

# Server tests (with RAR-gating)
cd comp/core/configstream/server
go test -tags test -v
```

Test coverage:
- ✅ Snapshot-first delivery
- ✅ Ordered sequence IDs
- ✅ Correct typed values
- ✅ Multiple subscribers
- ✅ Discontinuity resync
- ✅ Config layering and unsets
- ✅ RAR authorization
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

## Exit Criteria

| Criterion | Status |
|-----------|--------|
| Config event bus + sequencing | ✅ Complete |
| Snapshot creation | ✅ Complete |
| `AgentSecure.StreamConfigEvents` on IPC | ✅ Complete |
| RAR-gated authorization | ✅ Complete |
| Test client validation | ✅ Complete |

## Limitations (Phase 1+)

1. **No consumer library:** Remote agents must implement gRPC client manually
2. **No startup gating:** Can't block until config received
3. **No change notifications:** Can't subscribe to specific keys
4. **No origin filtering:** All config sent (datadog.yaml, system-probe.yaml, etc.)

## Next: Phase 1

Implement consumer library (`comp/core/configstreamconsumer`):

```go
type Consumer interface {
    Start(ctx) error              // Includes RAR registration
    WaitReady(ctx) error          // Block until snapshot
    Reader() model.Reader         // Config accessor
    Subscribe() <-chan ChangeEvent // Change notifications
}
```

Remote agents will use:
```go
consumer.WaitReady(ctx)  // Block startup
cfg := consumer.Reader() // Read config
```

---

**Phase 0 Complete:** Infrastructure ready for remote agent migration
