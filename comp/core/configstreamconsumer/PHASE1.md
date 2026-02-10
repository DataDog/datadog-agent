# Config Stream Consumer Component

## Overview

The Config Stream Consumer Component delivers a shared Go library (`configstreamconsumer`) that remote agents can use to consume configuration streams from the core agent. This eliminates the need for each remote agent to implement their own gRPC plumbing, sequencing logic, snapshot gating, and update notification mechanics.

## Architecture: Producer vs. Consumer

The config streaming system uses a **producer-consumer pattern** across process boundaries:

```
┌─────────────────────────┐          ┌─────────────────────────┐
│   Core Agent Process    │          │ System-Probe Process    │
│                         │          │                         │
│  ┌──────────────────┐   │          │  ┌──────────────────┐   │
│  │  configstream    │   │  gRPC    │  │configstream-     │   │
│  │  (producer)      │◄──┼──────────┼─►│consumer          │   │
│  │                  │   │  stream  │  │  (client)        │   │
│  └──────────────────┘   │          │  └──────────────────┘   │
│                         │          │                         │
└─────────────────────────┘          └─────────────────────────┘
         ONE                                  MANY
              │
              └──────────────┬──────────────┐
                             │              │
                        ┌────▼────┐    ┌───▼────┐
                        │trace-   │    │process-│
                        │agent    │    │agent   │
                        │         │    │        │
                        │configstr│    │configstr│
                        │consumer │    │consumer │
                        └─────────┘    └────────┘
```

**Why Separate Components?**

- **configstream** (producer): Runs in core agent, manages fan-out to N clients
- **configstreamconsumer** (consumer): Reusable library for all remote agents
- Clear process boundaries and independent testing
- Each side evolves independently while respecting the gRPC contract

## Implementation

### Component Structure

The `configstreamconsumer` component is located at `comp/core/configstreamconsumer/` and follows the standard Datadog Agent component structure:

```
comp/core/configstreamconsumer/
├── def/
│   └── component.go          # Component interface definition
├── fx/
│   └── fx.go                  # FX module definition
├── impl/
│   ├── consumer.go            # Core consumer implementation
│   ├── reader.go              # Config reader implementing model.Reader
│   └── consumer_test.go       # Comprehensive tests
├── mock/
│   └── mock.go                # Mock implementation for testing
├── PHASE1.md                  # This document
└── README.md                  # Component overview
```

### Key Features

#### 1. **Stream Management**
- Establishes and maintains gRPC connection to core agent
- Handles automatic reconnection with exponential backoff
- Uses mTLS and bearer token authentication via IPC component
- Supports RAR (Remote Agent Registry) session-based authorization

#### 2. **Config Reader (model.Reader)**
- Provides a `model.Reader` interface backed by streamed configuration
- Thread-safe read access to effective configuration
- Supports all standard config operations (GetString, GetInt, GetBool, etc.)
- Returns correct sequence IDs for config versioning

#### 3. **Readiness Gating**
- `WaitReady(ctx)` blocks until first snapshot is received
- Ensures remote agents have consistent config before starting
- Timeout support via context cancellation
- Metrics track time to first snapshot

#### 4. **Ordered Update Application**
- Applies config snapshots and updates in sequence ID order
- Detects and drops stale updates (seq_id <= last_seq_id)
- Detects discontinuities and waits for server resync
- Guarantees consistency with core agent's config state

#### 5. **Change Subscription**
- Optional subscription to config change events
- Non-blocking event delivery with buffered channels
- Multiple concurrent subscribers supported
- Automatic cleanup via unsubscribe function

#### 6. **Telemetry Metrics**
- `configstream_consumer.time_to_first_snapshot_seconds` - Time to receive first snapshot
- `configstream_consumer.reconnect_count` - Number of stream reconnections
- `configstream_consumer.last_sequence_id` - Last received config sequence ID
- `configstream_consumer.dropped_stale_updates` - Number of stale updates dropped
- `configstream_consumer.buffer_overflow_disconnects` - Disconnects due to buffer overflow

