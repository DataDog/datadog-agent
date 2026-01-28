name: smp-analyze
description: Analyze Single Machine Performance (SMP) quality gate results using Datadog MCP

# SMP Quality Gate Analysis

Examines Single Machine Performance (SMP) quality gate results to understand memory usage, identify top consumers, and find optimization targets.

## Quick Reference

### Available Experiments

| Experiment | Description | Memory Threshold |
|------------|-------------|------------------|
| `quality_gate_idle` | Base agent, no features | 175 MiB |
| `quality_gate_idle_all_features` | All features enabled, idle | 485 MiB |
| `quality_gate_logs` | Log collection workload | 220 MiB |
| `quality_gate_metrics_logs` | Metrics + logs workload | 475 MiB |

### Key Metrics

| Metric | Description |
|--------|-------------|
| `single_machine_performance.regression_detector.capture.total_pss_bytes` | Total PSS memory |
| `single_machine_performance.regression_detector.capture.smaps.pss.by_pathname` | PSS by binary/library |
| `single_machine_performance.regression_detector.capture.smaps_rollup.pss_anon` | Anonymous memory |
| `single_machine_performance.regression_detector.capture.smaps_rollup.pss_file` | File-backed memory |

### Key Tags

| Tag | Example Values |
|-----|----------------|
| `experiment` | `quality_gate_idle_all_features` |
| `job_id` | `f869e51d-f579-4761-a79c-d09a3382728f` |
| `target` | `datadog-agent` |
| `pathname` | `/opt/datadog-agent/bin/agent/agent` |
| `replicate` | `0` through `999` |

---

## Phase 1: Get Latest Results

### Find recent quality gate runs

```
Use mcp__datadog-mcp__get_datadog_metric with:
- queries: ["avg:single_machine_performance.regression_detector.capture.total_pss_bytes{experiment:quality_gate_idle_all_features} by {job_id}"]
- from: "now-24h"
- to: "now"
```

### Get total memory for specific job

```
Use mcp__datadog-mcp__get_datadog_metric with:
- queries: ["avg:single_machine_performance.regression_detector.capture.total_pss_bytes{job_id:<JOB_ID>,experiment:<EXPERIMENT>}"]
- from: "now-24h"
- to: "now"
```

---

## Phase 2: Memory Breakdown by Component

### Top memory consumers by pathname

```
Use mcp__datadog-mcp__get_datadog_metric with:
- queries: ["top(avg:single_machine_performance.regression_detector.capture.smaps.pss.by_pathname{job_id:<JOB_ID>,experiment:<EXPERIMENT>} by {pathname}, 20, 'mean', 'desc')"]
- from: "now-24h"
- to: "now"
```

### Expected top consumers for idle_all_features

| Component | Typical PSS | Notes |
|-----------|-------------|-------|
| `/opt/datadog-agent/bin/agent/agent` | ~78 MiB | Core agent |
| `/opt/datadog-agent/embedded/bin/otel-agent` | ~56 MiB | OTLP collector |
| `/opt/datadog-agent/embedded/bin/system-probe` | ~51 MiB | eBPF/network |
| `N/A` (anonymous) | ~47 MiB | Heap, stacks |
| `/opt/datadog-agent/embedded/bin/process-agent` | ~30 MiB | Process collection |
| `/opt/datadog-agent/embedded/bin/security-agent` | ~29 MiB | CWS/CSPM |
| `/opt/datadog-agent/embedded/bin/trace-agent` | ~21 MiB | APM |
| `anon_inode:bpf-map` | ~10 MiB | eBPF maps |

---

## Phase 3: Compare Passing vs Failing

### Get memory across replicates

```
Use mcp__datadog-mcp__get_datadog_metric with:
- queries: ["max:single_machine_performance.regression_detector.capture.total_pss_bytes{job_id:<JOB_ID>,experiment:<EXPERIMENT>}"]
- from: "now-24h"
- to: "now"
```

The max across replicates shows the worst case that may have failed the threshold.

---

## Phase 4: Analyze Anonymous Memory

Anonymous memory (`N/A` pathname) is Go heap + stacks + mmap.

### Get anonymous memory breakdown

```
Use mcp__datadog-mcp__get_datadog_metric with:
- queries: [
    "avg:single_machine_performance.regression_detector.capture.smaps_rollup.pss_anon{job_id:<JOB_ID>}",
    "avg:single_machine_performance.regression_detector.capture.smaps_rollup.pss_file{job_id:<JOB_ID>}",
    "avg:single_machine_performance.regression_detector.capture.smaps_rollup.pss_shmem{job_id:<JOB_ID>}"
  ]
- from: "now-24h"
- to: "now"
```

---

## Phase 5: Historical Comparison

### Compare memory over time

```
Use mcp__datadog-mcp__get_datadog_metric with:
- queries: ["avg:single_machine_performance.regression_detector.capture.total_pss_bytes{experiment:quality_gate_idle_all_features}"]
- from: "now-7d"
- to: "now"
```

Look for:
- Step increases (new feature/dependency added)
- Gradual growth (memory leak pattern)
- Variance (unstable initialization)

---

## Experiment Configuration

Configuration files are in `test/regression/cases/<experiment>/`:

```
test/regression/cases/quality_gate_idle_all_features/
├── datadog-agent/
│   ├── datadog.yaml      # Agent config (features enabled)
│   └── system-probe.yaml # System probe config
├── experiment.yaml       # Thresholds and checks
└── lading/              # Load generator config
```

### Features enabled in idle_all_features

From `datadog.yaml`:
- `logs_enabled: true`
- `apm_config.enabled: true`
- `process_config.process_collection.enabled: true`
- `runtime_security_config.enabled: true` (CWS)
- `compliance_config.enabled: true` (CSPM)
- `sbom.enabled: true`
- `otlp_config.{metrics,traces,logs}.enabled: true`
- `network_path.connections_monitoring.enabled: true`

From `system-probe.yaml`:
- `network_config.enabled: true`
- `service_monitoring_config.enabled: true`
- `dynamic_instrumentation.enabled: true`
- `discovery.enabled: true`
- `ping.enabled: true`
- `traceroute.enabled: true`

---

## Common Analysis Patterns

### "Why did quality gate fail?"

1. Get the job_id from CI failure message
2. Query total PSS: Is it above threshold?
3. Query by pathname: Which component grew?
4. Compare to previous passing run: What changed?

### "Where should we optimize?"

1. Query top 20 pathnames by PSS
2. Identify largest Go binaries (agent, system-probe, etc.)
3. For anonymous memory: Look at heap profiles
4. For file-backed: Look at shared libraries

### "Is this a regression or gradual growth?"

1. Query total PSS over 7-30 days
2. Look for step function vs linear growth
3. Correlate with git commits/releases

---

## Usage

```
/smp-analyze
```

Or invoke specific analysis:
```
/smp-analyze job_id:f869e51d-f579-4761-a79c-d09a3382728f
```
