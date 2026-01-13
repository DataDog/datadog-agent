# USM HTTP Metrics Investigation Summary

## Problem Statement

Flaky behavior in `universal.http.server.hits` metric where some HTTP hits are missing from the Datadog app:
- **Expected**: 40 curl requests → 40 hits for both docker-proxy and container
- **Observed**: 40 docker-proxy hits, but only 11-19 container (mysrv) hits
- **Missing**: 21-29 hits per test run (flaky)

## Architecture: How Connections and HTTP Data Flow

### Normal Connection Lifecycle

```
┌─────────────────────────────────────────────────────────────┐
│              ACTIVE CONNECTION                              │
├─────────────────────────────────────────────────────────────┤
│  1. Connection opens                                        │
│  2. eBPF kprobes track: tcp_sendmsg, tcp_recv, etc.        │
│  3. Stored in conn_stats eBPF map                          │
│  4. HTTP uprobes capture HTTP requests/responses           │
│  5. HTTP data stored in separate eBPF maps                 │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│              CONNECTION CLOSES                              │
├─────────────────────────────────────────────────────────────┤
│  1. tcp_close kprobe fires                                  │
│  2. cleanup_conn() called in eBPF                          │
│  3. bpf_map_delete_elem(&conn_stats) ← Removed from map   │
│  4. Connection data sent via perf/ring buffer              │
│  5. Userspace: storeClosedConnection()                     │
│  6. Stored in state.client.closed (per-client storage)    │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│           WHEN /connections IS QUERIED                      │
├─────────────────────────────────────────────────────────────┤
│  1. getConnections() → iterate conn_stats eBPF map         │
│     Returns: Active connections still open                  │
│                                                             │
│  2. GetProtocolStats() → get HTTP/USM data                 │
│     Returns: All buffered HTTP aggregations                 │
│                                                             │
│  3. state.GetDelta() → mergeConnections()                  │
│     - Merges active connections from step 1                 │
│     - Merges closed connections from state.client.closed   │
│     Returns: Combined list (active + closed)               │
│                                                             │
│  4. Encoding/Marshaling                                     │
│     - For each connection in combined list                  │
│     - Look up HTTP data by connection tuple                 │
│     - If match: attach httpAggregations                    │
│     - If no match: HTTP data becomes "orphan"              │
└─────────────────────────────────────────────────────────────┘
```

## Investigation Steps

### 1. HTTP Capture Verification ✅

**Test**: Stop process-agent, run 40 curl requests, query `/debug/http_monitoring`

**Results**:
- **80 HTTP entries captured** (40 localhost + 40 docker)
- All requests successfully captured by eBPF HTTP monitoring

**Conclusion**: ✅ HTTP monitoring works perfectly

### 2. Connection Tracking Investigation

**Test**: Run curl and check `conn_stats` eBPF map immediately

**Method**:
```bash
curl -s http://127.0.0.1:8085/daniel > /dev/null
sudo bpftool map dump name conn_stats | jq
```

**Results**:
- Map contains 16 connections total
- **ZERO connections on port 8085**
- Connections tracked: 8125 (StatsD), 53 (DNS), 443 (HTTPS), 22 (SSH)

### 3. Slow Connection Test ✅

**Test**: Use `curl --limit-rate 10` to keep connection open longer

**Results**: SUCCESS! When connection stays open:
```json
{
  "laddr": {"ip": "127.0.0.1", "port": 8085},
  "raddr": {"ip": "127.0.0.1", "port": 33260},
  "pid": 3340,
  "httpAggregations": "Eh8iBy9kYW5pZWww..." ✅
}
```

**4 connections on port 8085 captured with HTTP data attached!**

**Conclusion**: When connections are alive during query, matching works perfectly.

### 4. Closed Connection Flow Investigation

**Question**: What happens to short-lived connections that close before `/connections` is queried?

**Code Analysis** (`pkg/network/ebpf/c/tracer.c` + `pkg/network/tracer/tracer.go`):

1. **eBPF** (tracer.c:265):
   - `tcp_close` kprobe fires
   - `cleanup_conn()` → `bpf_map_delete_elem(&conn_stats)`
   - Connection sent to userspace via perf buffer

