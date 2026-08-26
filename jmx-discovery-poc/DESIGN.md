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
| Simple to implement | Not suitable for production |

**Verdict:** Rejected for production. Acceptable as a stepping stone only.

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

**Verdict:** Technically best, but significant architecture change. Keep as
a future optimization if Approach 2 proves insufficient.

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
| No new HTTP server | Config is scheduled before verification (unlike Python) |
| Minimal JMXFetch changes (error suppression only) | Error suppression must be carefully scoped |
| Works within existing architecture | Agent doesn't know if discovery succeeded |
| Easy to get accepted in review | |

**Verdict:** Recommended. Minimal changes, works within existing
architecture, preserves the key property (no error service checks on
discovery failure). Can be upgraded to Approach 1 later if lower latency
is needed.

---

## Recommendation

**Approach 2** for initial implementation. It requires:

- **Agent**: JMX discovery bridge that generates candidate config from
  service ports, composite discoverer to route JMX integrations
- **JMXFetch**: `discovery: true` flag handling in Instance, MBean
  inspection via `JmxDiscovery`, error suppression in `App.java` for
  discovery instances

No new endpoints, no new HTTP servers, no new command-line params. The
only JMXFetch behavioral change is suppressing error service checks for
instances with `discovery: true`.

## Related Files

- [initial-poc-findings.md](initial-poc-findings.md) — Research findings
  and initial PoC test results
- [approach-0-one-shot-subprocess.md](approach-0-one-shot-subprocess.md) —
  One-shot subprocess approach
- [approach-1-discovery-http-server.md](approach-1-discovery-http-server.md)
  — Discovery HTTP server approach
- [approach-2-error-suppression.md](approach-2-error-suppression.md) —
  Error suppression approach (recommended)
