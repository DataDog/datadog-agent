# Config Streaming to RAR-Registered Remote Agents

## Vision

Stream configuration from core-agent to remote agents (system-probe, trace-agent, process-agent, security-agent, ADP) in real-time. Config is resolved **once** in core-agent and pushed to all processes, eliminating duplicate parsing and enabling updates without restarts.

## Architecture

### Producer (Core Agent)

```
datadog.yaml/env/RC/runtime
         ↓
   Config Resolver (single source of truth)
         ↓
   Config Event Bus (OnUpdate with sequence IDs)
         ↓
   Config Stream Component
         ↓
   gRPC IPC Server (mTLS + RAR-gated)
         ↓
    (fan-out to N remote agents)
```

### Consumer (Remote Agents)

```
Remote Agent (system-probe, trace-agent, etc.)
         ↓
   1. Register with RAR → session_id
         ↓
   2. Subscribe to config stream (with session_id)
         ↓
   3. Receive snapshot (complete state)
         ↓
   4. Receive updates (incremental changes)
         ↓
   5. Apply config via Consumer library
```

## Protocol

### Flow

1. **Client-initiated (Pull-to-Open)**
   - Remote agent registers with RAR
   - Gets `session_id`
   - Calls `StreamConfigEvents(session_id)`

2. **Server-pushed (Push-to-Deliver)**
   - Server validates `session_id` via RAR
   - Sends snapshot first
   - Pushes updates as config changes

### Messages

```protobuf
// Request
message ConfigStreamRequest {
  string name = 1;        // Client identifier
  string session_id = 2;  // RAR session (required)
}

// Response (stream)
message ConfigEvent {
  oneof event {
    ConfigSnapshot snapshot = 1;  // First message
    ConfigUpdate update = 2;      // Subsequent messages
  }
}

message ConfigSnapshot {
  int32 sequence_id = 1;
  repeated ConfigSetting settings = 2;
}

message ConfigUpdate {
  int32 sequence_id = 1;
  ConfigSetting setting = 2;
}

message ConfigSetting {
  string source = 1;  // File, EnvVar, AgentRuntime, etc.
  string key = 2;     // Config key
  google.protobuf.Value value = 3;  // Typed value
}
```

## Key Features

### RAR-Gated Authorization
- Only registered remote agents can subscribe
- Session ID validation prevents unauthorized access
- Automatic deregistration on idle timeout

### Snapshot-First Delivery
- Every subscriber gets complete config state before updates
- Ensures consistency even if updates missed

### Ordered Sequence IDs
- Updates delivered in strictly increasing order
- Gaps automatically detected and trigger resync

### Discontinuity Handling
- Client falls behind → automatic snapshot resend
- Prevents missing critical updates

### Non-Blocking Fan-Out
- Slow consumers don't block others
- Buffered channels with overflow protection

## Migration Phases

### ✅ Phase 0: Infrastructure (Complete)
- Config event bus + sequencing
- Snapshot generation
- gRPC service with RAR-gating
- Server-side complete

### 🚧 Phase 1: Consumer Library (Next)
Create `comp/core/configstreamconsumer`:
```go
type Consumer interface {
    Start(ctx) error              // Register + subscribe
    WaitReady(ctx) error          // Block until snapshot
    Reader() model.Reader         // Config accessor
    Subscribe() <-chan ChangeEvent
}
```

**Goal:** Drop-in for any remote agent

### Phase 2: Pilot Integration
- Pick one agent (system-probe)
- Behind feature flag: `use_rar_config_stream: true`
- Validate in production

### Phase 3: Migrate Remote Agents
- trace-agent
- process-agent
- security-agent
- ADP (Rust)

### Phase 4: Remove ConfigSync
- Remove pull-based config sync
- Stream-only architecture

### Phase 5: Hardening
- Reconnect strategy
- Telemetry & metrics
- Debug tooling (`agent config` shows "from stream")

## Benefits

### Performance
- Config resolved once (not N times)
- Reduced CPU and memory per agent
- Faster startup (no parsing)

### Consistency
- Single source of truth
- All agents see same config
- No drift between processes

### Operations
- Real-time config updates
- No restarts required
- Centralized config management

### Security
- mTLS encryption
- RAR authorization
- Session validation

## Configuration

