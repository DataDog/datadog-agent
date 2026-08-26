# Approach 3: Dummy Config via Existing Channels

## Core Idea

Reuse both existing IPC channels to achieve synchronous discovery:

- **`/configs`** (JMXFetch → Agent): Agent includes a "dummy discovery
  config" alongside regular scheduled configs. This dummy config carries
  the serialized service struct (host, ports) and a unique request ID.
- **`/status`** (JMXFetch → Agent): JMXFetch posts the discovery result
  (verified config JSON or failure) as part of its normal status flush.
- **`DiscoverConfig()`** blocks on a channel until the result arrives via
  `/status` (or times out).

No new endpoints. No new HTTP server. No new command-line params. The only
new things are: the agent adds discovery requests to the `/configs` response
and reads discovery results from the `/status` payload; JMXFetch recognizes
discovery configs and processes them specially.

## End-to-End Flow

```
 ┌─ Agent (Go) ──────────────────────────────────────────────────────┐
 │                                                                   │
 │  1. Discovery Worker calls JmxBridge.DiscoverConfig(name, svcJSON)│
 │     → creates discovery request with unique ID                    │
 │     → registers it in pending discovery map                       │
 │     → blocks on result channel (with timeout)                     │
 │                                                                   │
 │  2. JMXFetch polls GET /agent/jmx/configs                         │
 │     → getJMXConfigs handler includes pending discovery requests   │
 │       alongside regular scheduled configs                         │
 │     → response JSON has extra entries with check_name="__jmx_discovery__"│
 │       and the service struct embedded in instances[0]             │
 │                                                                   │
 │  3. JMXFetch processes the response                              │
 │     → recognizes __jmx_discovery__ configs                        │
 │     → does NOT create regular Instance objects for them           │
 │     → reads service struct (host, ports)                          │
 │     → tries candidate JMX ports: connect, inspect MBeans,         │
 │       match app signatures, run one collection iteration          │
 │     → builds result (config JSON on success, error on failure)    │
 │                                                                   │
 │  4. JMXFetch POSTs /agent/jmx/status                              │
 │     → status JSON includes a "discovery_results" array           │
 │       [{id: "<request_id>", config: [...] | null, error: null|msg}]│
 │                                                                   │
 │  5. Agent's setJMXStatus handler receives the POST                │
 │     → extracts discovery_results                                 │
 │     → for each result, looks up pending request by ID             │
 │     → sends result to the blocked DiscoverConfig()'s channel      │
 │                                                                   │
 │  6. DiscoverConfig() unblocks                                    │
 │     → returns config JSON (success) or error (failure)            │
 │     → Discovery Worker proceeds as normal:                        │
 │       - success → onDiscoveryResult → schedule config             │
 │       - failure → retry or drop silently                          │
 │                                                                   │
 └───────────────────────────────────────────────────────────────────┘
```

## Timing

```
Time  0s          0-15s         5-20s        5-20s
      │            │             │            │
      ▼            ▼             ▼            ▼
      DiscoverConfig()    JMXFetch polls   JMXFetch    Agent receives
      called, blocks      /configs         processes   /status, unblocks
      waiting              (≤15s latency)   probe       DiscoverConfig()
```

- Worst case: 15s (poll interval) + 5s (connect + inspect + collect) = 20s
- Best case: immediate poll + 5s = 5s
- `DiscoverConfig()` has a timeout (e.g., 60s) — if JMXFetch isn't running
  or doesn't respond, the probe fails and the worker retries

## Agent Side

### 1. Discovery request registry (`pkg/jmxfetch/discovery.go`)

New file. Manages pending discovery requests:

```go
type DiscoveryRequest struct {
    ID           string
    Integration  string
    ServiceJSON  string
    Result       chan DiscoveryResult
}

type DiscoveryResult struct {
    ConfigJSON string  // empty on failure
    Error      error   // nil on success
}

var (
    pendingDiscovery      = map[string]*DiscoveryRequest{}
    pendingDiscoveryMutex = &sync.Mutex{}
)

func RegisterDiscoveryRequest(req *DiscoveryRequest) { ... }
func UnregisterDiscoveryRequest(id string)         { ... }
func GetPendingDiscoveryRequests() []*DiscoveryRequest { ... }
func CompleteDiscoveryRequest(id string, result DiscoveryResult) { ... }
```

### 2. Modified `/configs` handler (`agent_jmx.go`)

`getJMXConfigs` calls `jmxfetch.GetIntegrations()` as before, then appends
pending discovery requests as special config entries:

