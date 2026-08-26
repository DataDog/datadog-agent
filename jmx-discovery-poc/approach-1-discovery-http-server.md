# JMX Configuration Discovery — Production Design

## Problem Statement

JMX integrations (Kafka, ActiveMQ, Tomcat, Cassandra, Solr) run through
JMXFetch, a Java subprocess managed by the core agent. The existing
configuration-discovery mechanism only works for Python integrations: it
calls `discover_config()` on the Python check class via rtloader, which
**actually runs the check** to verify the config works before scheduling it.

We need the same for JMX: a discovery probe that connects to a candidate
JMX endpoint, inspects MBeans to auto-detect the application type, verifies
that metrics can be collected, and only then returns a config to be
scheduled. If the endpoint is unreachable or no metrics are collected, the
probe returns nothing — no config is scheduled, no error service checks are
generated.

## Constraint: Reuse the Running JMXFetch Process

Starting a JVM per discovery probe is unacceptable (~1–2 s startup each,
multiple probes per service, retry loop). JMXFetch is already a long-running
process with an established IPC channel to the agent. The design must reuse
it.

## Current IPC Architecture

```
Agent (Go)                         JMXFetch (Java subprocess)
┌──────────────┐                   ┌──────────────┐
│  IPC Server   │ ◀── HTTPS ────── │  HttpClient   │
│  :5001        │    GET /configs  │               │
│  (cmd_port)   │    POST /status  │               │
│               │                  │               │
│  Auth: Bearer │                  │  SESSION_TOKEN│
│  token        │                  │  env var      │
└──────────────┘                   └──────────────┘
```

- Agent starts JMXFetch with `--ipc_host`, `--ipc_port`, and
  `SESSION_TOKEN` env var.
- JMXFetch polls `GET /agent/jmx/configs?timestamp=<ts>` every check period
  (15 s default) to get scheduled configs.
- JMXFetch posts status to `POST /agent/jmx/status`.
- All communication is HTTPS with Bearer token auth.
- **No Agent → JMXFetch channel exists.**

## Proposed Design: Discovery Endpoint on JMXFetch

### Overview

Add a lightweight HTTP server to JMXFetch that the agent can call
synchronously for discovery probes. This reuses the running JVM and provides
low-latency request/response without polling.

```
Agent (Go)                         JMXFetch (Java subprocess)
┌──────────────┐                   ┌──────────────────┐
│  IPC Server   │ ◀── HTTPS ────── │  HttpClient       │
│  :5001        │    GET /configs  │                   │
│  (cmd_port)   │    POST /status  │  Discovery Server │
│               │                   │  :5002            │
│  JmxBridge     │ ── HTTP POST ──▶ │  POST /discover   │
│  (discoverer)  │    service JSON  │                   │
│               │ ◀── JSON resp ── │  config or []     │
└──────────────┘                   └──────────────────┘
```

### Why HTTP server on JMXFetch (not a polling pattern)

| Approach | Latency | Complexity | Reuses JVM | Fit with existing arch |
|---|---|---|---|---|
| One-shot subprocess per probe | High (JVM startup) | Low | No | Matches `list_everything` |
| Agent enqueues, JMXFetch polls | High (poll interval) | Medium | Yes | Similar to config polling |
| **JMXFetch HTTP server** | **Low (synchronous)** | **Medium** | **Yes** | **New but natural** |

The HTTP server approach gives us synchronous request/response, which is
what the Go `ConfigDiscoverer.DiscoverConfig()` interface expects. The
discovery worker calls `DiscoverConfig()` and blocks until it gets a result.

### Security

- JMXFetch's discovery server listens on **localhost only**.
- Auth: same `SESSION_TOKEN` Bearer token, sent by the agent and verified
  by JMXFetch.
- Transport: plain HTTP on localhost (no TLS needed for loopback; same
  trust model as a Unix socket). If needed, can be upgraded to HTTPS later.
- Port: configurable via `jmx_discovery_port` (default: `cmd_port + 1`).
  Passed to JMXFetch as `--discovery_port`.

