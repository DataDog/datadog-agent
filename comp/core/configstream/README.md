# Config Stream Component

Streams configuration from core-agent to remote agents in real-time. Config is resolved once and pushed to all processes.

## Documentation

- **[PHASE0.md](PHASE0.md)** - Phase 0 implementation details
- **[PROJECT.md](PROJECT.md)** - Overall project vision and roadmap

## Quick Start

### Core Agent (Phase 0 ✅)

Automatically enabled. Config changes are streamed to all RAR-registered remote agents.

### Remote Agents (Phase 1 🚧)

```go
// Future: Consumer library handles everything
consumer.WaitReady(ctx)  // Block until config received
cfg := consumer.Reader() // Use config
```

**Current:** Manually implement gRPC client. See `cmd/config-stream-client/` for example.

## Key Features

- **Snapshot-first:** Complete config state before updates
- **Ordered delivery:** Sequence IDs guarantee ordering
- **RAR-gated:** Only registered remote agents can subscribe
- **Auto-resync:** Gaps trigger automatic snapshot resend
- **Non-blocking:** Slow consumers don't affect others

## Testing

### Unit Tests

```bash
cd comp/core/configstream/impl
go test -tags test -v
```

### Integration Test Client

```bash
# Build
go build -o bin/config-stream-client ./cmd/config-stream-client

# Run (requires running core-agent)
./bin/config-stream-client --ipc-address localhost:5001
```

See `cmd/config-stream-client/README.md` for details.

## Key Features

### 1. Snapshot-First Delivery

Every subscriber receives a complete snapshot before any updates:

```
Client connects → Snapshot (seq=100, all settings)
                → Update (seq=101, log_level=debug)
                → Update (seq=102, feature_flag=true)
```

### 2. Ordered Sequence IDs

Updates are delivered in strictly increasing sequence ID order. Gaps trigger automatic resync:

```
Client at seq=100 → Update seq=105 arrives
                  → Gap detected (101-104 missing)
                  → Snapshot sent (seq=105, all settings)
```

### 3. Multiple Subscribers

Multiple remote agents can subscribe simultaneously. Each gets independent snapshot + updates:

```
system-probe  → Subscribe → Snapshot (seq=100) → Update (seq=101) ...
trace-agent   → Subscribe → Snapshot (seq=100) → Update (seq=101) ...
process-agent → Subscribe → Snapshot (seq=100) → Update (seq=101) ...
```

### 4. Non-Blocking Fan-Out

Slow consumers don't block others:
- Buffered channels (100 events)
- Dropped events logged as warnings
- Fast consumers unaffected

### 5. Automatic Resync

Discontinuities automatically trigger resync:
- Client falls behind → snapshot sent
- Client reconnects → snapshot sent
- No manual intervention required

## Security

- **mTLS:** All IPC communication encrypted
- **Authentication:** Bearer token required
- **Authorization:** Only authenticated agents can subscribe

## Performance

### Benchmarks

- **Snapshot generation:** ~1-2ms for ~350 settings
- **Update fan-out:** ~10-50μs per subscriber
- **Memory:** ~100KB per subscriber (buffered channel)

### Optimization

- Lazy snapshot generation (only when needed)
- Shared snapshots across subscribers in same cycle
- Buffered channels prevent blocking

## Troubleshooting

### Client Not Receiving Snapshot

**Symptoms:** Client connects but times out waiting for snapshot

**Causes:**
1. Component not started (check lifecycle hooks)
2. Config not initialized (check `config.Component`)
3. Subscriber channel full (check logs for warnings)

**Solution:**
```bash
# Check core-agent logs
grep "configstream" /var/log/datadog/agent.log

# Check if component is running
datadog-agent status | grep -i config
```

### Sequence ID Gaps

**Symptoms:** Frequent resync warnings in logs

**Causes:**
1. Rapid config changes (expected behavior)
2. Slow consumer (channel full)
3. Network issues (reconnection)

**Solution:**
- Increase buffer size (modify component)
- Optimize consumer processing
- Check network latency

### Authentication Errors

**Symptoms:** `rpc error: code = Unauthenticated`

**Causes:**
1. Wrong auth token
2. Token expired
3. IPC component not initialized

**Solution:**
```bash
# Verify auth token
cat /opt/datadog-agent/run/auth_token

# Check IPC server is running
netstat -an | grep 5001
```

## Development

### Adding New Features

1. **Modify proto:** `pkg/proto/datadog/model/v1/model.proto`
2. **Regenerate:** `dda inv protobuf.generate`
3. **Update component:** `comp/core/configstream/impl/configstream.go`
4. **Add tests:** `comp/core/configstream/impl/configstream_test.go`

### Debugging

Enable debug logging:

```bash
# In datadog.yaml
log_level: debug

# Or at runtime
datadog-agent config set log_level debug
```

Watch for:
- `New subscriber 'X' joining the config stream`
- `Discontinuity detected for subscriber 'X'`
- `Dropping config update for subscriber 'X'`

## Component Status

- ✅ **Phase 0:** Complete (See [PHASE0.md](PHASE0.md))
- 🚧 **Phase 1:** Next (Consumer library)

## Contact

- **Team:** agent-runtimes, agent-configuration
- **Component:** `comp/core/configstream`
