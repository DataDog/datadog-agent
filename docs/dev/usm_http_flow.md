# USM HTTP Flow Documentation

This document traces the complete lifecycle of an HTTP request through the Datadog Agent's Universal Service Monitoring (USM) system, from eBPF capture through encoding and service attribution.

## Flow Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│ 1. eBPF CAPTURE (Socket Filter / TLS Uprobe)                        │
│    pkg/network/ebpf/c/protocols/http/http.h                          │
│    - Captures HTTP request/response from socket                      │
│    - Stores in http_in_flight map (conn_tuple → http_transaction_t) │
│    - Flushes complete transactions to userspace                      │
└─────────────────────────────────────────────────────────────────────┘
                                 ↓
┌─────────────────────────────────────────────────────────────────────┐
│ 2. EBPF → USERSPACE (Perf Buffer / Batch Map)                       │
│    pkg/network/protocols/events/                                     │
│    - Events flow via perf buffer (kernel ≥ 5.8)                     │
│    - Or batched in map and polled (older kernels)                   │
└─────────────────────────────────────────────────────────────────────┘
                                 ↓
┌─────────────────────────────────────────────────────────────────────┐
│ 3. USERSPACE PROCESSING (StatKeeper)                                │
│    pkg/network/protocols/http/statkeeper.go                          │
│    - Extracts path from request_fragment                             │
│    - Quantizes path (cardinality reduction)                          │
│    - Constructs http.Key (ConnectionKey + path + method)             │
│    - Aggregates stats: map[http.Key]*RequestStats                    │
└─────────────────────────────────────────────────────────────────────┘
                                 ↓
┌─────────────────────────────────────────────────────────────────────┐
│ 4. CONNECTION EXTRACTION (Tracer)                                    │
│    pkg/network/tracer/connection/ebpf_tracer.go                      │
│    - Reads tcp_stats/udp_stats maps                                  │
│    - Extracts PID from conn_tuple                                    │
│    - Creates ConnectionStats (includes PID, ContainerID)             │
└─────────────────────────────────────────────────────────────────────┘
                                 ↓
┌─────────────────────────────────────────────────────────────────────┐
│ 5. SERVICE ATTRIBUTION                                               │
│    pkg/network/tracer/process_cache.go                               │
│    - Maps PID → Process metadata (ContainerID, Tags)                │
│    - Resolves container → pod → service                              │
│    - Populates ConnectionStats.ContainerID.Source/Dest               │
└─────────────────────────────────────────────────────────────────────┘
                                 ↓
┌─────────────────────────────────────────────────────────────────────┐
│ 6. USM LOOKUP (GroupByConnection)                                   │
│    pkg/network/encoding/marshal/usm.go                               │
│    - Groups HTTP stats by ConnectionKey (no PID!)                    │
│    - map[ConnectionKey]*USMConnectionData                            │
│    - USMConnectionData contains all http.Keys for that connection    │
└─────────────────────────────────────────────────────────────────────┘
                                 ↓
┌─────────────────────────────────────────────────────────────────────┐
│ 7. CONNECTION → HTTP STATS MATCHING (WithKey + IsPIDCollision)      │
│    pkg/network/usm_connection_keys.go + usm.go                       │
│    - For each ConnectionStats, try 4 lookup strategies:              │
│      1. NAT (client, server)                                         │
│      2. Normal (client, server)                                      │
│      3. NAT reversed (server, client)                                │
│      4. Normal reversed (server, client)                             │
│    - First match wins                                                │
│    - IsPIDCollision() prevents double-counting                       │
└─────────────────────────────────────────────────────────────────────┘
                                 ↓
