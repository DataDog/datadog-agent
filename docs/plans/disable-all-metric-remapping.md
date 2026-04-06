# Plan: Full Support for `exporter.datadogexporter.DisableAllMetricRemapping` in DDOT/DD Exporter

## Context

Upstream PR [opentelemetry-collector-contrib#45943](https://github.com/open-telemetry/opentelemetry-collector-contrib/pull/45943)
introduced the feature gate `exporter.datadogexporter.DisableAllMetricRemapping`
(registered as `featuregates.DisableMetricRemappingFeatureGate`). When enabled, this
gate disables **all** metric remapping:

- The `otel.` prefix added to `system.*`, `process.*`, and select Kafka metrics
- Runtime metric name mappings (e.g., `process.runtime.go.goroutines` → `runtime.go.num_goroutine`)
- Host/system metric remapping (e.g., `system.cpu.utilization` → `system.cpu.idle`)
- Container metric remapping (e.g., `container.cpu.usage.total` → `container.cpu.usage`)

The upstream PR note states: *"Note that this feature gate only works with the legacy
exporter. I intend to open a second PR to make it work on the new serializer exporter
path."* This plan implements that second PR in the agent repo, where the serializer
exporter is the active path for both DDOT and agent OTLP ingest.

---

## Architecture Overview

### Metric Translation Pipeline

```
OTel Metrics
    │
    ▼
metrics_translator.go (MapMetrics)
    ├── if withRemapping:    remapMetrics()    → creates Datadog-named copies
    ├── if withOTelPrefix:   renameMetrics()   → adds otel./ otelcol_ prefix to originals
    └── mapToDDFormat()                        → serializes to Datadog wire format
```

### Translator Config Flags (`pkg/opentelemetry-mapping-go/otlp/metrics/config.go`)

| Flag | Default | Set by |
|---|---|---|
| `withRemapping` | `false` | `WithRemapping()` — creates Datadog-named copies of container/system/Kafka metrics |
| `withOTelPrefix` | `false` | `WithOTelPrefix()` or `WithRemapping()` — prepends `otel.` to system/process/Kafka originals |
| `withRuntimeRemapping` | **`true`** | Defaults on; `WithoutRuntimeMetricMappings()` turns it off |

### Factory Constructors (`comp/otelcol/otlp/components/exporter/serializerexporter/factory.go`)

| Constructor | Ingestion Path | Default Options |
|---|---|---|
| `NewFactoryForAgent` | Agent OTLP ingest | `WithOTelPrefix()` |
| `NewFactoryForOTelAgent` | DDOT (embedded collector) | `WithOTelPrefix()` |
| `NewFactoryForOSSExporter` | OSS Collector | `WithRemapping()` (includes prefix) |

All three share a common switch statement:

```go
switch {
case featuregates.DisableMetricRemappingFeatureGate.IsEnabled():
    options = append(options, otlpmetrics.WithoutRuntimeMetricMappings())
case featuregates.MetricRemappingDisabledFeatureGate.IsEnabled():
    // old gate — no action needed (no prefix, no remapping)
default:
    options = append(options, otlpmetrics.WithOTelPrefix())  // or WithRemapping() for OSS
}
```

### Feature Gates

| Gate ID | Variable | Stage | Behavior when enabled |
|---|---|---|---|
| `exporter.datadogexporter.DisableAllMetricRemapping` | `featuregates.DisableMetricRemappingFeatureGate` | Alpha | Disable ALL remapping (new gate) |
| `exporter.datadogexporter.metricremappingdisabled` | `featuregates.MetricRemappingDisabledFeatureGate` | Alpha | Disable prefix only, keep runtime mapping (old gate, deprecated) |

Both gates are registered globally in
`github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/featuregates`.

---

## Current State and Gaps

### What is already implemented

The switch statement in `factory.go` is present for all three paths. When
`DisableMetricRemappingFeatureGate` is enabled:
- `WithoutRuntimeMetricMappings()` sets `withRuntimeRemapping = false`
- `withOTelPrefix` stays `false` (default) → no `otel.` prefix
- `withRemapping` stays `false` (default) → no remapping copies

The behavior is **semantically correct today**, but relies implicitly on two defaults
being `false`. If those defaults ever change, the gate silently breaks.

### Existing tests

| Test | Gate tested | Paths covered | Metrics tested |
|---|---|---|---|
| `TestMetricRemapping` | New + old | DDOT (`NewFactoryForOTelAgent`) only | Runtime metrics only |
| `TestMetricPrefix` | Old gate only | DDOT only | system/process/kafka prefix |

### Gaps

1. **`WithoutRuntimeMetricMappings()` has a wrong doc comment**: says *"enables mapping
   of runtime metrics"* — it actually disables it.
2. **No explicit option** that captures the full intent of "disable all three flags".
   The gate-enabled branch relies on two implicit defaults.
3. **`TestMetricPrefix` never tests the new gate** — only the old
   `MetricRemappingDisabledFeatureGate`.
4. **`TestMetricRemapping` only exercises runtime metrics**, not system/host metrics
   that are controlled by `withOTelPrefix`.
5. **No tests at all for `NewFactoryForOSSExporter`** with the new gate. The OSS path
   is especially important to test because its default (`WithRemapping()`) enables both
   `withRemapping` AND `withOTelPrefix`, making the gate's effect more visible (it
   suppresses both Datadog-remapped copies AND the `otel.` prefix on originals).

---

## Implementation Stages

### Stage 1 — Translator layer: add `WithNoRemapping()` + fix doc comment