```yaml
# datadog.yaml
remote_agent_registry:
  enabled: true
  recommended_refresh_interval: 30s
  idle_timeout: 60s

config_stream:
  sleep_interval: 10ms
```

## Usage (Phase 1+)

### Remote Agent Pattern

```go
import configstreamconsumer "github.com/DataDog/datadog-agent/comp/core/configstreamconsumer"

func run(cfgConsumer configstreamconsumer.Component) error {
    // Block until config received
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    if err := cfgConsumer.WaitReady(ctx); err != nil {
        return fmt.Errorf("config not received: %w", err)
    }
    
    // Use config
    cfg := cfgConsumer.Reader()
    logLevel := cfg.GetString("log_level")
    
    // React to changes
    for change := range cfgConsumer.Subscribe() {
        log.Infof("Config changed: %s = %v", change.Key, change.Value)
        // Apply change...
    }
    
    return startAgent()
}
```

### Current Pattern (Phase 0)

```go
// 1. Register with RAR
registerResp, err := client.RegisterRemoteAgent(ctx, &pb.RegisterRemoteAgentRequest{
    Pid:           os.Getpid(),
    Flavor:        "system-probe",
    DisplayName:   "System Probe",
    ApiEndpointUri: "localhost:6062",
    Services:      []string{"status", "flare"},
})

// 2. Subscribe with session_id
stream, err := client.StreamConfigEvents(ctx, &pb.ConfigStreamRequest{
    Name:      "system-probe",
    SessionId: registerResp.SessionId,
})

// 3. Receive events
for {
    event, err := stream.Recv()
    if err != nil {
        // Handle reconnect...
    }
    
    switch e := event.Event.(type) {
    case *pb.ConfigEvent_Snapshot:
        applySnapshot(e.Snapshot)
    case *pb.ConfigEvent_Update:
        applyUpdate(e.Update)
    }
}
```

## Monitoring

### Metrics (Planned)
- `config_stream.subscribers` - Number of connected agents
- `config_stream.snapshot_size_bytes` - Snapshot payload size
- `config_stream.updates_sent_total` - Total updates sent
- `config_stream.discontinuities_total` - Resync count
- `config_stream.authorization_errors_total` - Failed auth attempts

### Logs
- Subscriber connect/disconnect
- Authorization events
- Discontinuity detection
- Slow consumer warnings

## Testing

### Unit Tests
```bash
cd comp/core/configstream/impl
go test -tags test -v  # Component tests

cd comp/core/configstream/server
go test -tags test -v  # Server tests with RAR mock
```

### Integration Test
```bash
go build -o bin/config-stream-client ./cmd/config-stream-client
./bin/config-stream-client --ipc-address localhost:5001
```

### E2E (Future)
- Start core-agent
- Start remote agents with consumer library
- Verify config propagation
- Test updates without restart

## Components

| Component | Location | Purpose |
|-----------|----------|---------|
| Config resolver | `pkg/config/nodetreemodel` | Single source of truth |
| Event bus | `pkg/config/nodetreemodel` | Sequence tracking |
| Stream component | `comp/core/configstream/impl` | Fan-out logic |
| gRPC server | `comp/core/configstream/server` | Transport + RAR auth |
| Consumer library | `comp/core/configstreamconsumer` (Phase 1) | Client-side |
| RAR | `comp/core/remoteagentregistry` | Identity + lifecycle |

## Security Model

1. **mTLS:** All IPC communication encrypted
2. **Bearer token:** IPC authentication
3. **RAR registration:** Identity establishment
4. **Session validation:** Per-stream authorization
5. **Automatic expiry:** Idle sessions cleaned up

## Troubleshooting

| Issue | Cause | Solution |
|-------|-------|----------|
| `session_id required` | No RAR registration | Register first |
| `session_id not found` | Not registered or expired | Re-register |
| `Discontinuity detected` | Rapid updates | Expected, automatic resync |
| `Channel full` | Slow consumer | Increase buffer or optimize |

## Status

**Current:** Phase 0 Complete ✅  
**Next:** Phase 1 - Consumer Library  
**Target:** Full migration by 2026 Q2

---

**Contacts:**
- Team: agent-runtimes, agent-configuration
- Component: `comp/core/configstream`
- RFC: RAR Config Streaming (internal)
