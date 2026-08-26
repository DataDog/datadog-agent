# Approach 0: Full E2E Test Results

## Summary

The full end-to-end discovery flow was tested with a properly built agent
binary (built with `dda inv agent.build`) running in a Docker container
with Kafka. The flow works:

1. Agent discovers Kafka container via AD identifiers
2. Discovery worker enqueues a JMX probe
3. JMX bridge runs a one-shot JMXFetch subprocess in `discover` mode
4. Subprocess connects to Kafka's JMX endpoint, verifies MBean domains
5. Subprocess returns config JSON
6. Discovery worker schedules the verified config
7. Long-running JMXFetch picks up the config and starts collecting metrics

## Test Setup

- **Agent image**: `datadog/agent-dev:nightly-main-py3-jmx` base + agent
  binary built with `dda inv agent.build --build-exclude=systemd` +
  modified jmxfetch jar (with `discover` action)
- **Kafka**: `confluentinc/cp-kafka:7.7.0` with JMX on port 9999
- **Config template**: `conf.d/kafka.d/conf.yaml` with `discovery: {}` and
  `ad_identifiers: [cp-kafka]`

## Key Log Evidence

### Discovery subprocess ran (two attempts)

```
12:41:42 | JMX discovery: running subprocess: java [...] discover --integration kafka --service_json {"id":"docker://...","host":"172.17.130.4","ports":[...]}
12:41:43 | JMX discovery: subprocess found no valid JMX config for integration kafka
```
First attempt failed — Kafka's JMX endpoint wasn't ready yet. The worker
retried after the 15s delay.

```
12:41:58 | JMX discovery: running subprocess: java [...] discover --integration kafka --service_json {"id":"docker://...","host":"172.17.130.4","ports":[...]}
12:41:58 | JMX discovery: subprocess succeeded for integration kafka, result length: 151
```
Second attempt succeeded — Kafka JMX was now available, the subprocess
connected, verified kafka.server/kafka.controller MBean domains, and
returned a 151-byte config JSON.

### Config scheduled with discovery tag

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
discovery worker path. The `config.provider` is `ad-container-discovery+file`,
confirming it was resolved via the discovery worker, not normal template
resolution.

### JMXFetch collecting metrics

```
12:42:59 | Instance kafka-172.17.130.4-9999 is sending 28 metrics to the metrics reporter during collection #5
12:43:14 | Instance kafka-172.17.130.4-9999 is sending 28 metrics to the metrics reporter during collection #6
12:43:29 | Instance kafka-172.17.130.4-9999 is sending 28 metrics to the metrics reporter during collection #7
...
```

JMXFetch is stably collecting 28 metrics per cycle (default JVM metrics).

## Known Issue: init_config Not Preserved

The scheduled config shows `init_config: {}` instead of the template's
`init_config` (which has `is_jmx: true`, `collect_default_metrics: true`,
`new_gc_metrics: true`).

**Root cause**: `parseDiscoveryResult()` in `discovery_json.go` always sets
`init_config` to `json.RawMessage("{}")` when the discovered config has
no init_config. Our `isEmptyJSON()` check correctly identifies `"{}"` as
empty and should preserve the template's init_config. However, the template's
init_config appears to also be empty in the scheduled config, suggesting the
merge or resolution step is stripping it.

**Impact**: JMXFetch collects only 28 default JVM metrics instead of
Kafka-specific metrics (350+ in the initial PoC). The `is_jmx: true` flag
is missing, but JMXFetch still recognizes "kafka" as a JMX integration via
`StandardJMXIntegrations`.

**Fix needed**: Debug why `tpl.InitConfig` is empty after the merge, or
ensure the JMXFetch subprocess returns the integration's init_config
along with the instance config.

## Performance

- First probe (failed): ~1s (subprocess startup + connection attempt)
- Second probe (succeeded): ~1s (subprocess startup + connect + inspect)
- Total time from agent start to config scheduled: ~18s
  (agent startup + AD discovery + first probe failure + 15s retry + second probe)
- JVM cold start: ~1s per probe
- No impact on running JMXFetch instances (subprocess is isolated)

## Conclusion

The full discovery worker flow works end-to-end:
- AD discovers the Kafka container
- Discovery worker routes to JMX worker (separate from Python worker)
- JMX bridge runs a one-shot JMXFetch subprocess
- Subprocess verifies the JMX endpoint and returns config JSON
- Config is scheduled with `dd_config_discovery:true` tag
- JMXFetch collects metrics

The `init_config` preservation issue needs to be fixed to collect
Kafka-specific metrics, but the core discovery flow is proven.