---

## Agent Side

### 1. Config key

New config key in `all_settings.go`:

```go
config.BindEnvAndSetDefault("jmx_discovery_port", 5002)
```

### 2. JMXFetch startup (`pkg/jmxfetch/jmxfetch.go`)

Pass `--discovery_port` to the JMXFetch subprocess, same as `--ipc_port`.

### 3. JMX discovery bridge (`discoverer/jmx_bridge.go`)

The bridge implements `ConfigDiscoverer.DiscoverConfig(integrationName, serviceJSON) (string, error)`.

```go
func (b *jmxBridge) DiscoverConfig(integrationName, serviceJSON string) (string, error) {
    // 1. Check that this is a JMX integration
    if !isJMXIntegration(integrationName) {
        return "", fmt.Errorf("not a JMX integration: %s", integrationName)
    }

    // 2. POST service JSON to JMXFetch's discovery endpoint
    //    http://localhost:<discovery_port>/discover
    //    Auth: Bearer <session_token>
    //    Body: {"integration": "kafka", "service": <serviceJSON>}
    
    // 3. Read response body as string
    //    - 200 with JSON body → return body string
    //    - 200 with "[]" → return "", PermFail (no JMX endpoint found)
    //    - connection refused → return error (JMXFetch not running yet, worker retries)
    //    - timeout → return error (worker retries)
}
```

The bridge needs the discovery port and session token. These are available
from the `ipc.Component` and config. The bridge is constructed in
`createNewAutoConfig` where `ipc` is available.

### 4. Composite discoverer (`discoverer/composite.go`)

Unchanged from PoC: routes JMX integrations to JMX bridge, others to Python
bridge.

### 5. Config template (`conf.d/kafka.d/conf.yaml`)

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

- `discovery: {}` triggers the discovery worker path (same as Python).
- `instances: []` — the discovery bridge generates instances.
- `init_config` is merged with the discovered config by
  `applyDiscoveredConfigsLocked`. However, the discovered config's
  `init_config` **replaces** the template's, so the bridge must return a
  complete `init_config`.

---

## JMXFetch Side

### 1. Discovery HTTP server (`DiscoveryServer.java`)

New class using `com.sun.net.httpserver.HttpServer` (built into JDK 11):

```java
public class DiscoveryServer {
    private HttpServer server;
    private String authToken;
    private App app;  // reference to App for running discovery probes

    public void start(int port, String authToken, App app) {
        this.authToken = authToken;
        this.app = app;
        server = HttpServer.create(new InetSocketAddress("localhost", port), 0);
        server.createContext("/discover", this::handleDiscover);
        server.setExecutor(Executors.newSingleThreadExecutor());
        server.start();
    }

    private void handleDiscover(HttpExchange exchange) {
        // 1. Verify Bearer token
        // 2. Read request body: {"integration": "kafka", "service": {...}}
        // 3. Call app.discoverConfig(integration, serviceJSON)
        // 4. Write response: config JSON or "[]"
    }

    public void stop() { ... }
}
```

### 2. New command-line param (`AppConfig.java`)

```java
@Parameter(names = {"--discovery_port"},
    description = "Port for the discovery HTTP server (0 = disabled)",
    validateWith = PositiveIntegerValidator.class)
@Builder.Default
private int discoveryPort = 0;
```

### 3. Discovery flow (`App.java`)

New method `discoverConfig(String integrationName, String serviceJSON)`:

```
1. Parse service JSON → {host, ports[]}
2. Build candidate port list:
   - Ports named "jmx" or "jmx-rmi" first
   - Then common JMX ports: 9999, 9010, 1099, 7199
   - Then all other exposed ports
3. For each candidate port:
   a. Create a JMX connection to host:port
   b. If connection fails → next port
   c. Query MBean domains
   d. Match against AppSignatures (JmxDiscovery)
   e. If no match → next port (still log for debugging)
   f. If matched:
      - Build instance config: {host, port, collect_default_jvm_metrics: true}
      - Build init_config: {is_jmx: true, collect_default_metrics: true,
        new_gc_metrics: true, conf: [matched signature's conf entries]}
      - Create a temporary Instance, init it, run one collection iteration
      - If metrics collected > 0:
        - Return JSON: [{"instances": [...], "init_config": {...}}]
        - Clean up temporary instance and connection
      - If no metrics: clean up, try next port
4. If nothing matched: return "[]"
```

