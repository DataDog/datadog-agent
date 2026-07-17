# comp/anomalydetection — AI Agent Guide

## What This Subsystem Does

Edge anomaly detection inside the Datadog Agent. Data enters through lightweight
**observer handles**, is stored and analyzed by the **observer engine**, then
reported via **reporters** (stdout trace and optional Datadog change events).

```
Handle → Storage → Detect → Correlate → Report
```

## Component Tree

```
comp/anomalydetection/
  internal/logsfilter/     ← shared severity bucketing + rate limiting
  severityevents/
    def/                 ← Subscriber contract + severity event types (own go.mod)
    impl/                ← Dispatcher: one listener, cooldown, filtering
  observer/
    def/                 ← public interfaces (own go.mod)
    fx/                  ← production Fx wiring (python build tag)
    fx/fx_noop.go        ← IoT / !python stub
    impl/                ← engine, detectors, correlators, extractors
      patterns/          ← log tokenizer/clusterer subpackage
    scenarios/           ← replay scenario directories (testbench)
  logssource/
    def/ fx/ impl/       ← container + kubelet journald log ingestion
    fx/fx_noop.go        ← IoT / !python stub
  reporter/
    def/
    fx/                  ← production: stdout + optional event reporter
    fx-noop/             ← linter stub package only
    fx-testbench/        ← SSE reporter for testbench
    impl/                ← live reporter (stdout + event)
    impl-noop/           ← linter stub package only
    impl-testbench/      ← SSE hub + testbench reporter
    mock/
    reporter.allium      ← behavioral spec for reporter payloads
  recorder/
    def/
    fx-noop/             ← noop wired in production agent
    impl-noop/           ← noop implementation (full parquet impl planned)
```

## Agent Wiring

Wired in `cmd/agent/subcommands/run/command.go`:

| Module | Package | Role |
|--------|---------|------|
| Observer | `observer/fx` | Analysis pipeline (`python` build tag) |
| Log source | `logssource/fx` | Container + kubelet logs (`python` tag) |
| Reporter | `reporter/fx` | Stdout reporter + optional event reporter |
| Recorder | `recorder/fx-noop` | No-op (parquet middleware not shipped yet) |

**IoT / `!python` builds** use no-op `observer/fx` and `logssource/fx` modules.

**Testbench** (`internal/qbranch/anomalydetection-testbench/`) wires
`observer/fx`, `recorder/fx-noop`, and `reporter/fx-testbench`. It replays
scenarios from `observer/scenarios/` and has its own parquet tooling under
`internal/qbranch/anomalydetection-testbench/bench/`.

```bash
# Build the testbench binary
dda inv anomalydetection.build-testbench

# Launch backend + web UI (add --build to rebuild first)
dda inv anomalydetection.launch-testbench
```

See `internal/qbranch/anomalydetection-testbench/README.md` for flags and
headless/eval workflows.

## Data Ingress (Handle Sources)

Production callers of `observer.GetHandle()` use statically-defined source names:

| Source | Wired from |
|--------|------------|
| `dogstatsd` | `pkg/aggregator/demultiplexer_agent.go` (DogStatsD workers) |
| `check` | `pkg/aggregator/demultiplexer_agent.go` (core check aggregator) |
| `logs` | `logssource/impl/logssource.go` |
| `agent_logs` | `observer/impl/observer.go` (pkg/util/log tap) |

**Log ingestion split:**
- **Container + kubelet logs** → `logssource` component
- **Agent internal logs** → `observer` taps `pkg/util/log` directly via `agent_logs`

Both paths share filtering primitives from `internal/logsfilter/`.

Metrics with the `datadog.*` prefix are normalized as internal agent telemetry.
Only observer telemetry under `datadog.agent.observer.*` is dropped before it
reaches observer storage, preventing an ingestion loop.

## Severity Events (Scorer Push Contract)

`severityevents/def` defines the `Subscriber` interface
(`SubscribeSeverityEvents(cfg) SeverityEventsSubscription`) and the
`SeverityEvent`/`SeverityLevel`/`SeverityEventsConfiguration` types shared by any
consumer of anomaly scorer severity transitions. `severityevents/impl.Dispatcher`
is the concrete implementation: it owns one push listener plus one fixed
cooldown/filter state machine. The anomaly scorer
(`observer/impl/anomaly_scorer.go`) only derives the raw EWMA-based severity
level per second and feeds it into the dispatchers it owns — it does not
implement subscription logic itself.

Each `SubscribeSeverityEvents` call creates one new dispatcher bound to one
listener. If the scorer already knows the current level, it first seeds that
dispatcher before publishing it: `Medium`/`High` bootstrap as `Low -> current
level`, while `Low` emits no initial event. Before the scorer knows its
current level, new dispatchers also start at `Low`, so the first observed
`Medium`/`High` level emits a real escalation instead of being treated as a
pure seed.

