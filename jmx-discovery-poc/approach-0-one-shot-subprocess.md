# Revised JMX Configuration Discovery Plan

## Problem with the Current PoC

The current PoC has two fundamental flaws:

1. **Hardcoded port 9999**: The conf.d template specifies `port: 9999` directly.
   Real discovery must probe candidate ports from the service info.

2. **Errors on connection failure**: The config is scheduled immediately with
   `discovery: true`, and JMXFetch tries to connect at init time. If JMX isn't
   available, the instance generates error service checks. The proposal's goal
   is the opposite: **no config should be scheduled until the JMX endpoint is
   verified reachable and the application type is confirmed**.

## How Python Discovery Works (the pattern to follow)

```
1. Template has `discovery: {}` → AD skips normal resolution, enqueues probe
2. Worker calls PythonBridge.DiscoverConfig(name, serviceJSON)
3. Python `discover_config(cls, service_json)`:
   a. Parses service JSON (host, ports)
   b. Generates candidate configs from candidate_ports()
   c. For each candidate: instantiates the real check and calls check.run()
   d. Accepts the candidate ONLY if check.run() succeeds AND collects ≥1 metric
   e. Returns the accepted config as JSON, or "[]" if none accepted
4. If "[]" or error → worker retries (up to maxAttempts), no config scheduled
5. If valid config → agent resolves + schedules it (no errors during this phase)
```

Key principle: **the check is actually run against the target**. If it can't
connect or collect metrics, the candidate is rejected silently. Only a verified
config reaches the scheduler.

## Proposed JMX Discovery Design

### Architecture

```
                    ┌─────────────────────────────────┐
                    │  Agent (Go)                     │
                    │                                  │
                    │  DiscoveryConfig template        │
                    │  (discovery: {})                 │
                    │       │                          │
                    │       ▼                          │
                    │  Discovery Worker                │
                    │       │                          │
                    │       ▼                          │
                    │  JmxBridge.DiscoverConfig()       │
                    │       │                          │
                    │    Runs JMXFetch as              │
                    │    one-shot subprocess           │
                    │    with action="discover"        │
                    │       │                          │
                    └───────┼──────────────────────────┘
                            │
                            ▼
                    ┌─────────────────────────────────┐
                    │  JMXFetch (Java, one-shot)       │
                    │                                  │
                    │  1. Parse service JSON           │
                    │     (host, ports)                │
                    │  2. For each candidate port:     │
                    │     a. Try JMX connection        │
                    │     b. If connected:             │
                    │        - Inspect MBean domains   │
                    │        - Match against signatures│
                    │        - If match: build config  │
                    │          with conf entries       │
                    │        - Collect 1 iteration     │
                    │          to verify metrics flow  │
                    │        - If metrics > 0:         │
                    │          output config JSON      │
                    │          exit 0                  │
                    │     c. If no match or no metrics:│
                    │        try next port             │
                    │  3. If nothing matched:          │
                    │     output "[]" and exit 0       │
                    │     (NOT an error exit)          │
                    └─────────────────────────────────┘
```

### Why one-shot subprocess?

The Python discovery bridge runs the Python check in-process via rtloader.
JMXFetch is Java and runs as a separate process. The simplest approach that
matches the Python pattern is to run JMXFetch as a **one-shot subprocess** in
"discover" mode — the same way `agent jmx list_everything` already works today
(via `execJmxCommand` in `pkg/cli/standalone/jmx.go`).

The subprocess:
- Receives the service JSON (host + ports) as a command-line argument or
  via stdin
- Tries to connect to candidate JMX ports
- Inspects MBeans, matches app signatures
- Does one collection iteration to verify metrics actually flow
- Outputs the discovered config as JSON to stdout
- Exits

This is expensive (JVM startup ~1-2s per probe) but acceptable for a PoC.
A production version could add a discovery endpoint to the long-running
JMXFetch process via IPC.

### Agent Side Changes

#### 1. `discoverer/jmx_bridge.go` (rewrite)

Instead of generating a hardcoded config, the bridge:

1. Marshals the service JSON (same format as Python bridge receives)
2. Runs `jmxfetch` as a one-shot subprocess with `--action discover` and
   the service JSON
3. Captures stdout
4. If stdout is valid JSON with at least one config → return it
5. If stdout is "[]" or empty → return error (worker will retry)
6. If subprocess fails → return error (worker will retry)

The bridge needs access to the jmxfetch binary path and the IPC auth token
(for connecting to the agent to report status). It can reuse the existing
`JMXFetch` struct from `pkg/jmxfetch/jmxfetch.go` with `Command = "discover"`
and a custom reporter that outputs JSON to stdout.

#### 2. Config template (`conf.d/kafka.d/conf.yaml`)

```yaml
ad_identifiers:
  - cp-kafka

discovery: {}

init_config:
  is_jmx: true
  collect_default_metrics: true
  new_gc_metrics: true

instances:
  - host: "%%host%%"
    port: "%%port%%"
```

