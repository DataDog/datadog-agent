# Agent Telemetry (COAT) — component guide

`comp/core/agenttelemetry` is the component behind **COAT** (Continuous Observability
of Agent Telemetry). It does **not** create metrics. It *selects* a subset of metrics
already registered through `pkg/telemetry`, aggregates them, and flushes them on a
schedule to the internal agent-telemetry intake (the COAT org, not the customer org).

- Metric/event selection + schedules: `impl/defaultProfiles.yaml` (hand-edited, `//go:embed`)
- Config schema + authoritative field docs: `impl/config.go`
- Payload build + diagnose endpoint: `impl/agenttelemetry.go`
- Component interface: `def/component.go`

## Adding a new COAT metric

### Prerequisite (only if the metric doesn't exist yet)
COAT can only select metrics already registered through `pkg/telemetry`. If yours already
exists and is compliant, skip this and go straight to step 1.

Otherwise, create it first and make it compliant before it can go into the default profiles:

1. Register it with the normal `pkg/telemetry` APIs (`NewGauge`, `NewCounter`,
   `NewHistogram`, …).
2. Name it `<subsystem>.<name>` (subsystem = first arg, name = second arg).
3. Do **not** pass `Options.NoDoubleUnderscoreSep` — COAT does not support it.
4. Choose a tag set that is safe to collect (no customer data / PII — see "Allowlist tags"
   below). A metric is only eligible for the default profiles once it is compliant.

> **Nested-subsystem gotcha.** Only the *first* `__` of the Prometheus name is normalized
> to `.`. A metric registered with subsystem `gpu__collectors`, name `created` becomes
> `gpu.collectors__created` in the profile YAML — keep the second `__`. See the `gpu`
> profile for a live example.

### 1. Add the metric to a profile in `defaultProfiles.yaml`

```yaml
- name: checks
  metric:
    exclude:
      zero_metric: true      # drop zero-valued series
    metrics:
      - name: your_group.your_metric
        preserve_tags:
          - some_tag
```

Create a new profile (with its own `schedule`) only if the cadence/grouping doesn't fit an
existing one.

### 2. Allowlist tags with `preserve_tags` — this is a privacy control
Every series always carries the system tag `emitter`. Every *other* tag is dropped unless
listed:

- Omitted/empty → aggregates by `emitter` alone (all other labels collapsed and summed).
- Listed → a series is kept if it carries **at least one** of the listed labels (any-match,
  not all); a series carrying **none** of them is dropped. Among kept series, only `emitter`
  and the listed labels are emitted.

Only preserve tags that are safe to send. Never preserve tags carrying customer data,
hostnames, container names, or user input. When unsure, leave it off.

- `aggregate_tags` — **deprecated** alias; use `preserve_tags`. If both present,
  `preserve_tags` wins.
- `aggregate_total: true` — emits a per-emitter total with a reserved `total` tag holding
  that emitter's source-series count.

### 3. Exclude filters (optional)
- `exclude.zero_metric: true` drops zero-valued series (most profiles set this). If a metric
  sits at 0 it silently never appears — set `false` when 0 is meaningful.
- `exclude.tags: [status:disabled]` drops series by tag or tag:value (applies to every
  metric in the profile). The `gpu` profile uses this to drop `status:unsupported` /
  `status:disabled`.

### 4. Schedule (optional) — default if omitted:

```yaml
schedule:
  start_after: 30     # seconds after agent start
  iterations: 0       # 0 = forever
  period: 900         # 15m between collections
```

### 5. remote_flag (optional)
`remote_flag: <name>` gates the profile behind a Remote Config flag; it's only collected
while that flag is enabled.

> ⚠️ **Collection requires an agent release.** `defaultProfiles.yaml` is compiled into the
> agent binary (`//go:embed`). Editing it does **not** change what already-deployed agents
> collect. Customer telemetry for a newly added metric only starts flowing once a release
> containing the change ships and customers upgrade to it. Plan the rollout accordingly.
> (A custom `agent_telemetry.profiles` config can be used for local/ad-hoc testing, but the
> default profiles are what collect customer telemetry, and only via a release.)

## Adding an event (not a metric)
Events go through `Component.SendEvent(eventType, payload)`. `eventType` must be registered
in `impl/config.go` (name → `request_type` / `payload_key` / `message`) and listed under a
profile's `events[]`. Payload must serialize to JSON.

## Pitfalls
- **Don't edit generated config.** `defaultProfiles.yaml` is hand-edited and embedded; no
  codegen.
- **Error-log flush recursion.** The flush path (`sendLogsBatch → sendPayload`) MUST NOT log
  at Error or above — it re-enters the slog handler and feeds records back into the same
  channel. Enforced by convention only. See the `SubmitErrorLog` doc in `def/component.go`
  and `impl/errortracking_sender.go`.
- **Gazelle.** After adding/renaming `.go` files run
  `bazel run //:gazelle -- comp/core/agenttelemetry/...` and commit the `BUILD.bazel` diff.

## Testing

### Unit tests (`impl/agenttelemetry_test.go`)
Run: `dda inv test --targets=./comp/core/agenttelemetry/...`

The established pattern: register a metric on a mock telemetry registry, drive a run, and
assert the aggregated output. Minimal shape:

```go
tel := makeTelMock(t)
counter := tel.NewCounter("bar", "zoo", []string{"tag1", "tag2"}, "")
counter.AddWithTags(10, map[string]string{"tag1": "a1", "tag2": "b1"})
counter.AddWithTags(20, map[string]string{"tag1": "a2", "tag2": "b2"})

a := getTestAtel(t, tel, yamlConfig, sender, nil, runnerMock)
a.start()
runnerMock.run()

// preserve_tags: [] → one aggregated series, emitter-only
assert.Equal(t, float64(30), s.sentMetrics[0].metrics[0].Counter.GetValue())
assert.Equal(t, map[string]string{"emitter": "agent"}, metricLabels(m))
```

Useful existing coverage to extend / mirror:
- `TestNoTagSpecified*` — aggregation with empty `preserve_tags` (counter/gauge/histogram)
- `TestTagSpecifiedAggregation*` — aggregation that keeps listed tags
- `TestMandatoryEmitter*` — emitter is always injected; totals are per-emitter;
  `preserve_tags` act as user filters
- `TestAggregateTagsAliasBackwardCompat` — deprecated `aggregate_tags` still works
- `compileTestMetric(t, preserveTags, aggregateTotal)` — helper to build a `MetricConfig`

### Verifying a built agent locally

```bash
agent diagnose show-metadata agent-telemetry
```

This builds the payload on demand (and advances counter deltas), so assert on presence +
tags rather than exact values.

### E2E
`test/new-e2e/tests/gpu/gpu_test.go` → `TestCOATMetricsReported` is the reference pattern:
it calls `diagnose show-metadata agent-telemetry`, parses the payload, and asserts the
metric is selected with exactly the expected tag set (e.g. `gpu.collectors__created`). Note:
the payload is built by the **core agent**, so system-probe-only metrics aren't covered by
that path.
