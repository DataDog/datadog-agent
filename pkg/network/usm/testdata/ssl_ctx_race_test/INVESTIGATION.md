# Investigation: ssl_ctx_by_pid_tgid Race Condition

**Date**: February 4, 2025
**Updated**: February 5, 2025
**Status**: Confirmed - Bug is practical and causes significant misattribution
**Severity**: High - 64-81% misattribution rate in test scenarios

---

## Executive Summary

The `ssl_ctx_by_pid_tgid` fallback mechanism in USM's TLS monitoring has **two distinct bugs**:

### Bug 1: Race Condition on Write Path (CONFIRMED)
When a single thread operates on multiple TLS connections with interleaved writes, the `ssl_ctx_by_pid_tgid` map gets overwritten, causing **massive misattribution** of HTTP traffic.

- **Reproduction test result**: 63.9% misattribution rate (267 of 418 requests)
- **Affected operations**: SSL_write → tcp_sendmsg path

### Bug 2: Missing Correlation Hook on Read Path (CONFIRMED)
The `tcp_recvmsg` kernel probe does NOT call `map_ssl_ctx_to_sock()`, meaning SSL_read operations on pre-existing connections **cannot be correlated at all**.

- **Affected operations**: SSL_read → tcp_recvmsg path
- **Impact**: Read-only TLS traffic on pre-existing connections is completely uncaptured

---

## Bug Location

**File**: `pkg/network/ebpf/c/protocols/tls/https.h`
**Functions**: `tup_from_ssl_ctx()` (lines 271-310) and `map_ssl_ctx_to_sock()` (lines 324-345)

### The Fallback Mechanism

When `ssl_sock_by_ctx` lookup fails (connection established before monitoring started), the code uses a fallback:

```c
// In tup_from_ssl_ctx() - called from SSL_read/SSL_write probes
ssl_sock_t *ssl_sock = bpf_map_lookup_elem(&ssl_sock_by_ctx, &key);
if (ssl_sock == NULL) {
    // Fallback: store ssl_ctx keyed ONLY by pid_tgid
    bpf_map_update_with_telemetry(ssl_ctx_by_pid_tgid, &pid_tgid, &ssl_ctx, BPF_ANY);
    return NULL;
}
```

```c
// In map_ssl_ctx_to_sock() - called from tcp_sendmsg probe
void **ssl_ctx_map_val = bpf_map_lookup_elem(&ssl_ctx_by_pid_tgid, &pid_tgid);
if (ssl_ctx_map_val == NULL) {
    return;
}
bpf_map_delete_elem(&ssl_ctx_by_pid_tgid, &pid_tgid);
// ... associates ssl_ctx with the socket from tcp_sendmsg
```

### The Race Condition (Write Path)

**CORRECTION (Feb 5, 2025)**: The race occurs on the **SSL_write → tcp_sendmsg** path, not SSL_read.
SSL_read triggers tcp_recvmsg which has NO correlation hook (see Bug 2 below).

```
Timeline for single thread doing interleaved writes on conn1 and conn2:

T1: SSL_write(conn1) entry  → tup_from_ssl_ctx() stores ctx1 in map[pid_tgid]
T2: SSL_write(conn2) entry  → tup_from_ssl_ctx() OVERWRITES map[pid_tgid] with ctx2
T3: tcp_sendmsg for conn1   → map_ssl_ctx_to_sock() reads map[pid_tgid] → gets ctx2 (WRONG!)
T4: tcp_sendmsg for conn2   → map[pid_tgid] already deleted, no association

Result: conn1's HTTP request attributed to conn2's connection tuple
```

The fundamental problem: **`ssl_ctx_by_pid_tgid` is keyed only by thread ID, not by connection**. When a thread operates on multiple connections, they overwrite each other.

### The Missing Hook (Read Path)

```
Timeline for SSL_read on pre-existing connection:

T1: SSL_read(conn1) entry   → tup_from_ssl_ctx() stores ctx1 in ssl_ctx_by_pid_tgid
T2: SSL_read internally calls recv()
T3: tcp_recvmsg fires       → NO map_ssl_ctx_to_sock() call! (hook is missing)
T4: SSL_read(conn1) return  → tup_from_ssl_ctx() still misses ssl_sock_by_ctx → returns NULL
T5: HTTP response data goes to cleanup → NOT CLASSIFIED

Result: SSL_read data on pre-existing connections is completely lost
```