### Usage Example

```go
import (
    configstreamconsumer "github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/def"
    configstreamconsumerimpl "github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/impl"
)

// In your remote agent's FX module
func Module() fx.Option {
    return fx.Module("myremoteagent",
        // ... other dependencies ...
        configstreamconsumer.Module(),
        fx.Provide(func() configstreamconsumerimpl.Params {
            return configstreamconsumerimpl.Params{
                ClientName:       "my-remote-agent",
                CoreAgentAddress: net.JoinHostPort(cfg.GetString("cmd_host"), cfg.GetString("cmd_port")),
                SessionID:        rarSessionID, // From RAR registration
            }
        }),
    )
}

// In your remote agent's run function
func run(consumer configstreamconsumer.Component, ...) error {
    // Block until config is ready
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    if err := consumer.WaitReady(ctx); err != nil {
        return fmt.Errorf("config not ready: %w", err)
    }
    
    // Use the config reader
    cfg := consumer.Reader()
    port := cfg.GetInt("my_agent.port")
    enabled := cfg.GetBool("my_agent.enabled")
    
    // Optional: subscribe to config changes
    changes, unsubscribe := consumer.Subscribe()
    defer unsubscribe()
    
    go func() {
        for change := range changes {
            log.Infof("Config changed: %s = %v (was %v)", 
                change.Key, change.NewValue, change.OldValue)
        }
    }()
    
    // ... start your agent ...
}
```

### Proto Structure

The consumer uses the following protobuf messages (from `pkg/proto/datadog/model/v1/model.proto`):

```protobuf
message ConfigStreamRequest {
  string name = 1;        // Client name (e.g., "system-probe")
  // NOTE: session_id is passed via gRPC metadata (key: "session_id"), not in the message body.
}

message ConfigSetting {
  string source = 1;
  string key = 2;
  google.protobuf.Value value = 3;
}

message ConfigSnapshot {
  string origin = 1;
  int32 sequence_id = 2;
  repeated ConfigSetting settings = 3;
}

message ConfigUpdate {
  string origin = 1;
  int32 sequence_id = 2;
  ConfigSetting setting = 3;
}

message ConfigEvent {
  oneof event {
    ConfigSnapshot snapshot = 1;
    ConfigUpdate update = 2;
  }
}
```

### Authentication

The consumer authenticates with the core agent using the RAR session ID:

1. **SessionID Parameter**: The consumer requires a `SessionID` parameter (obtained from RAR registration)
2. **gRPC Metadata**: The consumer automatically adds the session_id to gRPC metadata when establishing the stream:
   ```go
   md := metadata.New(map[string]string{"session_id": sessionID})
   ctxWithMetadata := metadata.NewOutgoingContext(ctx, md)
   stream, err := client.StreamConfigEvents(ctxWithMetadata, request)
   ```
3. **Server Verification**: The core agent's configstream server verifies the session_id and ensures the client is a registered remote agent

### Testing

Comprehensive tests verify:
- ✅ Snapshot reception and application
- ✅ Ordered update application with sequence IDs
- ✅ Stale update detection and dropping
- ✅ Change event subscription and notification
- ✅ Config reader functionality (all getter methods)
- ✅ Readiness gating (WaitReady blocks until snapshot)
- ✅ Multiple subscribers
- ✅ Discontinuity detection

Run tests:
```bash
cd comp/core/configstreamconsumer/impl
go test -tags test -v
```

## Phase 1 Exit Criteria

✅ **All exit criteria met:**

1. ✅ Consumer component can start and establish connection
2. ✅ Consumer blocks startup until first snapshot is received
3. ✅ Consumer applies config updates in order
4. ✅ Consumer provides model.Reader interface for config access
5. ✅ Change subscription mechanism works
6. ✅ Telemetry metrics are emitted
7. ✅ Comprehensive tests pass

