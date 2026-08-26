# JMX Configuration Discovery — Design Summary

## Problem

JMX integrations (Kafka, ActiveMQ, Tomcat, Cassandra, Solr) run through
JMXFetch, a Java subprocess managed by the core agent. The existing
configuration-discovery mechanism only works for Python integrations: it
calls `discover_config()` on the Python check class via rtloader, which
**actually runs the check** to verify the config works before scheduling it.

We need the same for JMX: a discovery probe that connects to a candidate
JMX endpoint, inspects MBeans to auto-detect the application type, verifies
that metrics can be collected, and only then returns a config to be
scheduled. If the endpoint is unreachable or no metrics are collected, the
probe returns nothing — **no config is scheduled, no error service checks are
generated**.

## Existing Interface

The only interface between Agent and JMXFetch is two HTTP endpoints, both
initiated by JMXFetch → Agent:

- `GET /agent/jmx/configs?timestamp=<ts>` — JMXFetch polls for scheduled
  configs (every 15s default)
- `POST /agent/jmx/status` — JMXFetch posts status back

There is no Agent → JMXFetch channel. Everything is pull-from-agent /
push-to-agent. The agent starts JMXFetch as a subprocess with
`--ipc_host`, `--ipc_port`, and `SESSION_TOKEN` env var.

## Python Discovery Pattern (reference)

The Python discovery bridge calls `discover_config(cls, service_json)` which:
1. Parses service JSON (host, ports)
2. Generates candidate configs from `candidate_ports()`
3. For each candidate: **actually runs the real check** via `check.run()`
4. Accepts the candidate **only if check.run() succeeds AND collects ≥1
   metric** (`stats.metric_count > 0`)
5. Returns the accepted config as JSON, or `"[]"` if none accepted
6. If `"[]"` or error → worker retries silently, no config scheduled, no
   errors

Key principle: **the check is actually run against the target**. If it
can't connect or collect metrics, the candidate is rejected silently.

## Approaches Considered

### Approach 0: One-shot Subprocess per Probe

Run JMXFetch as a one-shot subprocess (like `list_everything`) with a new
`discover` action. The subprocess connects, inspects MBeans, runs one
collection iteration, outputs config JSON, exits.

Details: [approach-0-one-shot-subprocess.md](approach-0-one-shot-subprocess.md)

| Pros | Cons |
|---|---|
| Matches Python pattern (synchronous verify) | JVM startup ~1–2s per probe |
| No new channels needed | Multiple probes × retries = expensive |
| Simple to implement | Needs bounded concurrency + rate limiting |
| No impact on running JMXFetch instances | |
| Natural process isolation and timeouts | |
| Clean stdout result contract | |
| No bootstrap deadlock | |

**Verdict:** Reconsidered as near-term implementation. With bounded
concurrency (1–2 JVMs), per-service dedup, and rate limiting, the JVM
startup cost is acceptable for rare one-time discovery probes. See
[DESIGN-CRITIQUE.md](DESIGN-CRITIQUE.md) for the full argument.

---

### Approach 1: Discovery HTTP Server on JMXFetch

Add a lightweight HTTP server (`com.sun.net.httpserver.HttpServer`, built
into JDK 11) to the running JMXFetch process. The agent's `JmxBridge` sends
a synchronous `POST /discover` with the service JSON, and JMXFetch
responds with the verified config JSON or `[]`.

Details: [approach-1-discovery-http-server.md](approach-1-discovery-http-server.md)

| Pros | Cons |
|---|---|
| Reuses running JVM (no startup cost) | New HTTP server in JMXFetch — significant architecture change |
| Synchronous (matches `ConfigDiscoverer` interface) | New port to manage, secure, configure |
| Low latency | Harder to get accepted in review |
| Full verification before scheduling | Agent → JMXFetch direction is new |

**Verdict:** Technically best, but significant architecture change. A
better long-term alternative is dedicated pull endpoints on the existing
Agent IPC server (Approach 4), which avoids adding a server to JMXFetch.

---

### Approach 2: Error Suppression with Async Verification (recommended)

Schedule a candidate config immediately through the normal JMX scheduler
(through the discovery worker, same as Python integrations). The instance
carries a `discovery: true` flag. JMXFetch picks it up via existing
`/configs` polling and handles it as follows:

- **Connection fails** → silently retry next cycle. No error service checks,
  no error status. The instance stays in `brokenInstanceMap` but
  `processStatus` and `sendServiceCheck` are skipped for discovery instances.
- **Connection succeeds** → inspect MBean domains, match app signatures,
  add conf entries, start collecting metrics normally. Report OK status.

Details: [approach-2-error-suppression.md](approach-2-error-suppression.md)

| Pros | Cons |
|---|---|
| No new channels or endpoints | Higher latency (≤15s poll interval) |
| No new HTTP server | **Config scheduled before verification** |
| Minimal JMXFetch changes (error suppression only) | **Agent doesn't know if discovery succeeded** |
| Works within existing architecture | **Discovery telemetry useless** |
| Easy to get accepted in review | **Fleet Automation shows unverified config** |

