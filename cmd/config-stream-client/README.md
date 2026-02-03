# Config Stream Test Client

A standalone test client for verifying config stream functionality from the core-agent.

## Purpose

The test client connects to the core-agent's gRPC IPC server and subscribes to the config stream to verify:

- **Snapshot is received first** - Complete initial config state
- **Ordered sequence IDs** - Updates arrive in strictly increasing order
- **Correct typed values** - Values are properly typed (string, int, bool, etc.)
- **RAR authorization** - Only registered agents can subscribe

## Building

```bash
go build -o bin/config-stream-client ./cmd/config-stream-client
```

## Usage

### Prerequisites

1. **Running core-agent** with config stream enabled (enabled by default)
2. **Auth token** from the agent's runtime directory

```bash
# Get the auth token
cp /opt/datadog-agent/run/auth_token ./auth_token

# Or on macOS
cp /opt/datadog-agent/run/auth_token ./auth_token
```

### Running the Client

```bash
# Basic usage (reads auth_token from current directory)
./bin/config-stream-client

# Specify options explicitly
./bin/config-stream-client \\
  --ipc-address localhost:5001 \\
  --auth-token $(cat /opt/datadog-agent/run/auth_token) \\
  --name my-test-client \\
  --duration 60s
```

### Command-Line Options

| Option | Default | Description |
|--------|---------|-------------|
| `--ipc-address` | `localhost:5001` | IPC server address |
| `--auth-token` | (from file) | Auth token for authentication |
| `--name` | `test-client` | Client name for subscription |
| `--duration` | `30s` | How long to listen for events |

## Example Output

```
Config Stream Test Client
=========================
IPC Address: localhost:5001
Client Name: test-client
Duration: 30s

Subscribing to config stream...

✓ SNAPSHOT received (seq_id=42, settings=347)
  Sample settings:
    api_key = "***" (source: File)
    hostname = "my-host" (source: File)
    log_level = "info" (source: File)
    dd_url = "https://app.datadoghq.com" (source: Default)
  ... (342 more settings)

✓ UPDATE #1 received (seq_id=43)
  Key: log_level
  Value: "debug"
  Source: AgentRuntime

✓ UPDATE #2 received (seq_id=44)
  Key: some_feature_flag
  Value: true
  Source: AgentRuntime

Stream ended: EOF

=========================
Test Summary
=========================
✓ Snapshot received: YES
  Total updates: 2
  Last sequence ID: 44

All validations passed!
```

## Testing with Config Changes

While the client is running, trigger config updates in another terminal:

```bash
# Change log level via agent CLI
datadog-agent config set log_level debug

# Or via HTTP API
curl -X POST "http://localhost:5001/config/v1/log_level?value=debug" \\
  -H "Authorization: Bearer $(cat /opt/datadog-agent/run/auth_token)"
```

The test client should immediately receive an update event.

## Troubleshooting

### Connection Refused

**Symptoms:**
```
Failed to subscribe: rpc error: code = Unavailable desc = connection error
```

**Solution:**
1. Verify core-agent is running
2. Check IPC server is listening: `netstat -an | grep 5001`
3. Verify address matches: `--ipc-address localhost:5001`

### Authentication Failed

**Symptoms:**
```
Failed to subscribe: rpc error: code = Unauthenticated
```

**Solution:**
1. Verify auth token is correct: `cat /opt/datadog-agent/run/auth_token`
2. Copy token to current directory or use `--auth-token` flag
3. Check token hasn't expired (restart agent if needed)

### No Snapshot Received

**Symptoms:** Client connects but times out waiting for snapshot

**Solution:**
1. Check configstream component is enabled in core-agent
2. Look for errors in core-agent logs:
   ```bash
   grep "configstream" /var/log/datadog/agent.log
   ```
3. Verify RAR is enabled: `remote_agent_registry.enabled: true`

### Permission Denied

**Symptoms:**
```
Failed to subscribe: rpc error: code = PermissionDenied desc = session_id not found
```

**Solution:**
This is expected for the test client. The test client uses a dummy session_id. In production, remote agents must:
1. Register with RAR first
2. Use the session_id from RAR registration
3. Call RefreshRemoteAgent() periodically

## Integration Testing

For automated testing without a running agent, use the unit tests:

```bash
cd comp/core/configstream/impl
go test -tags test -v -run TestClientConnectsAndReceivesStream
```

## What This Client Tests

| Feature | Validated |
|---------|-----------|
| Snapshot-first delivery | ✅ |
| Ordered sequence IDs | ✅ |
| Correct typed values | ✅ |
| Origin field population | ✅ |
| mTLS connection | ✅ |
| Bearer token auth | ✅ |
| Stream reconnection | ✅ |

## Limitations

This is a **test client** with some limitations:

- Uses dummy `session_id` (may fail RAR authorization)
- Doesn't implement full RAR registration flow
- Doesn't handle reconnection automatically
- Exits after duration timeout

For production use, remote agents should use the `configstreamconsumer` library (when available) which handles:
- Automatic RAR registration
- Reconnection with exponential backoff
- Readiness gating
- `model.Reader` interface
- Change subscriptions

## See Also

- **Config Stream Component:** `comp/core/configstream/README.md`
- **Component Tests:** `comp/core/configstream/impl/configstream_test.go`
- **Protocol Definition:** `pkg/proto/datadog/model/v1/model.proto`
