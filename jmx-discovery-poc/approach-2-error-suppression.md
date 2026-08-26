# JMX Configuration Discovery — Design (v2, existing-architecture)

## Constraint

The only interface between Agent and JMXFetch is two HTTP endpoints, both
initiated by JMXFetch:

- `GET /agent/jmx/configs` — JMXFetch polls for scheduled configs (every 15s)
- `POST /agent/jmx/status` — JMXFetch posts status back

No Agent → JMXFetch channel exists. We must work within this.

## Design: Discovery Flag + Error Suppression

### Core Idea

The agent schedules a **candidate config** via the normal JMX scheduler
(through the discovery worker, same as Python integrations). The instance
carries a `discovery: true` flag. JMXFetch picks it up via `/configs`
polling and handles it as follows:

- **Connection fails** → silently retry next cycle. No error service checks,
  no error status. The instance stays in `brokenInstanceMap` but
  `processStatus` and `sendServiceCheck` are skipped for discovery instances.
- **Connection succeeds** → inspect MBean domains, match app signatures,
  add conf entries, start collecting metrics normally. Report OK status.

This means:
- No new HTTP server on JMXFetch
- No new endpoints
- The only JMXFetch change is suppressing error side-effects for discovery
  instances
- Higher latency (15s poll interval) but acceptable for discovery

### Why this is acceptable

The Python discovery pattern verifies the config synchronously before
scheduling. We can't do that with JMXFetch without adding a new channel.
Instead, we schedule the candidate and let JMXFetch verify asynchronously.
The key property is preserved: **no error service checks are generated
when discovery fails**. The user sees nothing until the config is verified
and metrics start flowing.

### Comparison with Python discovery

| Aspect | Python | JMX (this design) |
|---|---|---|
| Config generation | `candidates(service)` generates candidates | JMX bridge generates candidate from service ports |
| Verification | `check.run()` synchronously | JMXFetch connects + collects one iteration asynchronously |
| Error on failure | Silent retry, no config scheduled | Silent retry, config stays but no error service checks |
| Latency | Low (synchronous) | Higher (15s poll interval) |
| New channels | None | None |

---

## Agent Side

### 1. JMX discovery bridge (`discoverer/jmx_bridge.go`)

The bridge implements `ConfigDiscoverer.DiscoverConfig(name, serviceJSON)`.

Unlike the Python bridge (which runs the check to verify), the JMX bridge
generates a **best-guess candidate** from the service info and returns it
immediately. Verification happens asynchronously in JMXFetch.

```go
func (b *jmxBridge) DiscoverConfig(integrationName, serviceJSON string) (string, error) {
    // 1. Parse service JSON → {host, ports[]}
    // 2. Pick best candidate JMX port:
    //    - Ports named "jmx" or "jmx-rmi" first
    //    - Then common JMX ports: 9999, 9010, 1099, 7199
    //    - Then first exposed port
    // 3. Return config JSON:
    //    {
    //      "instances": [{"host": "%%host%%", "port": <port>,
    //                      "discovery": true,
    //                      "collect_default_jvm_metrics": true}],
    //      "init_config": {"is_jmx": true,
    //                      "collect_default_metrics": true,
    //                      "new_gc_metrics": true}
    //    }
}
```

The `%%host%%` is resolved by `configresolver.Resolve()` after the
discovery worker calls `applyDiscoveredConfigsLocked`.

If no JMX port is found, return `"[]"` (PermFail) — the worker drops it.

### 2. Config template (`conf.d/kafka.d/conf.yaml`)

```yaml
ad_identifiers:
  - cp-kafka

discovery: {}

init_config:
  is_jmx: true
  collect_default_metrics: true
  new_gc_metrics: true

instances: []
```

- `discovery: {}` triggers the discovery worker path
- `instances: []` — the bridge generates instances
- `init_config` is replaced by the discovered config's init_config

### 3. Composite discoverer (`discoverer/composite.go`)

Unchanged: routes JMX integrations to JMX bridge, others to Python bridge.

### 4. Autoconfig (`impl/autoconfig.go`)

Unchanged: uses composite discoverer.

---

## JMXFetch Side

### 1. Instance discovery flag (`Instance.java`)

When `discovery: true` is set in the instance config:

**In `init()`:**
- After connecting, inspect MBean domains via `JmxDiscovery.discover()`
- If app type detected, add conf entries to `configurationList`
- If not detected, proceed with default JVM metrics only
- If connection fails, the failure is handled differently (see below)

**In `processStatus()` and `processInstantiationStatus()`:**
- When the instance has `discovery: true`:
  - Do NOT call `sendServiceCheck()` with `STATUS_ERROR`
  - Do NOT call `reportStatus()` with `STATUS_ERROR`
  - Log at DEBUG level instead of WARN
  - The instance goes to `brokenInstanceMap` as usual (so it gets retried)
  - No error service checks are emitted to the backend

**In `fixBrokenInstances()`:**
- Discovery instances in `brokenInstanceMap` are retried as usual
- When they eventually connect, the discovery flow runs (inspect MBeans, etc.)
- Once metrics are collected, normal status reporting resumes

### 2. `JmxDiscovery.java` (unchanged from PoC)

MBean domain signature matching: Kafka, ActiveMQ, Tomcat, Cassandra, Solr.

### 3. `Connection.java` (unchanged from PoC)

`getMBeanServerConnection()` method added.

### 4. No new endpoints, no new HTTP server, no new command-line params

