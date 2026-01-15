# Config Stream Test Client

This is a test client to verify **Phase 0** of the RAR Config Streaming implementation.

## Purpose

The Config Stream Test Client connects to the core-agent's gRPC IPC server and subscribes to the config stream. It verifies that:

1. ✅ **Snapshot is received first** - The initial state of all config settings
2. ✅ **Ordered sequence IDs** - Updates arrive in strictly increasing sequence ID order
3. ✅ **Correct typed values** - Values are properly typed (string, int, bool, etc.)

## Building

```bash
cd /Users/rahul.kaukuntla/go/src/github.com/DataDog/datadog-agent
go build -o bin/config-stream-client ./cmd/config-stream-client
```

## Usage

### Prerequisites

1. **Start the core-agent** with the config stream enabled (it should be enabled by default if the component is wired up)
2. **Get the auth token** - The client needs the auth token to authenticate with the IPC server

   ```bash
   # Copy the auth token from the agent's runtime directory
   cp /opt/datadog-agent/run/auth_token ./auth_token
   
   # Or on macOS:
   cp /opt/datadog-agent/run/auth_token ./auth_token
   ```

### Running the Test Client

```bash
# Basic usage (reads auth_token from current directory)
./bin/config-stream-client

# Specify IPC address and auth token explicitly
./bin/config-stream-client \
  --ipc-address localhost:5001 \
  --auth-token $(cat /opt/datadog-agent/run/auth_token) \
  --name my-test-client \
  --duration 60s
```

### Command-line Options

- `--ipc-address` - IPC server address (default: `localhost:5001`)
- `--auth-token` - Auth token (reads from `auth_token` file if not provided)
- `--name` - Client name for the subscription (default: `test-client`)
- `--duration` - How long to listen for config events (default: `30s`)

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
    site = "datadoghq.com" (source: File)
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

=========================
Phase 0 Exit Criteria
=========================
✓ Can receive snapshot
✓ Ordered sequence IDs (validated during streaming)
✓ Correct typed values (successfully parsed)

✓✓✓ Phase 0 COMPLETE: All exit criteria met! ✓✓✓
```

## Testing with Config Changes

While the test client is running, you can trigger config updates in another terminal:

```bash
# Set log level via runtime config
curl -X POST "http://localhost:5001/config/v1/log_level?value=debug" \
  -H "Authorization: Bearer $(cat /opt/datadog-agent/run/auth_token)"

# Or use the agent CLI
datadog-agent config set log_level debug
```

The test client should immediately receive an update event showing the config change.

## Phase 0 Completion

This test client validates that **Phase 0** of the RAR Config Streaming implementation is complete:

| Requirement | Status |
|-------------|--------|
| StreamConfigEvents on IPC server with mTLS | ✅ Complete |
| sequence_id to config mutations and snapshot generation | ✅ Complete |
| Snapshot is consistent and emitted first per subscriber | ✅ Complete |
| Request filtering by origins (optional) | ⚠️ Proto supports it, but not yet populated |

### Next Steps (Phase 1)

Phase 1 will implement a **shared Go consumer library** that remote agents can import to:
- Connect to the stream
- Wait for snapshot (block startup)
- Apply ordered updates
- Expose a `Reader()` for config access
- Provide change notifications

This test client serves as a reference implementation for how that consumer library should work.

## Troubleshooting

### Connection Refused

```
Failed to subscribe to config stream: rpc error: code = Unavailable desc = connection error
```

**Solution:** Make sure the core-agent is running and the IPC server is listening on the specified address.

### Authentication Failed

```
Failed to subscribe to config stream: rpc error: code = Unauthenticated
```

**Solution:** Verify the auth token is correct. Copy it from the agent's runtime directory.

### No Snapshot Received

If the client connects but doesn't receive a snapshot within the timeout:

1. Check that the `configstream` component is enabled in the core-agent
2. Verify the component is properly wired in the FX dependency graph
3. Check core-agent logs for any errors related to config streaming

## Integration Testing

For automated testing, see the integration tests in `comp/core/configstream/impl/configstream_test.go`:

```bash
cd comp/core/configstream/impl
go test -v -run TestPhase0ExitCriteria
```

These tests verify the same Phase 0 exit criteria without needing a running agent.