The stored ssl_ctx in `ssl_ctx_by_pid_tgid` is never consumed by tcp_recvmsg because there's no correlation hook.

---

## Deep Dive: The Fallback Mechanism

### Normal Flow (Primary Path)

When system-probe is running **before** a TLS connection is established, the primary path works correctly:

```
1. Application calls SSL_set_fd(ssl_ctx, fd)
   └─→ uprobe__SSL_set_fd fires
       └─→ init_ssl_sock(ssl_ctx, fd)
           └─→ Stores (pid_tgid, ssl_ctx) → {fd} in ssl_sock_by_ctx map

2. Later, SSL_read/SSL_write is called
   └─→ tup_from_ssl_ctx(ssl_ctx, pid_tgid)
       └─→ Looks up ssl_sock_by_ctx[(pid_tgid, ssl_ctx)]
       └─→ FOUND! Returns the connection tuple
       └─→ Traffic correctly attributed ✓
```

### When Does the Fallback Fire?

The fallback fires when `ssl_sock_by_ctx` lookup **fails** (line 276-277 in https.h):

```c
ssl_sock_t *ssl_sock = bpf_map_lookup_elem(&ssl_sock_by_ctx, &key);
if (ssl_sock == NULL) {
    // FALLBACK PATH - ssl_sock_by_ctx has no entry for this connection
    bpf_map_update_with_telemetry(ssl_ctx_by_pid_tgid, &pid_tgid, &ssl_ctx, BPF_ANY);
    return NULL;
}
```

### Why Would ssl_sock_by_ctx Miss?

**Scenario 1: Connection existed before system-probe started**

```
Timeline:
T1: Application starts, calls SSL_set_fd(ctx, fd)
    └─→ system-probe not running yet, no probe fires
T2: Application does SSL_read/SSL_write
    └─→ system-probe still not running
T3: system-probe starts, attaches uprobes
T4: Application does SSL_read/SSL_write
    └─→ uprobe__SSL_read fires
    └─→ tup_from_ssl_ctx() called
    └─→ ssl_sock_by_ctx lookup MISSES (SSL_set_fd was never intercepted)
    └─→ FALLBACK FIRES
```

**Scenario 2: SSL_set_fd/SSL_set_bio hooks weren't intercepted**
- Some OpenSSL versions or wrappers might use different initialization paths
- The binary might not be traced (blocked, stripped, etc.)
- Alternative SSL libraries (though they have separate hooks)

### How the Fallback Attempts to Work

The fallback relies on correlating SSL operations with subsequent kernel `tcp_sendmsg` calls.

**IMPORTANT**: The correlation ONLY works for the WRITE path (SSL_write → tcp_sendmsg).
The READ path (SSL_read → tcp_recvmsg) has NO correlation hook.

```
WRITE PATH (works, but has race condition):
───────────────────────────────────────────
STEP 1: SSL_read entry (uprobe__SSL_read) - Line 89-100, native-tls.h
  - Calls tup_from_ssl_ctx(ssl_ctx, pid_tgid)
  - ssl_sock_by_ctx lookup MISSES
  - Store: ssl_ctx_by_pid_tgid[pid_tgid] = ssl_ctx
  - Return NULL (no tuple available yet)

STEP 2: SSL_write internally calls send() → tcp_sendmsg
  - kprobe/tcp_sendmsg fires (Line 561-567, native-tls.h)
  - Calls map_ssl_ctx_to_sock(sock) (Line 565)

STEP 3: map_ssl_ctx_to_sock correlates (Line 324-345, https.h)
  - Look up: ssl_ctx = ssl_ctx_by_pid_tgid[pid_tgid]
  - DELETE the entry from ssl_ctx_by_pid_tgid
  - Read connection tuple from sock kernel struct
  - Store: ssl_sock_by_ctx[(pid_tgid, ssl_ctx)] = {tuple}
  - Store: ssl_ctx_by_tuple[tuple] = ssl_ctx
  - Now future lookups will use the primary path!

READ PATH (BROKEN - no correlation hook):
─────────────────────────────────────────
STEP 1: SSL_read entry (uprobe__SSL_read) - Line 89-100, native-tls.h
  - Calls tup_from_ssl_ctx(ssl_ctx, pid_tgid)
  - Store: ssl_ctx_by_pid_tgid[pid_tgid] = ssl_ctx

STEP 2: SSL_read internally calls recv() → tcp_recvmsg
  - kprobe/tcp_recvmsg fires (Line 13-32, tcp_recv.h)
  - Does NOT call map_ssl_ctx_to_sock() ← MISSING HOOK!
  - Only stores socket pointer for metrics

STEP 3: SSL_read return (uretprobe__SSL_read) - Line 103-140, native-tls.h
  - Calls tup_from_ssl_ctx(ssl_ctx, pid_tgid) again
  - ssl_sock_by_ctx lookup STILL MISSES
  - Returns NULL → data NOT classified
```