---

## End-to-End Flow

```
1. Container listener discovers Kafka container
   → AD matches template (ad_identifiers: [cp-kafka], discovery: {})
   → resolveTemplateForService sees discovery: {}
   → enqueues discovery probe

2. Discovery Worker calls JmxBridge.DiscoverConfig("kafka", serviceJSON)
   → Bridge parses service JSON: host=172.17.130.4, ports=[9092, 9999]
   → Picks port 9999 (common JMX port)
   → Returns config JSON with discovery: true flag

3. Worker calls onDiscoveryResult
   → applyDiscoveredConfigsLocked merges with template
   → configresolver.Resolve resolves %%host%% → 172.17.130.4
   → Config scheduled via scheduler controller
   → JmxScheduler.Schedule adds it to jmxfetch state cache

4. JMXFetch polls GET /agent/jmx/configs (next cycle, ≤15s)
   → Picks up the discovery candidate config
   → Creates Instance with discovery: true

5a. JMX is available:
   → Instance.init() connects to 172.17.130.4:9999
   → JmxDiscovery.inspect() finds kafka.server, kafka.controller domains
   → Adds conf entries for kafka.server and kafka.controller
   → refreshBeansList(), getMatchingAttributes()
   → doIteration() collects 350 metrics
   → Reports OK status via POST /agent/jmx/status
   → Subsequent cycles collect metrics normally

5b. JMX is NOT available:
   → Instance.init() fails to connect
   → processInstantiationStatus sees discovery: true
   → SKIPS sendServiceCheck() and reportStatus() with ERROR
   → Logs at DEBUG: "discovery instance <name> connection failed, will retry"
   → Instance goes to brokenInstanceMap
   → No error service checks emitted
   → Next cycle: fixBrokenInstances() retries
   → Repeats until connection succeeds or instance is unscheduled

6. If the agent unschedules the config (e.g., container stopped):
   → Config disappears from /configs response
   → JMXFetch removes the instance on next init cycle
   → No cleanup needed
```

---

## Error Handling

| Scenario | JMXFetch behavior | Service checks | Status |
|---|---|---|---|
| JMX available, app detected | Connect, discover, collect | OK | OK |
| JMX available, app not detected | Connect, collect JVM defaults only | OK | OK |
| JMX not available (discovery: true) | Silent retry | **None** | Not reported (or "pending") |
| JMX not available (normal config) | Error + retry | ERROR | ERROR |
| Config unscheduled | Instance removed | None | None |

**Key property: discovery instances never emit ERROR service checks.**

---

## What Changed vs the Initial PoC

| Aspect | Initial PoC | This Design |
|---|---|---|
| Config source | conf.d template (no discovery worker) | Discovery worker with `discovery: {}` |
| Port selection | Hardcoded 9999 in template | Bridge picks from service's exposed ports |
| Error on connection failure | Error service checks generated | **Suppressed** for discovery instances |
| App detection | At JMXFetch init time | Same, but with error suppression |
| New JMXFetch endpoints | None | None |
| New JMXFetch HTTP server | None | None |

---

## Implementation Steps

### Phase 1: JMXFetch

1. Keep `JmxDiscovery.java` (MBean signature matching)
2. Keep `Connection.getMBeanServerConnection()`
3. Modify `Instance.java`:
   - Add `discovery` flag field (parsed from instance config)
   - In `init()`: when `discovery: true` and no explicit conf, call
     `JmxDiscovery.discover()` after connecting
   - Add `isDiscovery()` getter
4. Modify `App.java`:
   - In `processStatus()`: skip `sendServiceCheck()` and `reportStatus()`
     with ERROR when `instance.isDiscovery()` is true
   - In `processInstantiationStatus()`: skip error logging at WARN level
     for discovery instances, use DEBUG instead
   - In `processCollectionStatus()`: skip error service checks for discovery
     instances that haven't yet connected
5. Build jar

### Phase 2: Agent

1. Rewrite `jmx_bridge.go`:
   - Parse service JSON, pick candidate JMX port
   - Return config JSON with `discovery: true` in instance
2. Keep `composite.go` and `autoconfig.go` changes
3. Update conf.d template to use `discovery: {}`
4. Build agent

### Phase 3: Test

1. Baseline: Kafka + manual config → 28 metrics (unchanged)
2. Discovery: Kafka + `discovery: {}` template → 350+ metrics
3. Error case: Kafka without JMX + `discovery: {}` template → no error
   service checks, silent retries
4. Document results

---

## File Changes Summary

### JMXFetch (`~/dd/jmxfetch/`)

| File | Status | Description |
|---|---|---|
| `JmxDiscovery.java` | Unchanged | MBean signature matching |
| `Connection.java` | Unchanged | `getMBeanServerConnection()` |
| `Instance.java` | Modified | `discovery` flag, MBean inspection in `init()`, `isDiscovery()` getter |
| `App.java` | Modified | Suppress error service checks for discovery instances |

### Agent (`~/dd/datadog-agent/`)

| File | Status | Description |
|---|---|---|
| `discoverer/jmx_bridge.go` | Rewritten | Generate candidate config from service ports |
| `discoverer/jmx_bridge_nojmx.go` | Unchanged | No-op for non-JMX builds |
| `discoverer/composite.go` | Unchanged | Routes to JMX or Python bridge |
| `impl/autoconfig.go` | Unchanged | Uses composite discoverer |