The output format matches `parseDiscoveryResult` in `discovery_json.go`:

```json
[{
  "instances": [{
    "host": "172.17.130.4",
    "port": 9999,
    "collect_default_jvm_metrics": true
  }],
  "init_config": {
    "is_jmx": true,
    "collect_default_metrics": true,
    "new_gc_metrics": true,
    "conf": [
      {"include": {"domain": "kafka.server"}},
      {"include": {"domain": "kafka.controller"}}
    ]
  }
}]
```

### 4. AppSignature registry (`JmxDiscovery.java`)

Keep the MBean domain matching logic. Each signature contains:
- Name (e.g. "kafka")
- Required domains (any match wins)
- Conf entries to generate

Extend with more signatures over time. For the PoC: kafka, activemq, tomcat,
cassandra, solr.

### 5. Lifecycle

- `DiscoveryServer.start()` is called in `App.run()` when `discoveryPort > 0`
  and the action is `collect`.
- `DiscoveryServer.stop()` is called in `App.stop()` / shutdown hook.
- The discovery server runs on a separate thread, so it doesn't block the
  main collection loop.
- Discovery probes are processed on a single-threaded executor to avoid
  resource contention with regular metric collection.

### 6. Remove `discovery: true` instance flag

The `discovery: true` flag in the instance map (from the initial PoC) is no
longer needed. Discovery happens before scheduling, not at JMXFetch runtime.
Revert the `Instance.java` changes from the initial PoC.

---

## End-to-End Flow

```
1. Agent starts → JMXFetch starts with --discovery_port 5002
   JMXFetch's DiscoveryServer listens on localhost:5002

2. Container listener discovers Kafka container
   → AD matches template (ad_identifiers: [cp-kafka])
   → resolveTemplateForService sees discovery: {}
   → enqueues discovery probe

3. Discovery Worker calls JmxBridge.DiscoverConfig("kafka", serviceJSON)
   → JmxBridge POSTs to http://localhost:5002/discover
   → JMXFetch receives request

4. JMXFetch discovery:
   a. Parses service JSON: host=172.17.130.4, ports=[9092, 9999]
   b. Tries port 9999 (common JMX port)
   c. Connects to JMX at 172.17.130.4:9999 ✓
   d. Queries MBean domains → finds kafka.server, kafka.controller
   e. Matches "kafka" signature
   f. Creates temporary Instance, runs one collection iteration
   g. Collects 350 metrics ✓
   h. Returns config JSON

5. JmxBridge returns config JSON to Worker
   → Worker calls onDiscoveryResult
   → applyDiscoveredConfigsLocked merges with template
   → configresolver.Resolve resolves %%host%% etc.
   → Config scheduled via scheduler controller

6. JmxScheduler.Schedule receives the resolved config
   → Config added to jmxfetch state cache
   → JMXFetch polls GET /agent/jmx/configs, picks up new config
   → Creates Instance, starts collecting metrics

7. If JMX not available (step 4c fails):
   → JMXFetch tries all candidate ports, all fail
   → Returns "[]"
   → JmxBridge returns error
   → Worker retries (up to maxAttempts, 10 s delay)
   → After maxAttempts: gives up silently
   → NO config scheduled, NO error service checks
```

---

## Error Handling