### The Flawed Assumption

The code comment (lines 285-288 in https.h) explicitly states the assumption:

> "The whole thing works based on the assumption that SSL_read/SSL_write is
> then followed by the execution of tcp_sendmsg **within the same CPU context**.
> This is not necessarily true for all cases (such as when using the async SSL API)
> but seems to work on most-cases."

**The assumption is**: Only ONE SSL operation per thread at a time, and tcp_sendmsg fires before the next SSL operation.

**Reality**: A single thread can operate on multiple TLS connections with interleaved writes:

```
Thread doing interleaved WRITES on two connections (race scenario):

T1: SSL_write(conn1) entry → ssl_ctx_by_pid_tgid[pid_tgid] = ctx1
T2: SSL_write(conn2) entry → ssl_ctx_by_pid_tgid[pid_tgid] = ctx2  ← OVERWRITES ctx1!
T3: tcp_sendmsg(conn1)     → reads ssl_ctx_by_pid_tgid[pid_tgid] → gets ctx2 (WRONG!)
                             associates conn1's socket with ctx2's SSL context
T4: tcp_sendmsg(conn2)     → ssl_ctx_by_pid_tgid[pid_tgid] already deleted
                             no association made

Result: conn1's HTTP request is attributed to conn2's connection tuple

Thread doing interleaved READS on two connections (missing hook scenario):

T1: SSL_read(conn1) entry  → ssl_ctx_by_pid_tgid[pid_tgid] = ctx1
T2: tcp_recvmsg(conn1)     → NO correlation hook! ssl_ctx not consumed
T3: SSL_read(conn2) entry  → ssl_ctx_by_pid_tgid[pid_tgid] = ctx2  ← OVERWRITES ctx1!
T4: tcp_recvmsg(conn2)     → NO correlation hook!

Result: Neither read is correlated; data lost entirely
```

### Visual Summary

```
                    PRIMARY PATH                         FALLBACK PATH
                    (SSL_set_fd intercepted)             (SSL_set_fd missed)

SSL_set_fd ──────► ssl_sock_by_ctx populated             (nothing happens)
                          │
                          ▼
SSL_read/write ──► tup_from_ssl_ctx()
                          │
                   ┌──────┴──────┐
                   │             │
                   ▼             ▼
         ssl_sock_by_ctx    ssl_sock_by_ctx
              HIT              MISS
                │                │
                ▼                ▼
         Return tuple    ssl_ctx_by_pid_tgid[pid_tgid] = ssl_ctx
              ✓                  │
                                 ▼ (later, hopefully)
                         tcp_sendmsg fires
                                 │
                                 ▼
                         map_ssl_ctx_to_sock()
                                 │
                                 ▼
                         Populate ssl_sock_by_ctx from
                         kernel sock + stored ssl_ctx

                         ⚠️  RACE CONDITION:
                         If another SSL operation on a DIFFERENT
                         connection happens before tcp_sendmsg,
                         ssl_ctx_by_pid_tgid gets OVERWRITTEN
                         and the wrong ssl_ctx is associated!
```

### Key Insight

The map `ssl_ctx_by_pid_tgid` uses **only pid_tgid as the key**. This means:
- One entry per thread
- Multiple connections on the same thread share one slot
- Last writer wins → race condition

The primary path (`ssl_sock_by_ctx`) uses **(pid_tgid, ssl_ctx)** as the key, which is unique per connection per thread, avoiding this problem entirely.

---

## Reproduction

### Prerequisites

- Linux system with eBPF support
- system-probe built with USM/TLS enabled
- OpenSSL development libraries
- Python 3

### Test Files

