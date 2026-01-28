# USM Debugger Test Programs

Test programs for investigating USM HTTP misattribution bug on Vagrant VM.

## Overview

These programs simulate the k8s API misattribution scenario:
- **tls_server.go**: Simulates k8s API server with HTTPS endpoints
- **tls_client.go**: Simulates kafka-admin-daemon making HTTPS requests

## Quick Start (Vagrant VM)

### 1. Build Test Programs

```bash
# On Vagrant VM
cd /git/datadog-agent/test/usm-debugger-test

# Build server
go build -o tls_server tls_server.go

# Build client
go build -o tls_client tls_client.go
```

### 2. Build usm-debugger

```bash
cd /git/datadog-agent
dda inv system-probe.build-usm-debugger --arch=arm64
# Output: bin/usm-debugger
```

### 3. Run Test Server

```bash
# Terminal 1: Start server
cd /git/datadog-agent/test/usm-debugger-test
./tls_server
# Output: Starting TLS server on :8443 (PID: 12345)
```

### 4. Run usm-debugger

```bash
# Terminal 2: Run usm-debugger
cd /git/datadog-agent
sudo ./bin/usm-debugger

# Terminal 3: Monitor eBPF logs
sudo cat /sys/kernel/tracing/trace_pipe | grep -E "persistentvolumes|configmaps|namespaces"
```

### 5. Run Test Client

```bash
# Terminal 4: Make requests
cd /git/datadog-agent/test/usm-debugger-test

# Make 10 requests
./tls_client -url https://127.0.0.1:8443 -count 10 -interval 2s

# Or continuous requests (press Ctrl+C to stop)
./tls_client -url https://127.0.0.1:8443 -interval 1s
```

## Expected Results

### What to Look For in eBPF Logs

```
# Expected log format (approximate)
system-probe [003] .... 789.123: bpf_trace_printk: http_process: pid=<CLIENT_PID> GET /api/v1/persistentvolumes/pvc-... 127.0.0.1:<ephemeral> -> 127.0.0.1:8443
```

### Key Data to Extract

1. **PID**: Which process captured the HTTP request?
   - Should be client PID (from tls_client)
   - If misattribution occurs, might be server PID or other process

2. **Connection Tuple**:
   - Source: 127.0.0.1:<ephemeral_port>
   - Dest: 127.0.0.1:8443

3. **Path**: Should match client request paths
   - `/api/v1/persistentvolumes/pvc-*`
   - `/api/v1/namespaces/*/configmaps`
   - etc.

### Validation Steps

1. **Get PIDs**:
   ```bash
   # Client PID (from tls_client output)
   # Server PID (from tls_server output)
   ```

2. **Verify PID in logs**:
   ```bash
   # Check if eBPF logs show correct client PID
   ps -p <PID_FROM_LOGS>
   ```

3. **Map PID to process**:
   ```bash
   ps aux | grep tls_client
   ps aux | grep tls_server
   ```

## Testing Scenarios

### Scenario 1: Basic Validation (10 requests)

```bash
./tls_client -url https://127.0.0.1:8443 -count 10 -interval 1s
```

**Goal**: Verify eBPF captures requests with correct PID

### Scenario 2: Continuous Load (Infinite)

```bash
./tls_client -url https://127.0.0.1:8443 -interval 500ms
```

**Goal**: Sustained traffic to test for sporadic issues

### Scenario 3: Burst Traffic

```bash
./tls_client -url https://127.0.0.1:8443 -count 100 -interval 100ms
```

**Goal**: High-frequency requests to stress test

## Troubleshooting

### Issue: No eBPF logs appearing

**Check**:
1. Is usm-debugger running? `ps aux | grep usm-debugger`
2. Are Go TLS uprobes attached? `sudo bpftool prog list | grep usm`
3. Is trace_pipe readable? `sudo cat /sys/kernel/tracing/trace_pipe`

### Issue: TLS handshake failures

**Check**:
1. Server is running: `netstat -tulpn | grep 8443`
2. Client accepts self-signed certs (already configured with InsecureSkipVerify)

### Issue: Connection refused

**Check**:
1. Server PID alive: `ps -p <SERVER_PID>`
2. Port 8443 available: `sudo lsof -i :8443`

## Next Steps

After validating on Vagrant VM:

1. **If eBPF logs show correct PID**: Basic capture works, proceed to staging
2. **If eBPF logs show wrong PID**: Reproduced misattribution locally!
3. **If no eBPF logs**: Debug usm-debugger setup

### Deploy to Staging (if Vagrant test succeeds)

1. Find active misattribution host (from investigation doc)
2. Deploy usm-debugger to that host's system-probe container
3. Capture real misattribution event
4. Identify actual PID making k8s API calls
5. Implement 6-point instrumentation in system-probe
6. Redeploy and collect detailed logs

## Reference

- Investigation doc: `/docs/dev/usm_misattribution_investigation.md`
- HTTP flow doc: `/docs/dev/usm_http_flow.md`
- Target host: i-08016d33ffc180a4f (ephemera-login-rate-limiter)
- Known misattributed endpoint: `GET /api/v1/persistentvolumes/`