2. **Userspace** (tracer.go:327):
   - `storeClosedConnection()` called
   - `t.state.StoreClosedConnection(cs)` → stored in `client.closed`

3. **Query** (state.go:362):
   - `mergeConnections()` returns both active + closed
   - `GetDelta()` returns `append(active, closed...)`

**Expected**: Closed connections should be returned in `/connections` response!

### 5. Testing Closed Connection Retrieval ❌

**Test**: Run 3 curl requests, immediately query `/connections`

**Results**:
- `/connections` returned 13 connections
- **ZERO on port 8085** ❌
- Orphan logs: "detected orphan http aggregations. count=8"

**Critical Finding**: Despite being stored in `state.client.closed`, port 8085 connections are NOT appearing in `/connections` responses!

## Current Hypothesis

Short-lived HTTP connections (< 2 seconds) are:
1. ✅ Captured by HTTP monitoring eBPF
2. ✅ Removed from `conn_stats` map when tcp_close fires
3. ❓ **Being sent from eBPF to userspace via perf buffer?**
4. ❓ **Being received and stored in state.client.closed?**
5. ❌ **NOT appearing in `/connections` response**
6. ❌ HTTP data becomes orphaned (no matching connection)

### Possible Root Causes

**Option A: Connections not being sent from eBPF**
- `tcp_close` batching may drop connections
- Perf/ring buffer overflow
- eBPF program filtering certain connections

**Option B: Connections not being stored in userspace**
- Race condition in `storeClosedConnection()`
- Client not registered when connection closes
- Filtering in userspace rejecting port 8085

**Option C: Connections stored but not returned**
- `mergeConnections()` filtering them out
- Client state being cleared too early
- Timing issue with state retrieval

## ✅ Testing Artifact Discovered

**Issue**: `clients=0` causing connections to drop during testing

**Root Cause**: `storeClosedConnection()` only stores connections when `len(ns.clients) > 0`
- Without process-agent polling, no clients registered
- Connections closing before first query are dropped

**In Production**: NOT an issue - process-agent polls every ~30 seconds, keeping client registered

## ✅ System-Probe Verification/c

**Test with process-agent running** (10 curl requests):

```bash
[INVESTIGATION] mergeConnections: total_closed=47 port_8085=40 client=process-agent-unique-id
[INVESTIGATION] mergeConnections result: returning 47 closed (port_8085=40) + 16 active
http encoder: creating encoder with 20 HTTP aggregations
```

**Confirmed Working**:
- ✅ 40 connections on port 8085 captured
- ✅ 20 HTTP aggregations (10 docker-proxy + 10 mysrv)
- ✅ NO orphan HTTP data (all matched)
- ✅ Container tagged: `DD_SERVICE=mysrv`, PID=3307, IP=172.17.0.2
- ✅ Docker-proxy PID=3340

## ✅ ROOT CAUSE IDENTIFIED: Docker-Proxy Filter Bug

**Location**: `pkg/process/metadata/parser/dockerproxy.go`

### The Problem

Process-agent's `DockerProxy.Filter()` incorrectly filters out container connections, removing mysrv metrics before they reach the backend.

### Evidence from Process-Agent Logs

With debug logging enabled in dockerproxy.go, logs show container connections being filtered:

```
[INVESTIGATION] Filtering proxied connection: laddr=172.17.0.2:8085 raddr=172.17.0.1:55748 pid=3307 proxy_pid=3340 proxy_ip=172.17.0.1
[INVESTIGATION] Filtering proxied connection: laddr=172.17.0.1:55748 raddr=172.17.0.2:8085 pid=3340 proxy_pid=3340 proxy_ip=172.17.0.1
[INVESTIGATION] Docker proxy filter: port_8085_before=40 port_8085_after=20 total_before=55 total_after=35
```

**Key Evidence**:
- **40 connections** on port 8085 before filtering
- **20 connections** after filtering
- **20 connections removed**: 10 from docker-proxy (PID 3340) ✅ + 10 from mysrv (PID 3307) ❌

Notice: `pid=3307` (mysrv) ≠ `proxy_pid=3340` (docker-proxy), yet it's still filtered!

### Evidence from Backend Dump

