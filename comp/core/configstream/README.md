# Config Stream Component

Streams configuration from core-agent to remote agents in real-time. Configuration is resolved once in the core-agent and pushed to all registered remote agents, ensuring consistency and enabling hot-reload without restarts.

## Overview

The config stream component provides:
- **Real-time config updates** from core-agent to remote agents
- **RAR-gated authorization** - only registered remote agents can subscribe
- **Snapshot-first delivery** - complete config state before incremental updates
- **Automatic resync** on gaps or disconnections
- **Origin tracking** - identifies which config file (e.g., `datadog.yaml`) changes came from

## How It Works

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
│    ConfigEvents│ (session_id in│   → Send snapshot       │
│                │    metadata)  │   → Push updates        │
└────────────────┘<──────────────└─────────────────────────┘
```

**Flow:**
1. Remote agent registers with RAR, receives `session_id`
2. Remote agent calls `StreamConfigEvents` with `session_id` in gRPC metadata
3. Core-agent validates `session_id` against RAR
4. Core-agent sends initial snapshot (all config settings)
5. Core-agent streams incremental updates as config changes

## Protocol Specification

### gRPC Service

```protobuf
service AgentSecure {
  rpc StreamConfigEvents(ConfigStreamRequest) returns (stream ConfigEvent);
}
```

### Messages

**Request:**
```protobuf
message ConfigStreamRequest {
  string name = 1;        // Client identifier (e.g., "system-probe")
  // NOTE: session_id is passed via gRPC metadata (key: "session_id"), not in the message body.
}
```

**Authentication:**
- The `session_id` obtained from RAR registration **must** be passed via gRPC metadata
- Metadata key: `"session_id"`
- This ensures consistent authentication across all remote agent RPCs

**Response Stream:**
```protobuf
message ConfigEvent {
  oneof event {
    ConfigSnapshot snapshot = 1;  // Sent first, then on resync
    ConfigUpdate update = 2;      // Incremental changes
  }
}

message ConfigSnapshot {
  string origin = 1;                  // Config file (e.g., "datadog.yaml")
  int32 sequence_id = 2;              // Monotonic sequence ID
  repeated ConfigSetting settings = 3; // All config settings
}

message ConfigUpdate {
  string origin = 1;        // Config file (e.g., "datadog.yaml")
  int32 sequence_id = 2;    // Monotonic sequence ID
  ConfigSetting setting = 3; // Single changed setting
}

message ConfigSetting {
  string source = 1;             // "file", "env-var", "remote-config", etc.
  string key = 2;                // Setting name
  google.protobuf.Value value = 3; // Typed value (string, int, bool, etc.)
}
```

## Configuration

The config stream is automatically enabled when the component is loaded. No explicit configuration required.

**Optional settings:**
```yaml
# datadog.yaml
config_stream:
  sleep_interval: 10ms  # Backoff on non-terminal errors (default: 10ms)

remote_agent_registry:
  enabled: true  # Required for RAR-gated authorization
```

## Telemetry

The component exports the following metrics for monitoring:

| Metric | Type | Description |
|--------|------|-------------|
| `configstream.subscribers` | Gauge | Number of active subscribers |
| `configstream.snapshots_sent` | Counter | Snapshots sent (including resyncs) |
| `configstream.updates_sent` | Counter | Incremental updates sent |
| `configstream.discontinuities` | Counter | Sequence gaps detected (triggers resync) |
| `configstream.dropped_updates` | Counter | Updates dropped (slow consumers) |

**Monitoring examples:**
```promql
# Check for slow consumers
rate(configstream_dropped_updates[5m]) > 0

# Monitor resync frequency (should be near zero)
rate(configstream_discontinuities[5m])

# Track active subscribers
configstream_subscribers
```

## Testing

### Unit Tests

```bash
cd comp/core/configstream/impl
go test -tags test -v

