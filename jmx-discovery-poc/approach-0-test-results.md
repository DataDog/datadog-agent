# Approach 0: One-Shot Subprocess — Test Results

## Summary

Approach 0 (one-shot JMXFetch subprocess per discovery probe) was
implemented and tested both manually and end-to-end. The JMXFetch
`discover` action works correctly: it connects to candidate JMX ports,
inspects MBean domains, verifies the integration's expected domains are
present, and outputs a config JSON on stdout. Error cases (no JMX,
unknown integration, unreachable host) all return `[]` with exit 0 — no
errors generated.

The full Agent → JMXFetch discovery flow was tested end-to-end with a
properly built agent binary (`dda inv agent.build`) running in Docker
with Kafka. The flow works: AD discovers Kafka → discovery worker → JMX
bridge → one-shot subprocess → config scheduled → JMXFetch collects
metrics.

## JMXFetch Discover Action Manual Tests

### Test 1: Successful discovery (Kafka with JMX on port 9999)

**Command:**
```bash
docker exec dd-agent-approach0 java -classpath /opt/datadog-agent/bin/agent/dist/jmx/jmxfetch.jar \
  org.datadog.jmxfetch.App --reporter console --log_level OFF discover \
  --integration kafka \
  --service_json '{"id":"docker://test","host":"kafka","ports":[{"number":9092,"name":""},{"number":9999,"name":"jmx"}]}'
```

**Output (stdout):**
```json
[ {
  "init_config" : {},
  "instances" : [ {
    "port" : 9999,
    "host" : "kafka",
    "collect_default_jvm_metrics" : true
  } ]
} ]
```

**Exit code:** 0