Analysis of `/Users/daniel.lavie/Downloads/dump_1767024606.txt` (connections sent to backend):

```
Total port 8085 connections: 60

Grouped by PID:
  PID 3340 (docker-proxy): 30 connections ✅
  PIDs 12165-12391 (curl): 30 connections ✅
  PID 3307 (mysrv): 0 connections ❌
```

Only `127.0.0.1:8085` connections sent to backend. All Docker bridge (`172.17.0.x`) connections filtered out.

### Bypass Test - Definitive Proof

Modified filter to skip filtering port 8085 connections:

**Before fix** (08:48:37):
```
port_8085_before=40, port_8085_after=20  ← 20 connections filtered
```

**After bypass** (08:52:43):
```
port_8085_before=40, port_8085_after=40  ← NO connections filtered
```

**Result**: Both docker-proxy AND mysrv hits now appear in Datadog! ✅

### The Bug Explained

#### Docker-Proxy Setup
When running `docker run -p 8085:8085 mysrv`:
- **Target**: `172.17.0.2:8085` (mysrv container)
- **Proxy PID**: 3340
- **Proxy IP**: `172.17.0.1` (docker0 bridge)

#### Connections Created (per curl request)
1. `curl → 127.0.0.1:8085` (localhost, PID: curl)
2. `127.0.0.1:8085 → curl` (localhost, PID: 3340 docker-proxy) ← **Visible in Datadog**
3. `172.17.0.1:xxxxx → 172.17.0.2:8085` (bridge, PID: 3340 docker-proxy) ← **Should be filtered**
4. `172.17.0.2:8085 → 172.17.0.1:xxxxx` (bridge, PID: 3307 mysrv) ← **Should NOT be filtered!**

#### Current Flawed Logic (dockerproxy.go:134-156)

```go
func (d *DockerProxy) isProxied(c *model.Connection) bool {
    // Check if Laddr is the target
    if p, ok := d.proxyByTarget[c.Laddr]; ok {
        return p.ip == c.Raddr.Ip  // ← BUG: No PID check!
    }

    // Check if Raddr is the target
    if p, ok := d.proxyByTarget[c.Raddr]; ok {
        return p.ip == c.Laddr.Ip  // ← BUG: No PID check!
    }

    return false
}
```

The filter only checks if the connection matches the IP/port pattern:
- One endpoint = target (`172.17.0.2:8085`)
- Other endpoint = proxy IP (`172.17.0.1`)

**It never verifies the PID!**

#### What Gets Filtered

**Connection #3** (docker-proxy forwarding): `172.17.0.1 → 172.17.0.2:8085` (PID: 3340)
- Raddr `172.17.0.2:8085` in proxyByTarget? YES
- Laddr IP `172.17.0.1` == proxy IP? YES
- **FILTERED** ✅ (Correct - avoid double-counting)