```go
// After regular configs, add pending discovery requests
for _, req := range jmxfetch.GetPendingDiscoveryRequests() {
    config := map[string]interface{}{
        "check_name": "__jmx_discovery__",
        "init_config": map[string]interface{}{},
        "instances": []interface{}{
            map[string]interface{}{
                "discovery_id":      req.ID,
                "integration":       req.Integration,
                "service":           json.RawMessage(req.ServiceJSON),
            },
        },
    }
    configs["__discovery__"+req.ID] = config
}
```

The discovery requests are ephemeral — they're not in the scheduled configs
cache, so they don't show up in Fleet Automation status.

### 3. Modified `/status` handler (`agent_jmx.go`)

`setJMXStatus` decodes the status JSON as before, then checks for a
`discovery_results` field:

```go
// After existing status handling
if results, ok := status["discovery_results"]; ok {
    for _, r := range results.([]interface{}) {
        result := r.(map[string]interface{})
        id := result["id"].(string)
        
        var dr jmxfetch.DiscoveryResult
        if config, ok := result["config"]; ok && config != nil {
            configJSON, _ := json.Marshal(config)
            dr.ConfigJSON = string(configJSON)
        } else if errMsg, ok := result["error"]; ok && errMsg != nil {
            dr.Error = fmt.Errorf("%v", errMsg)
        }
        jmxfetch.CompleteDiscoveryRequest(id, dr)
    }
}
```

### 4. JMX discovery bridge (`discoverer/jmx_bridge.go`)

```go
func (b *jmxBridge) DiscoverConfig(integrationName, serviceJSON string) (string, error) {
    if !isJMXIntegration(integrationName) {
        return "", fmt.Errorf("not a JMX integration: %s", integrationName)
    }

    id := generateUUID()
    req := &jmxfetch.DiscoveryRequest{
        ID:          id,
        Integration: integrationName,
        ServiceJSON: serviceJSON,
        Result:      make(chan jmxfetch.DiscoveryResult, 1),
    }
    jmxfetch.RegisterDiscoveryRequest(req)
    defer jmxfetch.UnregisterDiscoveryRequest(id)

    select {
    case result := <-req.Result:
        if result.Error != nil {
            return "", result.Error  // worker will retry
        }
        return result.ConfigJSON, nil
    case <-time.After(discoveryTimeout):
        return "", fmt.Errorf("jmx discovery timed out after %v", discoveryTimeout)
    }
}
```

The bridge blocks the discovery worker goroutine until JMXFetch responds
(or times out). This is the same synchronous semantics as the Python bridge.

### 5. Blocking concern

The discovery worker (`worker.go`) has a fixed number of goroutines
(default 4). A blocked JMX probe occupies one goroutine for up to 20s. If
multiple JMX services are discovered simultaneously, this could block all
workers and delay Python discovery probes.

**Mitigation for PoC:** Acceptable — discovery is a one-time operation per
service, and the timeout prevents indefinite blocking.

**Mitigation for production:** The composite discoverer could use a
separate `Worker` instance for JMX probes, with its own workqueue and
worker count. This is a small refactor: `initDiscoveryWorker` would create
two workers instead of one, and `scheduleDiscovery` would route to the
right one based on the integration name.

### 6. Status struct change (`pkg/status/jmx/jmx.go`)

Add `DiscoveryResults` field to the `Status` struct:

```go
type Status struct {
    Info              map[string]interface{} `json:"info"`
    ChecksStatus      jmxCheckStatus         `json:"checks"`
    Timestamp         int64                  `json:"timestamp"`
    Errors            int64                  `json:"errors"`
    DiscoveryResults  []DiscoveryResult      `json:"discovery_results,omitempty"`
}

type DiscoveryResult struct {
    ID     string          `json:"id"`
    Config json.RawMessage `json:"config,omitempty"`
    Error  string          `json:"error,omitempty"`
}
```

---

## JMXFetch Side

### 1. Recognize discovery configs (`App.java`)

In `init()`, when processing `adJsonConfigs`, check for
`check_name == "__jmx_discovery__"`. If found:

- Do NOT create a regular `Instance` for it
- Extract the `discovery_id`, `integration`, and `service` from the
  instance config
- Run the discovery probe (see below)
- Store the result for the next `status.flush()`

```java
// In init(), after processing regular adJsonConfigs:
processDiscoveryConfigs(adJsonConfigs);
```

### 2. Discovery probe (`App.java` or new `JmxDiscoveryProbe.java`)