┌─────────────────────────────────────────────────────────────────────┐
│ 8. ENCODING (HTTP Encoder)                                          │
│    pkg/network/encoding/marshal/usm_http.go                          │
│    - Attaches HTTP stats to connection                               │
│    - Encodes as ProtoBuf (HTTPAggregations)                          │
│    - Final payload sent to Datadog backend                           │
└─────────────────────────────────────────────────────────────────────┘
```

## Detailed Stage Breakdown

### 1. eBPF Capture Layer

**Location:** `/pkg/network/ebpf/c/protocols/http/`

**Key Data Structures:**

```c
typedef struct {
    __u64 request_started;           // Timestamp of first request segment
    __u64 response_last_seen;        // Timestamp of last response segment
    __u16 response_status_code;      // HTTP status code (200, 404, etc.)
    __u8 request_method;             // GET, POST, PUT, DELETE, etc.
    char request_fragment[208];      // First ~208 bytes of HTTP request
    __u32 tcp_seq;                   // TCP sequence for deduplication
    __u64 tags;                      // Protocol tags bitfield
} http_transaction_t;

typedef struct {
    conn_tuple_t tuple;              // 4-tuple (no PID in socket filter context!)
    http_transaction_t http;
} http_event_t;
```

**eBPF Programs:**

1. **socket__http_filter** - Plain HTTP from socket filter
   - Reads HTTP payload via `read_into_buffer_skb()`
   - Processes with `http_process()`

2. **uprobe__http_process** - Encrypted HTTP from TLS uprobe
   - Reads from userspace buffer via `read_into_user_buffer_http()`
   - Processes with `http_process()`

3. **uprobe__http_termination** - TLS connection close
   - Flushes pending transactions

**Processing Logic (http_process):**

```c
http_process(buffer, skb_info, tuple):
  1. Parse packet type (REQUEST/RESPONSE/UNKNOWN)
     - Match against byte patterns: "GET ", "POST ", "HTTP/", etc.

  2. Fetch or create state in http_in_flight map
     - Key: conn_tuple_t
     - Value: http_transaction_t

  3. Check if seen before (TCP seq deduplication)
     - Avoid re-processing retransmitted segments
     - Handle keep-alive connection reuse

  4. Determine if should flush previous transaction
     - New REQUEST + existing transaction → flush
     - New RESPONSE + existing response → flush (pipelining)

  5. Populate transaction fields
     - http_begin_request(): Copy method, path, timestamp
     - http_begin_response(): Parse status code from bytes[9:12]

  6. Send to userspace if complete
     - http_batch_enqueue_wrapper()
     - Direct consumer (perf buffer) or batch consumer (map)

  7. Clean up on termination (FIN/RST)
     - tcp_seq set to HTTP_TERMINATING (0xFFFFFFFF)
     - Delete map entry