## Integration Steps for Remote Agents

To integrate a remote agent with the config stream consumer (Phase 2+):

1. **Add FX dependency**: Include `configstreamconsumer.Module()` in your agent's FX options
2. **Provide parameters**: Supply `ClientName`, `CoreAgentAddress`, and `SessionID` (from RAR)
3. **Block on WaitReady**: Call `consumer.WaitReady(ctx)` before starting your agent
4. **Use the Reader**: Replace local config with `consumer.Reader()`
5. **Feature flag**: Guard with `use_rar_config_stream` feature flag for gradual rollout

## Next Steps: Phase 2+

### Phase 2: Integrate into One Remote Agent (Pilot)

**Goal**: Prove the pattern works end-to-end with one remote agent behind a feature flag.

**Recommended pilot**: `system-probe` (since you're already working on it)

**Tasks**:
1. **Add configstreamconsumer dependency**
   - Import `configstreamconsumer.Module()` in system-probe's FX module
   - Wire up the component alongside existing config component

2. **Implement feature flag**: `use_rar_config_stream`
   ```yaml
   # datadog.yaml or system-probe.yaml
   use_rar_config_stream: false  # default: off for safety
   ```

3. **Conditional initialization**
   - If flag enabled: initialize configstreamconsumer, get SessionID from RAR
   - If flag disabled: use existing local config behavior (backward compatibility)

4. **Update system-probe run()**
   ```go
   func run(cfg config.Component, consumer configstreamconsumer.Component, ...) error {
       var effectiveConfig model.Reader
       
       if cfg.GetBool("use_rar_config_stream") {
           // Stream mode: block until snapshot
           ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
           defer cancel()
           if err := consumer.WaitReady(ctx); err != nil {
               return fmt.Errorf("config stream not ready: %w", err)
           }
           effectiveConfig = consumer.Reader()
           log.Info("Using streamed config from core agent")
       } else {
           // Legacy mode: use local config
           effectiveConfig = cfg.Object()
           log.Info("Using local config (legacy mode)")
       }
       
       return startSystemProbe(effectiveConfig, ...)
   }
   ```

5. **Testing**
   - Manual testing: enable flag, verify system-probe starts and receives config
   - Verify metrics: `configstream_consumer.time_to_first_snapshot_seconds` should be <1s
   - Test config updates: change a setting in core agent, verify system-probe sees it
   - Test reconnection: restart core agent, verify system-probe reconnects

6. **Gradual rollout**
   - Internal dogfooding with flag enabled
   - Monitor for issues (startup failures, config drift, memory leaks)
   - If stable: enable by default in one release, remove flag in next

**Exit criteria**:
- ✅ System-probe starts successfully with stream mode enabled
- ✅ Config values match between core agent and system-probe
- ✅ Updates propagate within <100ms
- ✅ Reconnection works after core agent restart
- ✅ No memory leaks or goroutine leaks
- ✅ Telemetry metrics look healthy

---

### Phase 3: Migrate Remaining Remote Agents

**Goal**: Roll out to all other remote agents once system-probe is stable.

**Agents to migrate**:
- `trace-agent` (APM)
- `process-agent` (process monitoring)
- `security-agent` (CSM/CWS)
- Any other RAR-registered agents

**Tasks per agent**:
1. Copy the Phase 2 pattern (feature flag + conditional initialization)
2. Replace `cfg.Object()` calls with `effectiveConfig` from consumer
3. Add integration tests
4. Gradual rollout with monitoring

**Timeline**: ~1-2 sprints after Phase 2 stabilizes

**Exit criteria**:
- ✅ All remote agents can run in stream mode
- ✅ At least one release where both modes coexist
- ✅ Production metrics show no regressions

---

## Related Documentation

- Phase 0 changes: `comp/core/configstream/PHASE0.md`
- Project overview: `comp/core/configstream/PROJECT.md`
- Component README: `comp/core/configstreamconsumer/README.md`