Key differences from the current PoC:
- `discovery: {}` (not `discovery: true` in the instance) — this triggers
  the discovery worker path, same as Python integrations
- No hardcoded port — `%%port%%` will be filled from the discovered config
- No `discovery: true` flag in the instance — discovery happens before
  scheduling, not at JMXFetch runtime

Wait — actually, the `%%port%%` template variable won't work because the
discovery worker bypasses normal template resolution. The discovery bridge
returns a complete config with the resolved host and port. The template's
instances are ignored; the discovered config replaces them.

So the template should be:

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

The `instances` from the template are not used — the discovery bridge
generates them. But `init_config` is merged with the discovered config
(see `applyDiscoveredConfigsLocked` in `configmgr_discovery.go`).

Actually, looking at the code more carefully:

```go
merged := tpl
merged.InitConfig = discovered.InitConfig
merged.Instances = discovered.Instances
```

The discovered config's InitConfig and Instances **replace** the template's.
So the bridge must return the complete init_config including `is_jmx: true`,
`collect_default_metrics: true`, etc.

### JMXFetch Side Changes

#### 1. New action: `discover` (in `AppConfig.java`)

Add `ACTION_DISCOVER = "discover"` to the actions set.

#### 2. New command-line params (in `AppConfig.java`)

- `--service_json <json>` — the service JSON payload (host, ports)
- Or read from stdin

#### 3. New discovery flow (in `App.java`)

When action is "discover":
1. Parse the service JSON to get host and ports
2. For each candidate port (common JMX ports first, then all exposed ports):
   a. Create a JMX connection to host:port
   b. If connection fails → try next port
   c. If connected → query MBean domains
   d. Match domains against app signatures (JmxDiscovery)
   e. If matched → build config with:
      - init_config: is_jmx, collect_default_metrics, new_gc_metrics, conf entries
      - instances: [{host, port, collect_default_jvm_metrics: true}]
   f. Do one collection iteration to verify metrics flow
   g. If metrics > 0 → output config JSON to stdout, exit 0
   h. If no metrics → try next port
3. If nothing matched → output "[]" to stdout, exit 0

The output format must match what `parseDiscoveryResult` in
`discovery_json.go` expects:

```json
[{
  "instances": [{"host": "172.17.130.4", "port": 9999, "collect_default_jvm_metrics": true}],
  "init_config": {"is_jmx": true, "collect_default_metrics": true, "new_gc_metrics": true, "conf": [...]}
}]
```

#### 4. `JmxDiscovery.java` (modify)

Keep the MBean domain matching logic but split it:
- `discoverDomains(Connection)` → returns matched app signature (or null)
- The App.java discover action handles the connection, iteration, and output

#### 5. Remove `discovery: true` flag handling from `Instance.java`

The `discovery: true` flag in the instance map is no longer needed.
Discovery happens before scheduling, not at runtime. Revert the Instance.java
changes (or keep them as a fallback, but they won't be used in the new flow).

### Test Setup

#### Baseline (unchanged)

Same as before: Kafka with AD labels, manual JMX config, 28 metrics.

#### Discovery (revised)

1. Kafka container with JMX enabled on port 9999, NO AD labels
2. Agent with conf.d/kafka.d/conf.yaml containing `discovery: {}`
3. Agent discovers Kafka container via AD identifiers
4. Discovery worker enqueues probe
5. JmxBridge runs JMXFetch one-shot with service JSON
6. JMXFetch connects to Kafka:9999, inspects MBeans, detects Kafka
7. JMXFetch does one collection iteration, verifies 350+ metrics
8. JMXFetch outputs config JSON to stdout
9. Bridge returns config to worker
10. Worker calls `onDiscoveryResult` → `applyDiscoveredConfigsLocked`
11. Config is resolved and scheduled via JmxScheduler
12. Long-running JMXFetch picks up the config and starts collecting

#### Error case test

1. Kafka container with JMX NOT enabled (no port 9999)
2. Agent with conf.d/kafka.d/conf.yaml containing `discovery: {}`
3. Discovery worker enqueues probe
4. JMXFetch tries to connect, fails
5. JMXFetch outputs "[]", exits 0
6. Bridge returns error (empty result)
7. Worker retries up to maxAttempts
8. After maxAttempts, gives up silently
9. **No error service checks generated**
10. No config scheduled

### Implementation Steps

1. **JMXFetch**: Add `discover` action to AppConfig
2. **JMXFetch**: Implement discover flow in App.java (connect, inspect, verify, output)
3. **JMXFetch**: Build jar
4. **Agent**: Rewrite `jmx_bridge.go` to run JMXFetch as subprocess
5. **Agent**: Update conf.d template to use `discovery: {}`
6. **Agent**: Build local agent image
7. **Test**: Run baseline test (should still work)
8. **Test**: Run discovery test (should detect Kafka and collect 350+ metrics)
9. **Test**: Run error case test (should produce no errors, just retries)
10. **Document**: Update JMX_DISCOVERY_POC.md