```

**Critical Detail:** Socket filter programs do NOT have PID context. The `conn_tuple_t` includes netns and protocol but connection identity is only the 4-tuple (src IP, dst IP, src port, dst port).

---

### 2. eBPF → Userspace Communication

**Location:** `/pkg/network/protocols/events/`

**Two Paths:**

1. **Direct Consumer (kernel ≥ 5.8):**
   - eBPF writes to perf ring buffer
   - Userspace reads continuously
   - Lower latency

2. **Batch Consumer (kernel < 5.8):**
   - eBPF writes to `http_batch_events` map
   - Network tracepoint triggers flush
   - Userspace reads batch

**Event Payload:** Complete `http_event_t` struct (connection tuple + transaction)

---

### 3. Userspace Processing (StatKeeper)

**Location:** `/pkg/network/protocols/http/statkeeper.go`

**Processing Steps:**

```go
StatKeeper.Process(tx):
  1. Check completeness
     - Skip if only request OR only response
     - Store incomplete for later joining

  2. Extract path from request_fragment
     - Remove query string: /orders?id=123 → /orders

  3. Quantize path (cardinality reduction)
     - /orders/123/view → /orders/*/view
     - Configurable via quantization rules

  4. Apply replace rules
     - Pattern matching on path
     - Can drop or rename endpoints

  5. Validate transaction
     - Method != UNKNOWN
     - Latency > 0
     - Valid status code

  6. Construct aggregation key
     key = http.Key{
       ConnectionKey: types.ConnectionKey{...},  // 4-tuple (no PID!)
       Path: PathStat{Content: quantizedPath, FullPath: rawPath},
       Method: GET/POST/etc.
     }

  7. Aggregate stats
     stats[key].AddRequest(statusCode, latency, tags)
```

**Result:** `map[http.Key]*RequestStats`

**Key Data Structure:**
```go
type Key struct {
    ConnectionKey types.ConnectionKey  // (src IP, dst IP, src port, dst port)
    Path          PathStat             // (quantized path, full path)
    Method        Method               // HTTP method enum
}

type ConnectionKey struct {
    SrcIPHigh, SrcIPLow uint64
    DstIPHigh, DstIPLow uint64
    SrcPort, DstPort uint16
    // NOTE: NO PID FIELD!
}
```

---

### 4. Connection Extraction

**Location:** `/pkg/network/tracer/connection/ebpf_tracer.go`

**Process:**

```go
EbpfTracer.GetConnections():
  - Read tcp_stats/udp_stats eBPF maps
  - For each conn_tuple entry:
    - Extract PID from conn_tuple (present in tcp_stats)
    - Create ConnectionStats:
      * Source, Dest, SPort, DPort
      * Pid ← from tcp_stats map key
      * Family, Type, Direction
      * Monotonic/Last traffic stats
  - Return []ConnectionStats
```

**ConnectionStats Structure:**
```go
type ConnectionStats struct {
    ConnectionTuple                  // Source, Dest, Pid, NetNS, SPort, DPort, etc.

    ContainerID struct {
        Source, Dest *intern.Value   // Container IDs (nil = host)
    }
    Tags []*intern.Value             // Process/container tags

    IPTranslation *IPTranslation     // NAT translation (Repl fields)
    Via *Via                          // Routing info

    // ... traffic stats, DNS stats, TCP failures, etc.
}
```

**Critical:** At this point we have PID, but HTTP stats don't.

---

### 5. Service Attribution

**Location:** `/pkg/network/tracer/process_cache.go`, `/pkg/network/resolver.go`

**Process Cache:**

```go
ProcessCache: map[(pid, startTime)] → ProcessMetadata {
    Pid int32
    ContainerID string
    Tags []string        // From orchestration: K8s service, pod, etc.
    ExecPath string
}

On process event:
  - Store metadata keyed by (pid, startTime)

When enriching connection:
  - Look up ConnectionStats.Pid in cache
  - Find closest timestamp match
  - Copy ContainerID → ConnectionStats.ContainerID.Source
  - Copy Tags → ConnectionStats.Tags
```

**Local Container Resolution:**

For localhost connections, resolve destination container:

```go
LocalResolver.Resolve(connections):
  # Phase 1: Index source containers by reversed tuple
  for conn with ContainerID.Source:
    reversedKey = (conn.Dest, conn.Source, conn.DPort, conn.SPort, netns)
    containerIndex[reversedKey] = conn.ContainerID.Source

  # Phase 2: Resolve destinations
  for local conn without ContainerID.Dest:
    key = (conn.Source, conn.Dest, conn.SPort, conn.DPort, netns)
    conn.ContainerID.Dest = containerIndex[key]
```

**Result:** Each `ConnectionStats` now has PID, ContainerID, and Tags (service name).

---

### 6. USM Lookup - Group HTTP Stats by Connection

**Location:** `/pkg/network/encoding/marshal/usm.go:60-103`

**Process:**

```go
GroupByConnection(httpStats map[http.Key]*RequestStats):

  # Two-pass algorithm for efficiency

  Pass 1: Count HTTP stats per ConnectionKey
    for httpKey in httpStats:
      connKey = httpKey.ConnectionKey  // Extract 4-tuple
      counts[connKey]++

  Pass 2: Build aggregated structure
    result = map[ConnectionKey]*USMConnectionData
    for httpKey, stats in httpStats:
      connKey = httpKey.ConnectionKey
      result[connKey].Data.append((httpKey, stats))

  return USMConnectionIndex{data: result}
```

**Result Structure:**
```go
type USMConnectionIndex[K, V] struct {
    data map[ConnectionKey]*USMConnectionData[K, V]
}

type USMConnectionData[K, V] struct {
    Data []USMKeyValue[K, V]     // All HTTP stats for this 4-tuple
    sport, dport uint16           // For IsPIDCollision detection
    claimed bool                  // Orphan tracking
}
```

**Critical:** Multiple PIDs can have the same ConnectionKey (4-tuple without PID).

---

### 7. Connection → HTTP Stats Matching

**The 4 Lookup Strategies:**

**Location:** `/pkg/network/usm_connection_keys.go:54-95`

```go
WithKey(connectionStats, callback):

  # Extract addresses
  clientIP, clientPort = conn.Source, conn.SPort
  serverIP, serverPort = conn.Dest, conn.DPort

  # If NAT exists, get translated addresses
  if conn.IPTranslation != nil:
    clientIPNAT, clientPortNAT = GetNATLocalAddress(conn)
    serverIPNAT, serverPortNAT = GetNATRemoteAddress(conn)

  # Heuristic: Flip if client port not ephemeral
  # (assumes client uses ephemeral, server uses fixed port)
  if !IsPortInEphemeralRange(clientPort):
    swap(clientIP, serverIP, clientPort, serverPort)
    if hasNAT:
      swap(clientIPNAT, serverIPNAT, clientPortNAT, serverPortNAT)

  # Try 4 lookups (stop on first match):

  1. NATed (client → server):
     key = ConnectionKey(clientIPNAT, serverIPNAT, clientPortNAT, serverPortNAT)
     if callback(key): return

  2. Normal (client → server):
     key = ConnectionKey(clientIP, serverIP, clientPort, serverPort)
     if callback(key): return

  3. NATed reversed (server → client):
     key = ConnectionKey(serverIPNAT, clientIPNAT, serverPortNAT, clientPortNAT)
     if callback(key): return

  4. Normal reversed (server → client):
     key = ConnectionKey(serverIP, clientIP, serverPort, clientPort)
     if callback(key): return
```

**IsPIDCollision Check:**

**Location:** `/pkg/network/encoding/marshal/usm.go:164-190`

```go
USMConnectionData.IsPIDCollision(conn ConnectionStats) bool:

  if sport == 0 && dport == 0:
    # First connection claiming this HTTP data
    sport, dport = conn.SPort, conn.DPort
    return false  // Not a collision

  if conn.SPort == dport && conn.DPort == sport:
    # Ports are swapped (opposite ends of same TCP connection)
    # Common in localhost connections
    return false  // Allow both ends

  # Different connection trying to claim same HTTP stats
  return true  // Collision! Skip this connection
```

**Purpose:** Prevent overcounting in pre-forked servers (NGINX workers sharing listen socket).

**BUG:** Only checks if ports are swapped, doesn't verify IPs or PIDs match!

---

### 8. Encoding

**Location:** `/pkg/network/encoding/marshal/usm_http.go`, `format.go`

**HTTP Encoding:**

```go
HTTPEncoder.EncodeConnection(conn, builder):

  # Lookup HTTP stats for this connection
  httpData = byConnection.Find(conn)
  if httpData == nil:
    return  # No HTTP stats

  # Check PID collision
  if httpData.IsPIDCollision(conn):
    return  # Skip due to collision

  # Encode all HTTP stats for this connection
  for kvPair in httpData.Data:
    httpKey = kvPair.Key          // http.Key (path + method)
    stats = kvPair.Value          // RequestStats (counts, latencies)

    builder.AddEndpointAggregation:
      SetPath(httpKey.Path.Content)      // Quantized
      SetFullPath(httpKey.Path.FullPath) // Original
      SetMethod(httpKey.Method)

      for statusCode, statsData in stats.Data:
        AddStatsByStatusCode:
          SetKey(statusCode)
          SetCount(statsData.Count)
          SetLatencies(statsData.Latencies)
```

**Connection Formatting:**

```go
FormatConnection(conn, usmEncoders):

  # Set connection fields
  builder.SetPid(conn.Pid)
  builder.SetLaddr(ip=conn.Source, port=conn.SPort,
                   containerId=conn.ContainerID.Source)
  builder.SetRaddr(ip=conn.Dest, port=conn.DPort,
                   containerId=conn.ContainerID.Dest)

  # Set NAT translation
  if conn.IPTranslation:
    builder.SetIpTranslation(...)

  # CRITICAL: Call all USM encoders
  for encoder in [httpEncoder, http2Encoder, kafkaEncoder, ...]:
    encoder.EncodeConnection(conn, builder)
    # This attaches protocol-specific data to the connection

  # Finalize
  builder.SetTags(conn.Tags)
  return builder.Build()
```

**Result:** ProtoBuf payload with HTTP stats attached to the connection, attributed to the service from `conn.ContainerID.Source`.

---

## Decision Points for Misattribution

### 1. eBPF Level
- **TCP seq deduplication:** `http_seen_before()` may miss retransmitted packets
- **Request/response pairing:** Keep-alive connections assume seq tracking works
- **Race conditions:** Socket filter vs TLS uprobe may capture same request twice

### 2. Userspace Lookup (WithKey)
- **Ephemeral port heuristic:** May guess wrong direction if non-standard ports
- **NAT lookup order:** Tries 4 variations, first match wins (could be wrong one)
- **IsPIDCollision bug:** Only checks ports swapped, not IPs/PIDs

### 3. Container Resolution
- **Process cache TTL:** Metadata may expire before connection finalized
- **Localhost resolution:** Only works for `IntraHost` connections
- **Metadata lag:** Container→pod→service mapping may be stale

### 4. HTTP Stats Aggregation
- **Incomplete transactions:** Request/response stored separately, joined later
- **Path quantization:** Changes original path (may mask issues)
- **ConnectionKey without PID:** Multiple processes share same key

### 5. Encoding
- **First PID claims:** In collision scenario, first process claims HTTP stats
- **No tuple validation:** HTTP stats attached even if tuple doesn't match exactly
- **Lookup strategy order:** NAT lookup tried first, may match wrong connection

---

## Key Files Reference

| Stage | File Path | Purpose |
|-------|-----------|---------|
| **eBPF Capture** |
| Main program | `pkg/network/ebpf/c/protocols/http/http.h` | Socket filter, uprobe programs |
| Data structures | `pkg/network/ebpf/c/protocols/http/types.h` | http_transaction_t, http_event_t |
| Entry points | `pkg/network/ebpf/c/runtime/usm.c` | Program registration |
| **Userspace Processing** |
| Event consumer | `pkg/network/protocols/http/protocol.go` | Consumer init, event loop |
| Stats aggregation | `pkg/network/protocols/http/statkeeper.go` | HTTP stat processing |
| Event model | `pkg/network/protocols/http/model_linux.go` | Event interpretation |
| **Connection Enrichment** |
| Main tracer | `pkg/network/tracer/tracer.go` | Connection extraction loop |
| Process metadata | `pkg/network/tracer/process_cache.go` | PID → container mapping |
| Container resolution | `pkg/network/resolver.go` | Localhost resolution |
| **USM Lookup & Encoding** |
| Grouping | `pkg/network/encoding/marshal/usm.go` | GroupByConnection, IsPIDCollision |
| HTTP encoding | `pkg/network/encoding/marshal/usm_http.go` | HTTP stats encoding |
| Connection formatting | `pkg/network/encoding/marshal/format.go` | Final payload construction |
| Lookup strategy | `pkg/network/usm_connection_keys.go` | WithKey() 4 lookups |
| **Data Structures** |
| Connection stats | `pkg/network/event_common.go` | ConnectionStats, IPTranslation |
| Connection key | `pkg/network/types/connection_key.go` | ConnectionKey (4-tuple) |

---

## Logging Strategy for Debugging

See `docs/dev/usm_http_instrumentation.md` for detailed instrumentation plan.