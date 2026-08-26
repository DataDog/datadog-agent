# Critique of the Recommended JMX Configuration Discovery Approach

## Conclusion

The "Recommended" Approach 3 should not be implemented as written. It
preserves the desired synchronous API on paper, but the existing JMXFetch
lifecycle and polling protocol do not provide the request-delivery or
result-delivery guarantees it assumes.

The recommended direction is:

- Near term: reconsider Approach 0, with bounded concurrency and rate
  limiting.
- Longer term: add a dedicated pull-based discovery RPC to the Agent's
  existing IPC server. JMXFetch remains the HTTP client, so this requires no
  new JMXFetch server or port.

## Critical Problems with Approach 3

### 1. It deadlocks when discovery is the first JMX workload

JMXFetch is started only when a JMX config is scheduled in
`pkg/jmxfetch/state.go`. Approach 3 deliberately registers a discovery request
before scheduling any config.

On a host with no existing JMX check:

```text
DiscoverConfig waits for JMXFetch
JMXFetch waits for a scheduled config before starting
```

The documented 60-second timeout would repeat until discovery is abandoned.
Approach 1 has the same bootstrap problem unless discovery explicitly starts
JMXFetch.

### 2. Pending requests are not reliably visible through `/configs`

The endpoint returns 204 based only on the scheduled-config cache timestamp in
`comp/api/api/apiimpl/internal/agent/agent_jmx.go`. Registering a separate
pending request does not update that timestamp.

Even if this is patched, the protocol uses second-resolution timestamps, and
JMXFetch only accepts a response whose timestamp is strictly greater than its
last value in `App.java`. Multiple request additions or removals within one
second can be ignored.

Approach 3 needs a monotonic version owned by the combined config/discovery
snapshot, not `time.Now().Unix()`.

### 3. Each request can disrupt every running JMX integration

When JMXFetch observes a changed `/configs` response, it sets `reinit`;
`init(true)` then clears and recreates all instances.

Therefore:

- Adding a dummy request reconnects every real JMX instance.
- Removing it causes another full reinitialization.
- Multiple discovery requests or retries produce repeated global churn.
- Regular JMX collection pauses while discovery executes.

The proposed separate Go workqueue does not solve contention inside JMXFetch.

### 4. The discovery probe is placed on JMXFetch's main lifecycle path

The design says `processDiscoveryConfigs()` runs synchronously during `init()`.
JMX connection defaults are 20 seconds for connection and 15 seconds for
client operations in `RemoteConnection.java`, not the assumed five seconds.
Trying several exposed ports can take much longer than the claimed 20 seconds
while existing checks are stopped.

Discovery needs a separate bounded executor that never participates in normal
instance reinitialization or collection.

### 5. Result delivery is lossy and has no acknowledgement protocol

The current status implementation clears state after every flush, including
failed POSTs, in `Status.java`. The proposal similarly says to clear discovery
results after flushing.

Missing pieces include:

- Delivery acknowledgement.
- Retention after a transient HTTP failure.
- Idempotent processing by request ID.
- Deduplication of repeated dummy configs.
- Cancellation when the service disappears.
- Handling a result arriving after the Agent timeout or unregistration.
- Recovery across a JMXFetch or Agent restart.

`/status` is presently a latest-snapshot telemetry mechanism, not a durable
request/reply transport.

### 6. The recommended merge can discard the integration's real metrics configuration

The Agent loads `metrics.yaml` and injects its entries before normal scheduling
in `comp/core/autodiscovery/impl/autoconfig.go`. Discovery currently replaces
`InitConfig` and `MetricConfig` with the returned values in
`configmgr_discovery.go`.

The design's generated domain-wide entries are not equivalent to the
integration definitions:

- Kafka's `metrics.yaml` contains detailed bean and attribute filters, aliases,
  and metric types.
- The POC replaces these with broad `kafka.server` and `kafka.controller`
  domain includes.
- Similar differences exist for Tomcat, Cassandra, ActiveMQ, and Solr.

Discovery should vary only the connection instance—host, port, and perhaps
path—not replace the integration-owned metric definitions.

### 7. "At least one metric" can validate the wrong thing

Candidates enable default JVM metrics. Any reachable Java process can emit JVM
metrics, so `metric_count > 0` alone does not prove that Kafka, Cassandra, or
another intended application was found.

The probe should require either:

- At least one metric produced by the intended integration's
  application-specific rules; or
- An explicit integration-specific identity predicate followed by a real
  collection using its normal `metrics.yaml`.

The intended integration is already known from the matched discovery template.
Auto-detecting one application from a global registry is unnecessary and risks
selecting the wrong integration on shared or proxy JMX servers.

### 8. The separate-worker proposal misclassifies empty discovery templates

The blocking analysis proposes routing with `check.IsJMXConfig(tpl)`. But
`IsJMXConfig` iterates over instances, while the proposed discovery template
has `instances: []`. It therefore returns false and routes the JMX job
incorrectly.

Routing needs an explicit discovery backend/type or a name/init-config-aware
classifier that works without instances.

### 9. The POC does not validate Approach 3

The tested `jmx-discovery-poc/conf.d/kafka.d/conf.yaml` has an instance-level
`discovery: true`, but no top-level `discovery: {}`. It exercises the
already-scheduled runtime path, not:

- The discovery worker.
- The dummy request registry.
- Timestamp delivery.
- Blocking and unblocking.
- Status result delivery.
- Pre-scheduling verification.

The "350 metrics" result also does not assert canonical Kafka metric names,
aliases, types, or parity with Kafka's real `metrics.yaml`.

## Better Alternatives

### 1. Reconsider Approach 0 as the first production implementation

The rejection is based on an unmeasured JVM startup concern, while the blocking
analysis simultaneously argues that JMX discovery is rare—usually zero to two
probes. Those claims cut against each other.

A bounded one-shot design offers:

- No bootstrap deadlock.
- No impact on running JMXFetch instances.
- Natural synchronous semantics.
- Process isolation and straightforward timeouts.
- A clean stdout result contract.
- Easy cancellation when a service disappears.
- No request/ack protocol hidden inside config and status messages.

It can be made operationally safe with:

- At most one or two concurrent JVMs.
- Per-service deduplication.
- Global rate limiting.
- Exponential startup retry rather than repeated immediate spawning.
- Service JSON and the full integration config over stdin.
- A no-op/counting reporter.
- An application-specific metric requirement.

Before rejecting it, benchmark cold start, peak RSS, and startup bursts on
realistic nodes. A two-second isolated probe may be preferable to a 10–60
second poll/reinitialize workflow.

### 2. Add dedicated pull endpoints to the existing Agent IPC server

A better long-term protocol is:

```text
JMXFetch -> GET  /agent/jmx/discovery/requests?cursor=N
JMXFetch -> POST /agent/jmx/discovery/results
```

This preserves the existing communication direction and uses the existing
Agent HTTPS server, IPC port, and session token. It does not require the
Approach 1 JMXFetch-side HTTP server.

The protocol should have:

- Monotonic request sequence numbers.
- Request IDs and idempotent results.
- Leases or explicit acknowledgements.
- Result retention until acknowledgement.
- Cancellation or tombstones.
- A separate JMXFetch discovery executor.
- On-demand `EnsureJMXFetchRunning()` when the first request is registered.
- Optional long polling to reduce the 15-second delay.

This is more explicit and likely less code overall than making `/configs` and
`/status` emulate reliable RPC.

### 3. Reconsider a narrow data-driven approach

The Confluence proposal rejected generic data-driven discovery because the
Agent would accumulate integration-specific probe mechanisms. That argument is
valid globally, but weaker for JMX: all these integrations share one protocol
and one existing execution engine.

Keep the generic engine in JMXFetch, but store integration-owned data in
integrations-core:

- Preferred or known ports.
- JMX path variants.
- Identity ObjectNames or domains.
- Required application-specific metric selectors.
- The normal `metrics.yaml`.

This avoids the hardcoded registry in `JmxDiscovery.java` and keeps behavior
versioned and tested with each integration. The original configuration-
discovery proposal explicitly prioritizes integration ownership and
testability; the recommended JMX design currently moves that ownership into
JMXFetch.

### 4. Reconsider trial mode as a two-phase state machine

Approach 2 should not be adopted as simple error suppression, but a stronger
form is viable:

```text
candidate pending -> verified by real Instance -> promoted to scheduled
                  -> rejected silently
```

The candidate must remain outside Fleet-visible scheduled state until
promotion. This can reuse normal `Instance` construction and the real metric
configuration, avoiding a second probe implementation.

It still needs a reliable result channel, but it is conceptually cleaner than
inventing dummy configs with a separate temporary-instance path.

## Scope Concerns Before Investing

The associated Confluence analysis places 14 integrations in a
`creds-jmx-rmi` blocker category, and the blocked-integrations roadmap notes
that Tomcat's observed legacy errors are overwhelmingly credential-related
rather than port-related.

Automatic JMX discovery cannot infer:

- Credentials.
- Trust/key stores.
- Registry SSL.
- Custom JMX URLs or paths.
- RMI-advertised hosts or secondary ports.

The eligible population may therefore be mostly deliberately unauthenticated
JMX endpoints. That population and its product value should be measured before
accepting substantial cross-repository protocol complexity.

For integrations already shipping `auto_conf.yaml`, follow the conservative
rollout from the migration analysis: shadow evaluation or an explicit fallback
phase is safer than immediate replacement.

## References

- [DESIGN.md](DESIGN.md)
- [Initial POC findings](initial-poc-findings.md)
- [Approach 0: one-shot subprocess](approach-0-one-shot-subprocess.md)
- [Approach 1: discovery HTTP server](approach-1-discovery-http-server.md)
- [Approach 2: error suppression](approach-2-error-suppression.md)
- [Approach 3: dummy config](approach-3-dummy-config.md)
- [Approach 3 blocking analysis](approach-3-blocking-analysis.md)
- [Configuration Discovery for Agent Integrations](https://datadoghq.atlassian.net/wiki/spaces/DSCVR/pages/6671862234/Configuration+Discovery+for+Agent+Integrations)
- [Blocked priority integrations in Config Discovery](https://datadoghq.atlassian.net/wiki/spaces/DSCVR/pages/7076840135/Blocked+priority+integrations+in+Config+Discovery+diagnosis+and+roadmap)
- [Converting existing shipped auto configs to configuration discovery](https://datadoghq.atlassian.net/wiki/spaces/DSCVR/pages/6858573622/Converting+existing+shipped+auto+configs+to+integration+configuration+discovery)
