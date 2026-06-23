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
| `agent` | internal agent telemetry namespace for `datadog.*` metrics |
| `logs` | `logssource/impl/logssource.go` |
| `agent_logs` | `observer/impl/observer.go` (pkg/util/log tap) |

**Log ingestion split:**
- **Container + kubelet logs** → `logssource` component
- **Agent internal logs** → `observer` taps `pkg/util/log` directly via `agent_logs`

Both paths share filtering primitives from `internal/logsfilter/`.

## Reporter Model

Reporters register through the `anomalydetection_reporters` Fx group
(`reporter/def`). The observer subscribes each injected `Reporter` after each
advance cycle.

- **StdoutReporter** — always active in `reporter/fx`; logs correlations at
  info on first-seen, debug for ongoing
- **EventReporter** — created when `anomaly_detection.reporting.enabled=true`
  AND the event-platform forwarder is available; publishes change events via
  `reporter/impl/notify.go`

See `reporter/reporter.allium` for the payload contract.

## Configuration

Keys are registered in `pkg/config/setup/common_settings.go`.

| Key | Default | Purpose |
|-----|---------|---------|
| `anomaly_detection.enabled` | `false` | Master analysis gate |
| `anomaly_detection.metrics.enabled` | `true` | External metric ingestion at handles |
| `anomaly_detection.reporting.enabled` | `false` | Event reporter (change events) |
| `anomaly_detection.recording.enabled` | `false` | Parquet recording middleware |
| `anomaly_detection.logs.enabled` | `true` | Parent gate for all log sources |
| `anomaly_detection.logs.containers.enabled` | `true` | Workloadmeta container logs |
| `anomaly_detection.logs.kubelet.enabled` | `true` | Kubelet journald source |
| `anomaly_detection.logs.internal.enabled` | `true` | Agent-internal log tap |
| `anomaly_detection.detectors.<name>.enabled` | varies | Per detector/correlator/extractor |
| `anomaly_detection.storage.max_series` | `50000` | Storage series cap |
| `anomaly_detection.storage.point_retention_secs` | `120` | Per-series point retention |

Per-source log rate limits and min severity live under
`anomaly_detection.logs.{internal,kubelet,containers}.*`.

## Allium Specifications

| Spec | Scope |
|------|-------|
| `reporter/reporter.allium` | Change-event payloads, deduplication, routing identity |

## Testing

```bash
dda inv test --targets=./comp/anomalydetection/observer/...
dda inv test --targets=./comp/anomalydetection/reporter/impl/
dda inv test --targets=./comp/anomalydetection/logssource/impl/
```

Benchmarks: `dda inv test --targets=./comp/anomalydetection/observer/impl/ -- -bench=.`

## Sub-Guides

| Path | Focus |
|------|-------|
| `observer/AGENTS.md` | Engine architecture, key files, design decisions, pitfalls |