# Run specific test
go test -tags test -v -run TestClientConnectsAndReceivesStream
```

**Test coverage:**
- Snapshot-first delivery with origin field
- Ordered sequence IDs
- Correct typed values (string, int, bool, float)
- Multiple subscribers (fan-out)
- Discontinuity detection and resync
- Config layering and unsets
- RAR authorization

### Integration Test Client

Build and run the standalone test client:

```bash
# Build
go build -o bin/config-stream-client ./cmd/config-stream-client

# Run (requires running core-agent with RAR enabled)
./bin/config-stream-client \
  --ipc-address localhost:5001 \
  --auth-token $(cat /etc/datadog-agent/auth_token) \
  --name my-test-client \
  --duration 60s
```

**Note:** The test client performs actual RAR registration to obtain a valid `session_id`, validating the complete end-to-end authentication flow. See `cmd/config-stream-client/README.md` for detailed usage and limitations.

## Troubleshooting

### Client Cannot Subscribe

**Symptoms:**
```
rpc error: code = Unauthenticated desc = session_id required in metadata
```

**Solution:**
1. Ensure remote agent registers with RAR first using `RegisterRemoteAgent()`
2. Pass `session_id` from RAR registration via gRPC metadata (not in request body)
3. Check RAR is enabled: `remote_agent_registry.enabled: true`

Example:
```go
md := metadata.New(map[string]string{"session_id": sessionID})
ctx := metadata.NewOutgoingContext(context.Background(), md)
stream, err := client.StreamConfigEvents(ctx, &pb.ConfigStreamRequest{Name: "my-agent"})
```

### No Snapshot Received

**Symptoms:** Client connects but times out waiting for snapshot

**Solution:**
```bash
# Check core-agent logs
grep "configstream" /var/log/datadog/agent.log

# Look for:
# "New subscriber 'X' joining the config stream"
# "Failed to create config snapshot"
```

### Frequent Resyncs

**Symptoms:** Many `Discontinuity detected` warnings in logs

**Solution:**
- Check `configstream.dropped_updates` metric
- Optimize consumer processing
- Increase buffer if needed (code change)

### Authentication Failures

**Symptoms:**
```
rpc error: code = PermissionDenied desc = session_id 'xxx' not found
```

**Solution:**
This indicates the `session_id` is not recognized by the core-agent. Possible causes:

1. **Agent restarted** - RAR state was lost, client must re-register
2. **Session expired** - Registration timeout exceeded without refresh
3. **Registration failed** - Initial `RegisterRemoteAgent()` did not complete successfully

**Fix:** Ensure the remote agent calls `RefreshRemoteAgent()` periodically (every 30s recommended) to keep the session alive.

## Development

### Modifying the Protocol

1. **Edit proto:** `pkg/proto/datadog/model/v1/model.proto`
2. **Regenerate:**
   ```bash
   dda inv protobuf.generate
   ```
3. **Update implementation:** `comp/core/configstream/impl/configstream.go`
4. **Add tests:** `comp/core/configstream/impl/configstream_test.go`

### Debugging

Enable debug logging:

```yaml
# datadog.yaml
log_level: debug
```

**Key log messages:**
- `New subscriber 'X' joining the config stream` - Subscription started
- `Discontinuity detected for subscriber 'X'` - Gap found, sending snapshot
- `Dropping config update for subscriber 'X'` - Slow consumer, channel full
- `Config stream authorized for remote agent` - RAR auth succeeded

## Performance

**Benchmarks** (approximate, based on ~350 settings):
- Snapshot generation: ~1-2ms
- Update fan-out: ~10-50μs per subscriber
- Memory per subscriber: ~100KB (buffered channel)

## Security

- **mTLS:** All IPC communication encrypted
- **Bearer token:** Required for authentication
- **RAR authorization:** Only registered agents can subscribe
- **No config secrets:** Sensitive values handled by secrets backend

## Contact

- **Teams:** agent-metric-pipelines, agent-configuration
- **Component:** `comp/core/configstream`
- **Test Client:** `cmd/config-stream-client`