**What happened:**
1. Parsed service JSON: host=kafka, ports=[9092, 9999]
2. Picked port 9999 (named "jmx" — highest priority)
3. Connected to Kafka's JMX endpoint at kafka:9999
4. Queried MBean domains: found kafka.server, kafka.controller, etc.
5. Verified kafka's expected domains (kafka.server OR kafka.controller) are present
6. Output config JSON with empty init_config (preserves template's metrics.yaml)
7. Exited 0

### Test 2: No JMX port available (only Kafka's plaintext port 9092)

**Output:** `[]`  **Exit code:** 0

Port 9092 is not a JMX port. Connection attempt failed. No errors.

### Test 3: Unknown integration

**Output:** `[]`  **Exit code:** 0

"unknown" is not in StandardJMXIntegrations. No signature to verify against.

### Test 4: Unreachable host

**Output:** `[]`  **Exit code:** 0

Connection to unreachable.invalid:9999 failed. No errors.

## Full E2E Test (with dda-built agent)

### Setup

- **Agent image**: `datadog/agent-dev:nightly-main-py3-jmx` base + agent
  binary built with `dda inv agent.build --build-exclude=systemd` +
  modified jmxfetch jar (with `discover` action)
- **Kafka**: `confluentinc/cp-kafka:7.7.0` with JMX on port 9999
- **Config template**: `conf.d/kafka.d/conf.yaml` with `discovery: {}` and
  `ad_identifiers: [cp-kafka]`

### Key Log Evidence

**Discovery subprocess ran (two attempts):**

```
12:41:42 | JMX discovery: running subprocess: java [...] discover --integration kafka --service_json {...}
12:41:43 | JMX discovery: subprocess found no valid JMX config for integration kafka
```
First attempt failed — Kafka's JMX endpoint wasn't ready yet. Worker retried.

```
12:41:58 | JMX discovery: running subprocess: java [...] discover --integration kafka --service_json {...}
12:41:58 | JMX discovery: subprocess succeeded for integration kafka, result length: 151
```
Second attempt succeeded — subprocess connected, verified kafka.server/kafka.controller
MBean domains, returned 151-byte config JSON.

**Config scheduled with discovery tag:**

```
12:41:58 | Scheduling jmxfetch config: kafka_3f4968170d1a4303:
  check_name: kafka
  init_config: {}
  instances:
  - collect_default_jvm_metrics: true
    host: 172.17.130.4
    port: 9999
    tags:
    - dd_config_discovery:true
    - docker_image:confluentinc/cp-kafka:7.7.0
    - short_image:cp-kafka
```

The `dd_config_discovery:true` tag confirms the config went through the
discovery worker path. The `config.provider` is `ad-container-discovery+file`.

**JMXFetch collecting metrics:**

```
12:42:59 | Instance kafka-172.17.130.4-9999 is sending 28 metrics to the metrics reporter during collection #5
12:43:14 | Instance kafka-172.17.130.4-9999 is sending 28 metrics to the metrics reporter during collection #6
...
```

JMXFetch is stably collecting 28 metrics per cycle (default JVM metrics).

### Performance

- First probe (failed): ~1s (subprocess startup + connection attempt)
- Second probe (succeeded): ~1s (subprocess startup + connect + inspect)
- Total time from agent start to config scheduled: ~18s
- JVM cold start: ~1s per probe
- No impact on running JMXFetch instances (subprocess is isolated)

## init_config Preservation: Fixed

**Root cause**: The JMXFetch subprocess outputs pretty-printed JSON with
`init_config: {}`. The JSON parser in `parseDiscoveryResult()` preserves
the whitespace, producing `"{\n\n  }"` instead of `"{}"`. The
`isEmptyJSON()` check only compared against `"{}"` (no whitespace), so it
returned `false` and the template's init_config was incorrectly replaced
with the empty one.

**Fix**: Updated `isEmptyJSON()` to strip all whitespace (newlines,
spaces, tabs) before comparing, so `"{\n\n  }"` is correctly
identified as empty.

**Verified**: Debug traces confirm the template's init_config
(`is_jmx: true, collect_default_metrics: true, new_gc_metrics: true`)
is now preserved through the entire pipeline:

```
tpl.InitConfig="collect_default_metrics: true\nis_jmx: true\nnew_gc_metrics: true\n" (len=64)
discovered.InitConfig="{\n\n  }" (len=6), isEmptyJSON=true
after merge, merged.InitConfig="collect_default_metrics: true\nis_jmx: true\nnew_gc_metrics: true\n" (len=64)
after Resolve, resolved.InitConfig="collect_default_metrics: true\nis_jmx: true\nnew_gc_metrics: true\n" (len=64)
after decrypt, decrypted.InitConfig="collect_default_metrics: true\nis_jmx: true\nnew_gc_metrics: true\n" (len=64)
JMXFetch received init_config={new_gc_metrics=true, collect_default_metrics=true, is_jmx=true}
```

**Note on metric count**: JMXFetch collects 74 metrics per cycle —
Kafka-specific metrics from the integration's `metrics.yaml` plus default
JVM metrics. This is the correct count for a single-broker Kafka with no
active producers/consumers. (The initial PoC reported 350 because it
used broad domain includes that matched every MBean attribute, not the
real `metrics.yaml` with specific bean/attribute filters.)

The `metrics.yaml` is loaded by `processNewConfig` via
`check.CollectDefaultMetrics()`. Discovery templates have `instances: []`,
which causes `IsJMXConfig` to return false. Fixed by adding a fallback
in `CollectDefaultMetrics` that checks `init_config` for `is_jmx: true`
when `config.IsDiscovery()` is true, limited to discovery configs only.

## Files Changed

### JMXFetch (`~/dd/jmxfetch/`, commit `77c6b8e`)

| File | Description |
|---|---|
| `AppConfig.java` | Added `ACTION_DISCOVER`, `--integration`, `--service_json` params |
| `App.java` | Added `discoverConfig()` method, handles discover action |
| `JmxDiscovery.java` | Changed to verification: `verifyIntegration(name, domains)` |
| `JmxFetch.java` | Suppresses logging to stdout for discover action |
| `Instance.java` | Reverted `discovery: true` flag handling |
| `Connection.java` | `getMBeanServerConnection()` (kept from initial PoC) |

### Agent (worktree, commit `7be9abe2615`)

| File | Description |
|---|---|
| `pkg/jmxfetch/discovery.go` (new) | `RunDiscovery()` — one-shot subprocess with 60s timeout |
| `pkg/jmxfetch/stub.go` | `RunDiscovery` stub for non-JMX builds |
| `discoverer/jmx_bridge.go` | Calls `RunDiscovery()` instead of generating hardcoded config |
| `discoverer/composite.go` (removed) | Routing at worker level now |
| `impl/autoconfig.go` | Passes both bridges separately |
| `impl/configmgr.go` | Accepts two ConfigDiscoverers |
| `impl/configmgr_discovery.go` | Separate Python and JMX workers, preserves template init_config |
| `impl/configmgr_nodiscovery.go` | Updated signatures |