`observer.Component.SubscribeSeverityEvents` structurally satisfies
`severityevents/def`'s `Subscriber` interface (same method signature), so any
caller holding an `observer.Component` can be passed directly wherever a
`Subscriber` is expected, without importing `observer` from the consuming side.

## Reporter Model

Reporters register through the `anomalydetection_reporters` Fx group
(`reporter/def`). The observer calls each injected `Reporter.Report()` after
every advance cycle.

**Reporters hold no deduplication state.** All first-seen and recurrence logic
lives inside each correlator via the shared `correlationEmitter` helper
(`observer/impl/correlation_emitter.go`). Reporters iterate
`ReportOutput.CorrelatorEvents` and dispatch directly — no per-reporter seen-map.

- **StdoutReporter** — always active in `reporter/fx`; logs
  `CorrelationDetected` events at info and ongoing active correlations at debug
- **EventReporter** — created when `anomaly_detection.reporting.events.enabled=true`
  AND the event-platform forwarder is available; dispatches change events for
  `CorrelationDetected` and scorer episode events via `reporter/impl/notify.go`.
  Not fully stateless: it carries a `retryPending` queue of `CorrelationDetected`
  sends that failed transiently; entries are retried each advance cycle and evicted
  after `defaultMaxRetryAttempts` consecutive failures.

`ReportOutput.CorrelatorEvents` carries three event kinds:
- `CorrelatorEventCorrelationDetected` — emitted by `TimeCluster`, `CrossSignal`,
  `Passthrough` at first-seen (and again after a pattern goes inactive and recurs)
- `CorrelatorEventEpisodeStarted` — emitted by `anomaly_scorer` when severity enters
  the configured correlation threshold (`medium` or `high`)
- `CorrelatorEventEpisodeEnded` — emitted by `anomaly_scorer` when severity exits
  the configured correlation threshold

See `reporter/reporter.allium` for the payload contract.

## Configuration

Keys are registered in `pkg/config/setup/common_settings.go`.

| Key | Default | Purpose |
|-----|---------|---------|
| `anomaly_detection.reporting.events.enabled` | `false` | Active gate for Datadog event reporting |
| `anomaly_detection.anomaly_scorer.dry_run.enabled` | `false` | Active gate for scorer telemetry without scorer outputs |
| `anomaly_detection.anomaly_scorer.output.correlation_event_threshold` | `high` | Lowest scorer severity that opens a correlation episode (`medium` or `high`) |
| `anomaly_detection.metrics.enabled` | `true` | External metric ingestion at handles |
| `anomaly_detection.metrics.processing_rules` | `[]` | Ordered metric filter rules (source/name/tags) |
| `anomaly_detection.recording.enabled` | `false` | Parquet recording middleware |
| `anomaly_detection.logs.enabled` | `true` | Parent gate for all log sources |
| `anomaly_detection.logs.processing_rules` | `[]` | Ordered log filter rules evaluated per message for all log sources (container, kubelet, agent-internal) |
| `anomaly_detection.logs.containers.enabled` | `true` | Workloadmeta container logs |
| `anomaly_detection.logs.kubelet.enabled` | `true` | Kubelet journald source |
| `anomaly_detection.logs.internal.enabled` | `true` | Agent-internal log tap |
| `anomaly_detection.detectors.<name>.enabled` | varies | Per detector/correlator/extractor |
| `anomaly_detection.storage.max_series` | `50000` | Storage series cap |
| `anomaly_detection.storage.point_retention` | `120s` | Per-series point retention |

Per-source log rate limits and min severity live under
`anomaly_detection.logs.{internal,kubelet,containers}.*`.

Log `processing_rules` fields: `type` (`exclude_at_match` / `include_at_match`), `name` (required),
`source` (e.g. `containerd`, `docker`, `kubelet`, `datadog-agent`), `tags` (list of `key:value`).
Rules are evaluated per-message for all sources (container, kubelet, agent-internal).
Both `source` and `tags` constraints are supported; first-match wins.

## Allium Specifications

| Spec | Scope |
|------|-------|
| `reporter/reporter.allium` | Change-event payloads, deduplication, routing identity |

## Testing

```bash
dda inv test --targets=./comp/anomalydetection/observer/...
dda inv test --targets=./comp/anomalydetection/severityevents/...
dda inv test --targets=./comp/anomalydetection/reporter/impl/
dda inv test --targets=./comp/anomalydetection/logssource/impl/
```

Benchmarks: `dda inv test --targets=./comp/anomalydetection/observer/impl/ -- -bench=.`

## Sub-Guides

| Path | Focus |
|------|-------|
| `observer/AGENTS.md` | Engine architecture, key files, design decisions, pitfalls |
