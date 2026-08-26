# Approach 0: One-Shot Subprocess — Test Results

## Summary

Approach 0 (one-shot JMXFetch subprocess per discovery probe) was
implemented and tested. The JMXFetch `discover` action works correctly:
it connects to candidate JMX ports, inspects MBean domains, verifies
the integration's expected domains are present, and outputs a config
JSON on stdout. Error cases (no JMX, unknown integration, unreachable
host) all return `[]` with exit 0 — no errors generated.

The full Agent → JMXFetch discovery flow (via the discovery worker,
JMX bridge, and one-shot subprocess) was not tested end-to-end because
the locally-built agent binary was missing workloadmeta collector
registrations (the `go build` command doesn't include all the
init() registrations that the full build system wires up). The
JMXFetch side was tested manually and works. The Agent-side Go code
compiles cleanly with `jmx,python` build tags.

## JMXFetch Discover Action Tests

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

**Command:**
```bash
docker exec dd-agent-approach0 java -classpath ... discover \
  --integration kafka \
  --service_json '{"id":"docker://test","host":"kafka","ports":[{"number":9092,"name":""}]}'
```

**Output:** `[]`
**Exit code:** 0

**What happened:** Port 9092 is not a JMX port. Connection attempt failed.
No other ports to try. Output `[]`, exit 0. No errors.

### Test 3: Unknown integration

**Command:**
```bash
docker exec dd-agent-approach0 java -classpath ... discover \
  --integration unknown \
  --service_json '{"id":"docker://test","host":"kafka","ports":[{"number":9999,"name":"jmx"}]}'
```

**Output:** `[]`
**Exit code:** 0

**What happened:** "unknown" is not in StandardJMXIntegrations. No
signature to verify against. Output `[]`, exit 0.

### Test 4: Unreachable host

**Command:**
```bash
docker exec dd-agent-approach0 java -classpath ... discover \
  --integration kafka \
  --service_json '{"id":"docker://test","host":"unreachable.invalid","ports":[{"number":9999,"name":"jmx"}]}'
```

**Output:** `[]`
**Exit code:** 0

**What happened:** Connection to unreachable.invalid:9999 failed.
Output `[]`, exit 0. No errors.

## Full AD Flow Test (with stock agent)

Since the locally-built agent binary was missing workloadmeta collector
registrations, the full discovery worker flow was tested with the stock
nightly agent image + modified jmxfetch jar, using a conf.d template
with `ad_identifiers` (not `discovery: {}`).

### Config template used:
```yaml
ad_identifiers:
  - cp-kafka

init_config:
  is_jmx: true
  collect_default_metrics: true
  new_gc_metrics: true

instances:
  - host: "%%host%%"
    port: 9999
    collect_default_jvm_metrics: true
```

### Result:
- Agent discovered Kafka container via AD identifiers
- JmxScheduler scheduled the config
- JMXFetch collected **28 metrics** per cycle (default JVM metrics)
- No errors

This confirms the JMXFetch jar with the `discover` action works
correctly in the agent container. The `discover` action was also
tested manually (above) and produces correct output.

## What's NOT Tested Yet

The full discovery worker flow (Agent → discovery worker → JMX bridge →
one-shot subprocess → result → schedule config) requires a properly
built agent binary with all workloadmeta collectors registered. The
`go build` command used in this PoC doesn't include all the init()
registrations that the full build system (omnibus/bazel) wires up.

To test the full flow:
1. Build the agent using the full build system (omnibus or bazel)
2. Create a Docker image with the properly built agent + modified jmxfetch jar
3. Use a conf.d template with `discovery: {}` (triggers the discovery worker)
4. Verify that the discovery worker calls the JMX bridge, which runs
   the one-shot subprocess, which discovers Kafka and returns a config
5. Verify that the config is scheduled and JMXFetch collects Kafka-specific metrics

## Performance Notes

- JVM cold start: ~1-2 seconds per probe
- Probe time (connect + inspect + verify): ~1-3 seconds
- Total per-probe: ~2-5 seconds
- With bounded concurrency (1-2 JVMs) and per-service dedup, this is
  acceptable for rare one-time discovery
- The subprocess uses -Xmx200m -Xms50m (same as the regular JMXFetch)

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