```
For each discovery config received:
  1. Parse service JSON → {host, ports[]}
  2. Build candidate port list:
     - Ports named "jmx" or "jmx-rmi" first
     - Then common JMX ports: 9999, 9010, 1099, 7199
     - Then all other exposed ports
  3. For each candidate port:
     a. Create JMX connection to host:port
     b. If connection fails → next port
     c. Query MBean domains
     d. Match against AppSignatures (JmxDiscovery)
     e. If matched:
        - Build instance config: {host, port, collect_default_jvm_metrics: true}
        - Build init_config: {is_jmx: true, collect_default_metrics: true,
          new_gc_metrics: true, conf: [matched signature's conf entries]}
        - Create temporary Instance, init it, run one collection iteration
        - If metrics > 0:
          - Store result: {id, config: [{instances: [...], init_config: {...}}]}
          - Clean up, break
     f. If not matched → next port
  4. If nothing matched:
     - Store result: {id, error: "no JMX endpoint found"}
```

### 3. Report results via `/status` (`Status.java`)

Add a `discoveryResults` list to `Status`:

```java
private List<Map<String, Object>> discoveryResults = new ArrayList<>();

public void addDiscoveryResult(String id, Object config, String error) {
    Map<String, Object> result = new HashMap<>();
    result.put("id", id);
    if (config != null) {
        result.put("config", config);
    }
    if (error != null) {
        result.put("error", error);
    }
    discoveryResults.add(result);
}
```

In `generateJson()`, add:

```java
if (!discoveryResults.isEmpty()) {
    status.put("discovery_results", discoveryResults);
}
```

After `flush()`, clear `discoveryResults`.

### 4. Lifecycle

Discovery probes run during the normal `init()` call (when JMXFetch detects
new configs via polling). They execute synchronously in the init phase,
before the collection loop. Results are flushed via the next `status.flush()`
call, which happens at the end of each iteration.

Timeline within one JMXFetch cycle:
```
1. Poll /configs → get new discovery config + regular configs
2. init(true)
   a. Process regular configs → create Instances
   b. Process discovery configs → run probes, store results
3. doIteration() → collect metrics from regular Instances
4. status.flush() → POST /status with regular status + discovery_results
5. Sleep for check_period
```

### 5. No changes to regular config processing

Discovery configs are completely separate from regular configs. They don't
create Instances, don't generate service checks, and don't appear in the
regular status. The only touchpoint is the `discovery_results` field in the
status POST.

---

## Error Handling

| Scenario | JMXFetch | /status response | Bridge return | Worker behavior |
|---|---|---|---|---|
| JMX found, app detected, metrics > 0 | Config JSON | `{id, config: [...]}` | config JSON | Schedule config |
| JMX found, app detected, 0 metrics | Error | `{id, error: "no metrics"}` | error | Retry |
| JMX found, no app match | Error | `{id, error: "no app match"}` | error | Retry |
| JMX not reachable (all ports) | Error | `{id, error: "connection refused"}` | error | Retry |
| JMXFetch not running | No response | N/A | Timeout error | Retry |
| Bridge timeout | N/A | N/A | Timeout error | Retry |
| Max retries exceeded | N/A | N/A | N/A | Drop silently |

**Key property: no config is scheduled until verified. No error service
checks are generated during discovery. Discovery telemetry works because
the worker records success/failure for each probe.**

---

## Pros / Cons

| Pros | Cons |
|---|---|
| No new channels or endpoints | Latency: ≤15s poll + 5s probe = ≤20s |
| Config NOT scheduled before verification | Blocks a worker goroutine during wait |
| Agent knows if discovery succeeded | Requires separate workqueue for JMX (production) |
| Discovery telemetry works | Status struct needs new field |
| Works within existing architecture | /configs and /status payloads grow slightly |
| Minimal JMXFetch changes | |
| No error service checks on failure | |
| Fleet Automation status stays clean | |

---

## Comparison with Other Approaches

| Aspect | Approach 1 (HTTP server) | Approach 2 (error suppression) | **Approach 3 (dummy config)** |
|---|---|---|---|
| New channels | Yes (HTTP server) | No | **No** |
| Config scheduled before verify | No | Yes | **No** |
| Agent knows discovery result | Yes | No | **Yes** |
| Discovery telemetry | Yes | No | **Yes** |
| Error service checks on failure | No | Suppressed | **No** |
| Latency | Low (synchronous) | High (15s poll) | **Medium (≤20s)** |
| JMXFetch changes | Large (HTTP server) | Small (error suppression) | **Medium (probe logic)** |
| Agent changes | Small | Small | **Medium (request registry, handlers)** |
| Fleet Automation status | Clean | Shows unverified config | **Clean** |
