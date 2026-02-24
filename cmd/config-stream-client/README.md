# Config Stream Test Client

A standalone test client for verifying config stream functionality from the core-agent.

## Purpose

The test client connects to the core-agent's gRPC IPC server and subscribes to the config stream to verify:

- **Snapshot is received first** - Complete initial config state
- **Ordered sequence IDs** - Updates arrive in strictly increasing order
- **Correct typed values** - Values are properly typed (string, int, bool, etc.)
- **RAR authorization** - Only registered agents can subscribe

## Note on Authentication

This client performs actual Remote Agent Registry (RAR) registration to obtain a valid `session_id`, as required by the config streaming authentication model. While lightweight and designed for testing, it is technically a registered remote agent during its runtime. The client:

1. Registers with RAR using minimal metadata (PID, flavor, display name)
2. Receives a valid `session_id` from the core-agent
3. Uses that `session_id` in gRPC metadata to authenticate with the config stream
4. Automatically unregisters when the test completes (via context timeout)

This ensures the test client validates the complete end-to-end authentication flow that real remote agents (like Saluki ADP) will use.

## Building

```bash
go build -o bin/config-stream-client ./cmd/config-stream-client
```

## Usage

### Prerequisites

1. **Running core-agent** with config stream enabled (enabled by default)
2. **Auth token** from the agent's runtime directory

```bash
chmod 777 bin/agent/dist/auth_token
cp bin/agent/dist/auth_token /etc/datadog-agent/auth_token
```

### Running the Client

```bash
# Basic usage (reads auth_token from current directory)
./bin/config-stream-client

# Specify options explicitly
./bin/config-stream-client \
  --ipc-address localhost:5001 \
  --auth-token $(cat /etc/datadog-agent/auth_token) \
  --name my-test-client \
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

Registering with Remote Agent Registry...
Successfully registered. Session ID: X

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

### Method: Using Agent CLI (Recommended)

The easiest way to trigger config updates is using the `agent config set` command:

```bash
# List available runtime-configurable settings
datadog-agent config list-runtime

# Change a setting (e.g., log_level)
datadog-agent config set log_level debug
```

**Note**: You must run this command while the agent is running. The command connects to the running agent via IPC to change the setting.

### Verifying Updates

When a config update is successfully triggered, you should see output in your stream client like:

```
✓ UPDATE #1 received (seq_id=4)
  Key: log_level
  Value: debug
  Source: cli
```

### Common Issues

- **"Setting not found"**: The setting name might not be registered for runtime changes. Use `agent config list-runtime` to see available settings
- **"No update received"**: Verify the setting change was successful and that the configstream component is enabled
- **"Connection refused"**: Make sure the agent is running and the IPC server is accessible

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
1. Verify auth token is correct: `cat /etc/datadog-agent/auth_token`
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
This indicates the client's session_id is not recognized by the core-agent. Possible causes:
1. **Agent restarted** - RAR state was lost, client needs to re-register
2. **Session expired** - Registration timeout exceeded (check agent logs)
3. **Client registration failed** - Check that RAR registration completed successfully before subscribing to config stream

The test client automatically handles registration, but if you see this error, the core-agent may have been restarted since the client registered.

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

This is a **test client** with some limitations compared to production remote agents:

- **No session refresh** - Does not call `RefreshRemoteAgent()` periodically
- **No reconnection logic** - Doesn't handle network failures or agent restarts
- **Fixed duration** - Exits after timeout (not designed for long-running use)
- **Minimal services** - Registers with RAR but provides no actual services (Status, Flare, etc.)

For production use, remote agents should implement:
- Periodic session refresh via `RefreshRemoteAgent()`
- Automatic reconnection with exponential backoff
- Full service implementation (Status, Flare, Telemetry)
- Graceful shutdown and session cleanup

## See Also

- **Config Stream Component:** `comp/core/configstream/README.md`
- **Component Tests:** `comp/core/configstream/impl/configstream_test.go`
- **Protocol Definition:** `pkg/proto/datadog/model/v1/model.proto`