Located in `pkg/network/usm/testdata/ssl_ctx_race_test/`:
- `ssl_ctx_race.c` - C test client
- `Makefile` - Build script
- `run_race_test.sh` - Automated test script
- `manual_test.sh` - Step-by-step instructions

### Reproduction Steps

#### 1. Build the test client

```bash
cd pkg/network/usm/testdata/ssl_ctx_race_test
make
```

#### 2. Start two HTTPS servers

Terminal 1:
```bash
python3 -c "
import http.server, ssl
class H(http.server.BaseHTTPRequestHandler):
    protocol_version = 'HTTP/1.1'
    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-Length', '2')
        self.send_header('Connection', 'keep-alive')
        self.end_headers()
        self.wfile.write(b'OK')
s = http.server.HTTPServer(('127.0.0.1', 18001), H)
c = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
c.load_cert_chain('../../protocols/http/testutil/testdata/cert.pem.0',
                  '../../protocols/http/testutil/testdata/server.key')
s.socket = c.wrap_socket(s.socket, server_side=True)
s.serve_forever()
"
```

Terminal 2: Same but with port 18002.

#### 3. Start the test client (establishes connections and waits)

```bash
./ssl_ctx_race 127.0.0.1 18001 127.0.0.1 18002 500
```

Output:
```
Establishing connection 1 to 127.0.0.1:18001...
Establishing connection 2 to 127.0.0.1:18002...
READY:37180:18001:45612:18002
Connections established:
  conn1: local=37180 -> remote=18001 (marker=conn1)
  conn2: local=45612 -> remote=18002 (marker=conn2)
Waiting for SIGUSR1 to start test (PID=901791)...
```

#### 4. Start system-probe AFTER connections are established

This is critical - connections must exist BEFORE monitoring starts to force the fallback path.

```bash
sudo ./bin/system-probe/system-probe run -c dev/dist/datadog.yaml
```

#### 5. Signal the client to start rapid operations

```bash
kill -USR1 <CLIENT_PID>
```

#### 6. Query USM debug endpoint

```bash
sudo curl -s --unix-socket /opt/datadog-agent/run/sysprobe.sock \
    "http://unix/network_tracer/debug/http_monitoring" > /tmp/http_debug.json
```

#### 7. Analyze results

```bash
cat /tmp/http_debug.json | python3 -c "
import json, sys
data = json.load(sys.stdin)
conn1_correct = sum(1 for e in data if 'conn1' in e.get('Path','') and e.get('Server',{}).get('Port')==18001)
conn1_wrong = sum(1 for e in data if 'conn1' in e.get('Path','') and e.get('Server',{}).get('Port')==18002)
conn2_wrong = sum(1 for e in data if 'conn2' in e.get('Path','') and e.get('Server',{}).get('Port')==18001)
conn2_correct = sum(1 for e in data if 'conn2' in e.get('Path','') and e.get('Server',{}).get('Port')==18002)
print(f'conn1 -> 18001 (correct): {conn1_correct}')
print(f'conn1 -> 18002 (WRONG):   {conn1_wrong}')
print(f'conn2 -> 18001 (WRONG):   {conn2_wrong}')
print(f'conn2 -> 18002 (correct): {conn2_correct}')
print(f'Misattribution rate: {(conn1_wrong+conn2_wrong)/(conn1_correct+conn1_wrong+conn2_wrong+conn2_correct)*100:.1f}%')
"
```

### Results (Original Test - Sequential Mode)

```
=== Attribution Analysis ===
conn1 -> port 18001 (CORRECT):  138
conn1 -> port 18002 (WRONG):    408
conn2 -> port 18001 (WRONG):    402
conn2 -> port 18002 (CORRECT):  150

*** RACE CONDITION CONFIRMED: 810 misattributed requests! ***
```

**Misattribution rate: 81%**

### Results (February 5, 2025 - Interleaved Mode on Vagrant VM)

Using `ssl_ctx_race_v2` with `--interleaved` mode which maximizes the race window by doing:
`SSL_write(conn1)`, `SSL_write(conn2)`, `SSL_read(conn1)`, `SSL_read(conn2)` per iteration.

**Baseline test (system-probe started BEFORE connections):**
```
Total entries captured: 80
Misattribution: 0%
```