| Scenario | JMXFetch response | Bridge return | Worker behavior |
|---|---|---|---|
| JMX endpoint found, metrics collected | 200 + config JSON | config JSON | Schedule config |
| JMX endpoint found, no app match | 200 + `[]` | `""`, PermFail | Drop immediately |
| JMX endpoint found, app matched, 0 metrics | 200 + `[]` | `""`, PermFail | Drop immediately |
| JMX endpoint unreachable (all ports) | 200 + `[]` | `""`, PermFail | Drop immediately |
| JMXFetch not running yet | Connection refused | error | Retry after delay |
| Discovery server timeout | Timeout | error | Retry after delay |
| Max retries exceeded | N/A | N/A | Drop silently, no errors |

Key principle: **discovery failures never produce error service checks**.
They are either retried silently or dropped.

---

## Configuration

### Agent config keys

| Key | Default | Description |
|---|---|---|
| `jmx_discovery_port` | `5002` | Port for JMXFetch's discovery server. Passed as `--discovery_port` to JMXFetch. Set to 0 to disable. |

### JMXFetch command-line params

| Param | Default | Description |
|---|---|---|
| `--discovery_port` | `0` (disabled) | Port for discovery HTTP server. 0 = disabled. |

### Config template

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

---

## Implementation Plan

### Phase 1: JMXFetch

1. Add `--discovery_port` param to `AppConfig.java`
2. Create `DiscoveryServer.java` — lightweight HTTP server on localhost
3. Add `discoverConfig(integration, serviceJSON)` method to `App.java`:
   - Parse service JSON
   - Try candidate ports
   - Connect, inspect MBeans, match signatures
   - Run one collection iteration to verify
   - Return config JSON or `[]`
4. Wire `DiscoveryServer` lifecycle into `App.run()` / `App.stop()`
5. Revert `Instance.java` changes from initial PoC (remove `discovery: true` handling)
6. Keep `JmxDiscovery.java` (MBean signature matching)
7. Keep `Connection.getMBeanServerConnection()` (needed by discovery)
8. Build jar

### Phase 2: Agent

1. Add `jmx_discovery_port` config key
2. Pass `--discovery_port` to JMXFetch subprocess in `jmxfetch.go`
3. Rewrite `jmx_bridge.go`:
   - POST service JSON to `http://localhost:<port>/discover`
   - Parse response, return to worker
   - Handle connection refused (JMXFetch not up yet → retry)
4. Update `composite.go` (unchanged from PoC)
5. Update `autoconfig.go` (unchanged from PoC)
6. Update conf.d template to use `discovery: {}`
7. Build agent

### Phase 3: Test

1. Baseline test (unchanged): Kafka + manual config → 28 metrics
2. Discovery test: Kafka + `discovery: {}` template → 350+ metrics
3. Error case test: Kafka without JMX + `discovery: {}` template → no errors, no config scheduled, retries exhausted silently
4. Document all results

---

## File Changes Summary

### JMXFetch (`~/dd/jmxfetch/`)

| File | Status | Description |
|---|---|---|
| `src/.../AppConfig.java` | Modified | Add `--discovery_port` param |
| `src/.../DiscoveryServer.java` | New | HTTP server for discovery probes |
| `src/.../App.java` | Modified | Add `discoverConfig()`, wire DiscoveryServer |
| `src/.../JmxDiscovery.java` | Modified | Keep signature matching, split for reuse |
| `src/.../Connection.java` | Modified | Keep `getMBeanServerConnection()` |
| `src/.../Instance.java` | Reverted | Remove `discovery: true` handling |

### Agent (`~/dd/datadog-agent/`)

| File | Status | Description |
|---|---|---|
| `comp/.../discoverer/jmx_bridge.go` | Rewritten | HTTP client to JMXFetch discovery server |
| `comp/.../discoverer/jmx_bridge_nojmx.go` | Unchanged | No-op for non-JMX builds |
| `comp/.../discoverer/composite.go` | Unchanged | Routes to JMX or Python bridge |
| `comp/.../impl/autoconfig.go` | Unchanged | Uses composite discoverer |
| `pkg/config/setup/all_settings.go` | Modified | Add `jmx_discovery_port` config key |
| `pkg/jmxfetch/jmxfetch.go` | Modified | Pass `--discovery_port` to subprocess |