**Files:**
- `pkg/opentelemetry-mapping-go/otlp/metrics/config.go`
- `pkg/opentelemetry-mapping-go/otlp/metrics/metrics_translator_test.go`

**Changes:**

1. Fix the doc comment on `WithoutRuntimeMetricMappings()`:
   ```go
   // WithoutRuntimeMetricMappings disables mapping of runtime metrics to their
   // Datadog counterparts (e.g. process.runtime.go.goroutines → runtime.go.num_goroutine).
   ```

2. Add a new `WithNoRemapping()` option that explicitly sets all three flags:
   ```go
   // WithNoRemapping disables all metric remapping: no otel. prefix, no
   // container/system/Kafka remapping, and no runtime metric name mapping.
   // Use this when the DisableAllMetricRemapping feature gate is enabled.
   func WithNoRemapping() TranslatorOption {
       return func(t *translatorConfig) error {
           t.withRemapping = false
           t.withOTelPrefix = false
           t.withRuntimeRemapping = false
           return nil
       }
   }
   ```

3. Add a unit test verifying that `WithNoRemapping()` produces pass-through metrics
   (no prefix, no copies, no runtime mappings) even when other options are combined with it.

**Rationale:** Pure library-level, zero runtime behavior change. Reviewable in isolation.

---

### Stage 2 — Factory: use `WithNoRemapping()` when the gate is enabled

**File:** `comp/otelcol/otlp/components/exporter/serializerexporter/factory.go`

In both `newFactoryForAgentWithType` (lines 101–103) and `NewFactoryForOSSExporter`
(lines 147–149), change the new-gate branch:

```go
// Before
case featuregates.DisableMetricRemappingFeatureGate.IsEnabled():
    options = append(options, otlpmetrics.WithoutRuntimeMetricMappings())

// After
case featuregates.DisableMetricRemappingFeatureGate.IsEnabled():
    options = append(options, otlpmetrics.WithNoRemapping())
```

Semantically identical to today's behavior but explicit and robust to future default changes.

**Rationale:** Consumer of Stage 1. Small, easy to review in isolation.

---

### Stage 3 — Tests: comprehensive coverage across all factory paths

**File:** `comp/otelcol/otlp/components/exporter/serializerexporter/exporter_test.go`

#### 3a. Extend `TestMetricPrefix` to cover the new gate

Add calls that toggle `DisableMetricRemappingFeatureGate` (alongside the existing old-gate calls):

```go
// new gate: system/process/kafka metrics must NOT get otel. prefix
testMetricPrefixWithNewGate(t, true, "system.memory.usage", "system.memory.usage")
testMetricPrefixWithNewGate(t, true, "process.cpu.utilization", "process.cpu.utilization")
testMetricPrefixWithNewGate(t, true, "kafka.producer.request-rate", "kafka.producer.request-rate")
// new gate disabled: prefix still applied
testMetricPrefixWithNewGate(t, false, "system.memory.usage", "otel.system.memory.usage")
```

#### 3b. Extend `TestMetricRemapping` with system/host metric cases

Add `system.cpu.utilization` to `createTestMetricsWithRuntimeMetrics()` (or a new
helper). For the DDOT path with `newGate=false`:

- Expect `otel.system.cpu.utilization` (prefix applied)
- Expect NO `system.cpu.idle` (DDOT doesn't enable `withRemapping`)

For the DDOT path with `newGate=true`:
- Expect `system.cpu.utilization` (no prefix)
- Expect NO `system.cpu.idle`

#### 3c. Add `TestMetricRemappingOSS` for `NewFactoryForOSSExporter`

Mirrors `TestMetricRemapping` but for the OSS path. Include both runtime and system metrics.
Test the four gate combinations:

| `newGate` | `oldGate` | Expected behavior |
|---|---|---|
| false | false | `otel.system.cpu.utilization` + remapped copies (`system.cpu.idle` etc.), runtime mappings present |
| true | false | `system.cpu.utilization` only, no copies, no runtime mappings |
| false | true | No `otel.` prefix, remapped copies still present, runtime mappings still present |
| true | true | Same as `(true, false)` — new gate takes precedence |

**Rationale:** Non-functional tests; fast to review. Serve as the formal specification
of expected behavior for all three ingestion paths.

---

### Stage 4 — Release note

**File:** `releasenotes/notes/disable-all-metric-remapping-<branch-slug>.yaml`

Document that `exporter.datadogexporter.DisableAllMetricRemapping` is now fully
supported in all three serializer exporter paths (DDOT, agent OTLP ingest, OSS
collector), with a brief explanation of what remapping is disabled.

---

## Dependency Graph

```
Stage 1 (WithNoRemapping + doc fix in pkg/)
  └── Stage 2 (factory adopts WithNoRemapping)
        └── Stage 3 (tests for all paths, including OSS)

Stage 4 (release note) — independent, lands any time after Stage 2
```

Stages 1 and 2 can be combined into a single PR if the review overhead of splitting is
not worthwhile.

---

## Verification

### Manual (after Stage 2)

Run a local DDOT with and without the gate:

```bash
# Without gate: system.memory.usage → otel.system.memory.usage
./bin/agent/agent run -c bin/agent/dist/datadog.yaml

# With gate: system.memory.usage → system.memory.usage (no prefix)
./bin/agent/agent run -c bin/agent/dist/datadog.yaml \
  --feature-gates=exporter.datadogexporter.DisableAllMetricRemapping
```

### Automated (after Stage 3)

```bash
dda inv test --targets=./comp/otelcol/otlp/components/exporter/serializerexporter/...
dda inv test --targets=./pkg/opentelemetry-mapping-go/otlp/metrics/...
```