**Connection #4** (mysrv serving): `172.17.0.2:8085 → 172.17.0.1` (PID: 3307)
- Laddr `172.17.0.2:8085` in proxyByTarget? YES
- Raddr IP `172.17.0.1` == proxy IP? YES
- **FILTERED** ❌ (Wrong - it's the container, not docker-proxy!)

Both match the pattern, so both get filtered, even though only #3 is actually a docker-proxy connection.

### Historical Context

This is a **5-year-old bug**:

```bash
git log --format="%ai %s" -- pkg/process/dockerproxy/filter.go
2020-01-10 15:42:27 [system-probe] Add support for docker-proxy traffic (#4665)
```

The original implementation from January 2020 never checked PID. The bug persisted when code was moved to `pkg/process/metadata/parser/dockerproxy.go` in December 2022.

**Why undetected?** The original test used the redis-server PID for both the container and docker-proxy connections, never verifying that only docker-proxy's own connections get filtered.

### Impact

Any containerized service behind docker-proxy with published ports (`-p`) loses metrics:
- System-probe captures all HTTP transactions correctly
- Process-agent filters out container connections before sending to backend
- Only docker-proxy metrics appear in Datadog, not the actual service metrics

## The Fix

**File**: `pkg/process/metadata/parser/dockerproxy.go:128-152`

### Problem
The `isProxied()` function only checks if the connection matches the IP/port pattern, without verifying the PID:

```go
// BEFORE (Buggy)
func (d *DockerProxy) isProxied(c *model.Connection) bool {
    if p, ok := d.proxyByTarget[c.Laddr]; ok {
        return p.ip == c.Raddr.Ip  // ← No PID check!
    }

    if p, ok := d.proxyByTarget[c.Raddr]; ok {
        return p.ip == c.Laddr.Ip  // ← No PID check!
    }

    return false
}
```

This causes both docker-proxy AND container connections to be filtered.

### Solution
Add PID check to ensure only docker-proxy's own connections are filtered:

```go
// AFTER (Fixed)
func (d *DockerProxy) isProxied(c *model.Connection) bool {
    if p, ok := d.proxyByTarget[model.ContainerAddr{Ip: c.Laddr.Ip, Port: c.Laddr.Port, Protocol: c.Type}]; ok {
        // Only filter if IP pattern matches AND it's the docker-proxy's connection
        return p.ip == c.Raddr.Ip && c.Pid == p.pid  // ← Added PID check
    }

    if p, ok := d.proxyByTarget[model.ContainerAddr{Ip: c.Raddr.Ip, Port: c.Raddr.Port, Protocol: c.Type}]; ok {
        // Only filter if IP pattern matches AND it's the docker-proxy's connection
        return p.ip == c.Laddr.Ip && c.Pid == p.pid  // ← Added PID check
    }

    return false
}
```

### Verification

**Before Fix:**
```
[INVESTIGATION] Filtering proxied connection: pid=3307 proxy_pid=3340  ← Container filtered!
[INVESTIGATION] Filtering proxied connection: pid=3340 proxy_pid=3340  ← Docker-proxy filtered
[INVESTIGATION] Docker proxy filter: port_8085_before=40 port_8085_after=20
```
- 20 connections filtered (10 docker-proxy + 10 mysrv ❌)

**After Fix:**
```
[INVESTIGATION] Filtering proxied connection: pid=3340 proxy_pid=3340  ← Only docker-proxy!
[INVESTIGATION] Filtering proxied connection: pid=3340 proxy_pid=3340  ← Only docker-proxy!
[INVESTIGATION] Docker proxy filter: port_8085_before=40 port_8085_after=30
```
- 10 connections filtered (only docker-proxy ✅)
- No more mysrv (PID 3307) connections being filtered ✅

### Expected Result

With the fix:
- **Docker-proxy bridge connections** (PID 3340): Filtered ✅ (avoid double-counting)
- **Container bridge connections** (PID 3307): Kept ✅ (show service metrics)
- **Localhost connections** (both PIDs): Kept ✅ (show docker-proxy metrics)

Final metrics in Datadog:
- **Docker-proxy metrics**: From localhost connections (`127.0.0.1:8085`)
- **Mysrv metrics**: From container bridge connections (`172.17.0.2:8085`)
- **No double-counting**: Only docker-proxy bridge connections filtered

## Files Modified

- `/pkg/network/encoding/marshal/usm_http.go` - Added HTTP encoder debug logs
- `/pkg/network/encoding/marshal/usm.go` - Enhanced orphan detection logging

## Test Commands

### Prerequisites
```bash
# On Vagrant VM
docker run -d --rm --name mysrv-test -e DD_SERVICE=mysrv -p 8085:8085 \
    python:3.12-slim python -m http.server -b 0.0.0.0 8085
sudo pkill -9 process-agent
```

### Quick Test
```bash
# Run 3 requests
for i in {1..3}; do
    curl -s http://127.0.0.1:8085/daniel > /dev/null
    echo "Request $i done"
    sleep 2
done

# Immediately query
sudo curl --unix-socket /opt/datadog-agent/run/sysprobe.sock \
    "http://unix/network_tracer/connections" 2>/dev/null \
    | jq '.conns[] | select(.laddr.port == 8085 or .raddr.port == 8085)'
```

### Check HTTP Data
```bash
sudo curl --unix-socket /opt/datadog-agent/run/sysprobe.sock \
    "http://unix/network_tracer/debug/http_monitoring" 2>/dev/null \
    | jq 'length'
```