**Verdict:** Rejected. The critique identified fundamental problems:
bootstrap deadlock, global reinit churn, lossy result delivery, wrong
validation. See [DESIGN-CRITIQUE.md](DESIGN-CRITIQUE.md).

---

## Approach 3: Dummy Config via Existing Channels (recommended)

Reuse both existing IPC channels in a novel way: the agent includes a
"dummy discovery config" (carrying the serialized service struct) in the
`/configs` response; JMXFetch processes it, runs the probe (connect,
inspect MBeans, collect one iteration), and posts the result via
`/status`. `DiscoverConfig()` blocks until the result arrives (or times
out), then proceeds exactly like the Python path.

Details: [approach-3-dummy-config.md](approach-3-dummy-config.md)

| Pros | Cons |
|---|---|
| No new channels or endpoints | Latency: ≤15s poll + 5s probe = ≤20s |
| Config NOT scheduled before verification | Blocks a worker goroutine during wait |
| Agent knows if discovery succeeded | Requires separate JMX workqueue (see analysis) |
| Discovery telemetry works | Status struct needs new field |
| No error service checks on failure | |
| Fleet Automation status stays clean | |
| Works within existing architecture | |

**Verdict:** Rejected. The critique identified fundamental problems:
bootstrap deadlock (JMXFetch not started), global reinit churn (every
discovery request disrupts all running JMX integrations), lossy result
delivery (/status is not a reliable RPC), wrong validation (JVM metrics
don't prove app detection), and IsJMXConfig misclassification with empty
instances. See [DESIGN-CRITIQUE.md](DESIGN-CRITIQUE.md).

---

## Approach 4: Dedicated Pull Endpoints on Existing Agent IPC Server

Add two new endpoints to the Agent's existing HTTPS IPC server (the same
server that already serves `/agent/jmx/configs` and `/agent/jmx/status`):

- `GET /agent/jmx/discovery/requests?cursor=N` — JMXFetch polls for
  pending discovery requests (with monotonic sequence numbers)
- `POST /agent/jmx/discovery/results` — JMXFetch posts discovery results

JMXFetch remains the HTTP client; no new server or port on JMXFetch.
This preserves the existing communication direction.

Details: [approach-4-pull-endpoints.md](approach-4-pull-endpoints.md)

| Pros | Cons |
|---|---|
| No new JMXFetch server or port | New endpoints on Agent IPC server |
| Preserves existing communication direction | More complex protocol than one-shot |
| Monotonic versioning, reliable delivery | Needs lease/ack protocol |
| Separate JMXFetch discovery executor | |
| No reinit churn on regular configs | |
| Long-term production solution | |

**Verdict:** Best long-term solution. More work than Approach 0 but
cleaner protocol. Can be built after Approach 0 proves the concept.

---

## Recommendation

**Near term: Approach 0** (one-shot subprocess with bounded concurrency).
It's simple, correct, and avoids all the protocol issues identified in
the critique. The JVM startup cost (~1–2s) is acceptable for rare,
one-time discovery probes. With bounded concurrency (1–2 concurrent
JVMs), per-service dedup, and rate limiting, it's operationally safe.

**Long term: Approach 4** (dedicated pull endpoints). Once the concept is
proven, add proper pull endpoints to the Agent IPC server with monotonic
versioning, reliable delivery, and a separate JMXFetch discovery executor.
This eliminates the JVM startup cost and provides a clean, reliable
protocol.

**Key design principles** (from critique, apply to both):

1. **Don't replace integration metric definitions.** Discovery should
   vary only the connection instance (host, port), not replace the
   integration's `metrics.yaml`. The template's `init_config` and
   `MetricConfig` must be preserved.
2. **Use application-specific validation.** Don't just check
   `metric_count > 0` (any JVM produces JVM metrics). Require at least
   one metric from the intended integration's application-specific rules,
   or use an explicit identity predicate (domain/ObjectName check).
3. **The integration is already known.** The discovery template carries
   the integration name. Don't auto-detect from a global registry —
   verify that the specific integration's expected MBeans are present.
4. **Store integration-owned data in integrations-core.** Preferred
   ports, identity ObjectNames, and required metric selectors should live
   with each integration, not hardcoded in JMXFetch.

## Related Files

- [initial-poc-findings.md](initial-poc-findings.md) — Research findings
  and initial PoC test results
- [approach-0-one-shot-subprocess.md](approach-0-one-shot-subprocess.md) —
  One-shot subprocess approach
- [approach-1-discovery-http-server.md](approach-1-discovery-http-server.md)
  — Discovery HTTP server approach
- [approach-2-error-suppression.md](approach-2-error-suppression.md) —
  Error suppression approach (rejected)
- [approach-3-dummy-config.md](approach-3-dummy-config.md) —
  Dummy config approach (rejected — see critique)
- [approach-3-blocking-analysis.md](approach-3-blocking-analysis.md) —
  Blocking analysis for Approach 3
- [approach-4-pull-endpoints.md](approach-4-pull-endpoints.md) —
  Dedicated pull endpoints on Agent IPC server (long-term)
- [DESIGN-CRITIQUE.md](DESIGN-CRITIQUE.md) —
  Critique of Approach 3 with suggested alternatives
