# Approach 4: Dedicated Pull Endpoints on Existing Agent IPC Server

## Core Idea

Add two new endpoints to the Agent's existing HTTPS IPC server — the same
server that already serves `/agent/jmx/configs` and `/agent/jmx/status`.
JMXFetch remains the HTTP client; no new server or port on JMXFetch.

```
Agent (Go)                         JMXFetch (Java subprocess)
┌──────────────┐                   ┌──────────────┐
│  IPC Server   │ ◀── HTTPS ────── │  HttpClient   │
│  :5001        │    GET /configs  │               │
│  (cmd_port)   │    POST /status  │               │
│               │                   │               │
│  NEW:         │ ◀── HTTPS ────── │               │
│  GET /jmx/    │    GET /discovery │               │
│  discovery/   │       /requests  │               │
│  requests     │                   │               │
│               │ ◀── HTTPS ────── │               │
│  POST /jmx/   │    POST /discovery│               │
│  discovery/   │       /results   │               │
│  results      │                   │               │
└──────────────┘                   └──────────────┘
```

## Protocol

### `GET /agent/jmx/discovery/requests?cursor=N`

Returns pending discovery requests with sequence numbers > N.

Response (200):
```json
{
  "cursor": 42,
  "requests": [
    {
      "id": "uuid-1",
      "integration": "kafka",
      "service": {
        "id": "docker://abc123",
        "host": "172.17.130.4",
        "ports": [{"number": 9092, "name": ""}, {"number": 9999, "name": "jmx"}]
      }
    }
  ]
}
```

Response (204): no new requests since cursor.

The cursor is a monotonic sequence number owned by the discovery request
registry, not `time.Now().Unix()`. This fixes the timestamp resolution
problem identified in the critique.

### `POST /agent/jmx/discovery/results`

JMXFetch posts discovery results. The agent acknowledges receipt.

Request:
```json
{
  "id": "uuid-1",
  "config": [
    {
      "instances": [{"host": "172.17.130.4", "port": 9999}],
      "init_config": {"is_jmx": true}
    }
  ]
}
```

Or on failure:
```json
{
  "id": "uuid-1",
  "error": "no JMX endpoint found"
}
```

Response (200): acknowledged. The agent retains the result until
acknowledged, so a transient HTTP failure doesn't lose it.

### Cancellation

When a service disappears, the agent adds a tombstone to the requests
response:
```json
{
  "cursor": 43,
  "requests": [],
  "cancelled": ["uuid-1"]
}
```

JMXFetch sees the cancellation and aborts any in-flight probe for that
request.

## Agent Side

### Discovery request registry (`pkg/jmxfetch/discovery.go`)

Same as Approach 3, but with monotonic sequence numbers:

```go
type DiscoveryRequest struct {
    Seq          int64
    ID           string
    Integration  string
    ServiceJSON  string
    Result       chan DiscoveryResult
    Cancelled    bool
}

var (
    pendingDiscovery      = map[string]*DiscoveryRequest{}
    pendingDiscoveryMutex = &sync.Mutex{}
    discoverySeq           int64  // monotonic counter
)
```

### New endpoints (`agent_jmx.go`)

```go
func getJMXDiscoveryRequests(w http.ResponseWriter, r *http.Request) {
    cursor, _ := strconv.ParseInt(r.URL.Query().Get("cursor"), 10, 64)
    requests := jmxfetch.GetPendingDiscoveryRequests(cursor)
    // ... marshal and respond
}

func postJMXDiscoveryResults(w http.ResponseWriter, r *http.Request) {
    // ... decode result, complete the pending request
}
```

### JMX bridge (`discoverer/jmx_bridge.go`)

Same blocking pattern as Approach 3 — register request, block on channel,
return result. But delivery and cancellation are now reliable.

### JMXFetch startup

The critique identified a bootstrap deadlock: JMXFetch only starts when a
config is scheduled, but discovery happens before any config is scheduled.

**Fix**: `EnsureJMXFetchRunning()` — called by the bridge before
registering a discovery request. This starts JMXFetch if it's not already
running, even if no regular configs are scheduled yet.

## JMXFetch Side

### Separate discovery executor

The critique identified that running discovery probes in `init()` disrupts
all running integrations (reinit flag). The fix: a separate bounded
executor for discovery probes that never participates in normal instance
reinitialization or collection.

```java
// In App.java
private ExecutorService discoveryExecutor;

// Discovery probes run on a separate thread pool
public void processDiscoveryRequest(DiscoveryRequest req) {
    discoveryExecutor.submit(() -> {
        try {
            // 1. Parse service JSON
            // 2. Try candidate ports
            // 3. Connect, inspect MBeans
            // 4. Verify application-specific metrics
            // 5. Build result
            // 6. POST to /agent/jmx/discovery/results
        } catch (Exception e) {
            // POST error result
        }
    });
}
```

### Polling loop

JMXFetch's main loop adds a poll for discovery requests alongside the
existing `/configs` poll:

```java
// In App.start()
while (true) {
    // Existing: poll /configs
    getJsonConfigs();

    // NEW: poll /discovery/requests
    getDiscoveryRequests();

    // ... rest of main loop
}
```

The discovery poll is separate from the config poll, so adding/removing
discovery requests doesn't trigger reinit of regular configs.

### Application-specific validation

The critique identified that `metric_count > 0` is insufficient because
any JVM produces JVM metrics. The fix: use the integration's own
`metrics.yaml` to verify that application-specific metrics are collected.

The discovery probe:
1. Connects to the JMX endpoint
2. Checks for the integration's expected MBean domains (e.g.,
   `kafka.server` for Kafka)
3. Creates a temporary Instance with the **integration's real
   `init_config`** (including `metrics.yaml` conf entries)
4. Runs one collection iteration
5. Requires at least one **application-specific** metric (not just JVM
   metrics)

### Integration-owned discovery data

The critique suggested storing integration-owned data in integrations-core
instead of hardcoding in JMXFetch. Each integration would provide:

- Preferred/known ports
- Identity ObjectNames or domains
- Required application-specific metric selectors

This avoids the hardcoded `JmxDiscovery.java` registry and keeps behavior
versioned with each integration.

## Comparison

| Aspect | Approach 0 (one-shot) | Approach 4 (pull endpoints) |
|---|---|---|
| JVM startup cost | Yes (~1–2s per probe) | No (reuses running JVM) |
| New endpoints | No | Yes (2 new on Agent IPC) |
| New JMXFetch server | No | No |
| Bootstrap deadlock | No (subprocess is independent) | Needs EnsureJMXFetchRunning() |
| Reinit churn | No (separate process) | No (separate executor) |
| Reliable delivery | Yes (stdout + exit code) | Yes (ack protocol) |
| Latency | ~2s (JVM startup + probe) | ~15s (poll interval) + probe |
| Complexity | Low | Medium |
| Production-ready | Yes (with bounded concurrency) | Yes |