**Race condition test (connections established BEFORE system-probe):**
```
Expected requests:     400
Captured requests:     418

ATTRIBUTION BREAKDOWN:
  conn1 -> 18001 (correct):  67
  conn1 -> 18002 (WRONG):    133
  conn2 -> 18001 (WRONG):    134
  conn2 -> 18002 (correct):  84

Correct attribution:   151 (36.1%)
MISATTRIBUTION:        267 (63.9%)

Sample misattributed entry:
  Path=/200/conn1-iter163, Server.Port=18002  ← conn1 request attributed to port 18002!
```

**Key observation**: The misattribution is symmetric (133 vs 134), confirming SSL contexts are being randomly swapped due to the race condition.

---

## Impact

### When Does This Occur?

1. **Pre-existing connections**: TLS connections established before system-probe starts
2. **Missed SSL_set_fd**: When `SSL_set_fd`/`SSL_set_bio` hooks aren't intercepted
3. **Single-threaded multiplexing**: Applications using one thread for multiple TLS connections (common in async frameworks, connection pools, proxies)

### Real-World Scenarios

- **HTTP/1.1 connection pools**: Multiple keep-alive connections handled by worker threads
- **Async I/O frameworks**: libuv, Boost.Asio, etc. multiplexing connections
- **Proxy servers**: HAProxy, nginx, envoy handling multiple upstream connections
- **Database connection pools**: Multiple TLS connections to different shards

### Consequences

- HTTP requests attributed to wrong services
- Incorrect latency metrics (mixing fast/slow endpoints)
- Wrong topology in service maps
- Security monitoring blind spots

---

## Suggested Solution

### Option 1: Include SSL Context in Map Key (Recommended)

Change `ssl_ctx_by_pid_tgid` to include the SSL context pointer in the key:

```c
// New key structure
typedef struct {
    __u64 pid_tgid;
    void *ssl_ctx;
} ssl_ctx_pid_tgid_key_t;

// In tup_from_ssl_ctx():
ssl_ctx_pid_tgid_key_t key = {
    .pid_tgid = pid_tgid,
    .ssl_ctx = ssl_ctx,
};
bpf_map_update_with_telemetry(ssl_ctx_by_pid_tgid, &key, &ssl_ctx, BPF_ANY);

// In map_ssl_ctx_to_sock() - need ssl_ctx from somewhere
// Problem: tcp_sendmsg doesn't have ssl_ctx...
```

**Challenge**: `tcp_sendmsg` doesn't have the SSL context, so we can't look up by (pid_tgid, ssl_ctx).

### Option 2: Use Socket/FD as Key

Store the file descriptor or socket pointer along with pid_tgid:

```c
typedef struct {
    __u64 pid_tgid;
    __u32 fd;  // or sock pointer
} ssl_ctx_fd_key_t;
```

**Challenge**: Getting the FD in `tup_from_ssl_ctx()` requires additional probes or map lookups.

### Option 3: Probabilistic Correlation with Timing

Keep track of the most recent SSL operation timestamp and correlate with tcp_sendmsg timing:

```c
typedef struct {
    void *ssl_ctx;
    __u64 timestamp;
} ssl_ctx_entry_t;

// Store multiple entries per pid_tgid (small array)
BPF_HASH_MAP(ssl_ctx_by_pid_tgid, __u64, ssl_ctx_entry_t[4], ...)
```

Match tcp_sendmsg to the entry with closest timestamp.

**Downside**: Still probabilistic, adds complexity.

### Option 4: Deprecate the Fallback (Simplest)

Remove or disable the fallback mechanism entirely:

```c
static __always_inline conn_tuple_t* tup_from_ssl_ctx(void *ssl_ctx, u64 pid_tgid) {
    ssl_sock_t *ssl_sock = bpf_map_lookup_elem(&ssl_sock_by_ctx, &key);
    if (ssl_sock == NULL) {
        // Don't use fallback - just return NULL
        // Connection will not be monitored, but at least data is accurate
        return NULL;
    }
    // ... rest of function
}
```

**Trade-off**: Lose visibility into pre-existing connections, but eliminate misattribution.

### Option 5: Per-Connection Tracking via Socket Pointer

Track the mapping from SSL context to socket at the kernel level using the socket pointer from `tcp_sendmsg`:

```c
// New approach: in SSL_read/SSL_write entry, store (pid_tgid, ssl_ctx)
// In tcp_sendmsg, store (pid_tgid, sock)
// Correlate by matching pid_tgid AND ensuring they happen in sequence

// Use a bounded buffer per pid_tgid
typedef struct {
    void *ssl_ctx;
    struct sock *sock;  // filled in by tcp_sendmsg
    __u8 state;  // 0=ssl_pending, 1=sock_filled
} pending_ssl_sock_t;

BPF_HASH_MAP(pending_ssl_by_pid_tgid, __u64, pending_ssl_sock_t[MAX_PENDING], ...)
```

---

## Code Verification (February 5, 2025)

### Verified: `map_ssl_ctx_to_sock()` is ONLY called from tcp_sendmsg

```bash
$ grep -r "map_ssl_ctx_to_sock" pkg/network/ebpf/c/
pkg/network/ebpf/c/protocols/tls/native-tls.h:565:    map_ssl_ctx_to_sock(sk);
pkg/network/ebpf/c/protocols/tls/https.h:324:static __always_inline void map_ssl_ctx_to_sock(...)
```

Only 2 matches: the definition (https.h:324) and the single call site (native-tls.h:565 in kprobe/tcp_sendmsg).

### Verified: tcp_recvmsg has NO TLS correlation logic

The tcp_recvmsg hooks in `tracer/tcp_recv.h`:
- `kprobe__tcp_recvmsg` (Line 13-32): Stores socket pointer only
- `kretprobe__tcp_recvmsg` (Line 64-83): Calls `handle_tcp_recv()`
- `handle_tcp_recv()` (stats.h:354-367): Only updates connection stats

**None of these call `map_ssl_ctx_to_sock()` or any TLS-related functions.**

### Code Path Asymmetry Summary

| Path | Hook | Correlation | Status |
|------|------|-------------|--------|
| SSL_write entry | uprobe (L155) | No | - |
| SSL_write return | uretprobe (L200) | tup_from_ssl_ctx | Primary path |
| tcp_sendmsg | kprobe (L561) | map_ssl_ctx_to_sock | **Fallback (has race)** |
| SSL_read entry | uprobe (L89) | tup_from_ssl_ctx | Stores in fallback map |
| SSL_read return | uretprobe (L140) | tup_from_ssl_ctx | Primary path |
| tcp_recvmsg | kprobe (L13, tcp_recv.h) | **NONE** | **MISSING HOOK** |

---

## Recommendation

**Short term**: Consider Option 4 (disable fallback) if misattribution is worse than missing data.

**Long term**: Two fixes needed:
1. **Fix the race condition**: Implement Option 5 (bounded queue) or Option 3 (timestamp correlation)
2. **Fix the missing hook**: Add `map_ssl_ctx_to_sock(sk)` call to `kprobe/tcp_recvmsg`

### Proposed tcp_recvmsg Fix

Add to `native-tls.h`:
```c
SEC("kprobe/tcp_recvmsg")
int BPF_BYPASSABLE_KPROBE(kprobe__tcp_recvmsg_tls, struct sock *sk) {
    log_debug("kprobe/tcp_recvmsg_tls: sk=%p", sk);
    map_ssl_ctx_to_sock(sk);  // Same correlation as tcp_sendmsg
    return 0;
}
```

**Note**: This alone doesn't fix the race condition - it would just extend the race to the read path too. The bounded queue solution is needed for both paths.

The fallback was designed for "best effort" monitoring of pre-existing connections, but with 64-81% misattribution, it's causing more harm than good in multi-connection scenarios.

---

## Files

- `ssl_ctx_race.c` - Original test client (sequential mode)
- `ssl_ctx_race_v2.c` - Enhanced test client with multiple modes:
  - `--sequential`: Original behavior (write+read, write+read)
  - `--interleaved`: Maximize race window (write, write, read, read)
  - `--writes-only`: Isolate write path (all writes, then drain reads)
- `analyze_results.py` - Python script to analyze debug endpoint output
- `Makefile` - Build script (builds both test clients)
- `run_race_test.sh` - Automated test
- `manual_test.sh` - Step-by-step guide
- `INVESTIGATION.md` - This document

## Test Commands

```bash
# Build test clients
make

# Run with interleaved mode (recommended for race condition testing)
./ssl_ctx_race_v2 127.0.0.1 18001 127.0.0.1 18002 200 --interleaved

# Analyze results
sudo curl -s --unix-socket /opt/datadog-agent/run/sysprobe.sock \
    "http://unix/network_tracer/debug/http_monitoring" | python3 analyze_results.py
```