# Per-Container CPU Oscillation Detector - Executive Summary

## Requirements Summary

SREs operating Kubernetes clusters need early warning when specific containers exhibit rapid CPU oscillation - a symptom of restart loops, thrashing, retry storms, or misconfigured autoscaling. Host-level detection is untriageable on modern K8s hosts running 20-100 diverse workloads; when oscillation fires, operators cannot determine which container caused the problem.

Per-container detection solves this by attributing oscillation to specific workloads. When the detector flags oscillation, the metric includes K8s tags (namespace, deployment, container_name), enabling direct triage: alert fires -> read tags -> navigate to that workload's logs/events. Each container learns its own baseline CPU variance during a 5-minute warmup, then flags when oscillation amplitude significantly exceeds that baseline.

**Critical Path**: Alert -> Workload Identification. If tags don't correctly identify the workload, the entire feature value is lost.

## Technical Summary

Implemented as a long-running Datadog Agent check that samples CPU for each running container at 1Hz using the existing container metrics provider (cgroup-based). Each container gets its own `OscillationDetector` instance with a 60-sample ring buffer (~500 bytes per container). Container discovery and lifecycle management via WorkloadMeta subscription; detectors are created on container start and immediately deleted on container stop.

Oscillation is detected via zero-crossing count (direction changes) combined with amplitude threshold relative to exponentially-decayed baseline variance. Metrics emitted every 15 seconds for ALL containers with proper tags via tagger component (respecting DD_CHECKS_TAG_CARDINALITY).

**Key architectural changes from original host-level design:**
- CPU source: Container metrics provider (cgroup) instead of gopsutil host CPU
- State: `map[containerID]*OscillationDetector` instead of single detector
- Discovery: WorkloadMeta subscription for container lifecycle
- Tagging: Full tagger integration for K8s/ECS context
- Namespace: `container.cpu.oscillation.*` instead of `system.cpu.oscillation.*`

**Scale**: Supports 100 containers per host within ~50KB memory budget. Default disabled (explicit opt-in required).

## Status Summary

| Requirement | Status | Notes |
|-------------|--------|-------|
| **REQ-COD-001:** Detect Rapid CPU Cycling Per Container | Complete | Zero-crossing + amplitude detection per container in `detector.go:Analyze()` |
| **REQ-COD-002:** Establish Container-Specific Baseline | Complete | Per-container exponential decay + lifecycle cleanup via WorkloadMeta |
| **REQ-COD-003:** Report Oscillation Characteristics with Tags | Complete | Tagger integration for K8s/ECS tags in `oscillation.go:emitMetrics()` |
| **REQ-COD-004:** Minimal Performance Impact at Scale | Complete | ~500 bytes/container ring buffer, RWMutex for concurrent access |
| **REQ-COD-005:** Configurable Detection (Default Disabled) | Complete | `config.go` with enabled, amplitude_multiplier, min_amplitude, warmup_seconds |
| **REQ-COD-006:** Metric Emission for All Containers | Complete | Emit for all containers regardless of detection state, detected=0 during warmup |
| **REQ-COD-007:** Graceful Error Handling | Complete | Skip and continue on cgroup read failures, WorkloadMeta event-driven lifecycle |

**Progress:** 7 of 7 requirements complete

## Implementation Details

**New Implementation Location:**
- `pkg/collector/corechecks/containers/cpu_oscillation/`
  - `oscillation.go` - Main check implementation (long-running)
  - `detector.go` - OscillationDetector algorithm (ring buffer, zero-crossing, baseline)
  - `config.go` - Configuration parsing and validation
  - `stub.go` - Stub for non-Linux platforms
  - `detector_test.go` - Unit tests for detector algorithm
  - `oscillation_test.go` - Unit tests for check configuration

**Configuration:**
- `cmd/agent/dist/conf.d/container_cpu_oscillation.d/conf.yaml.example`

**Check Registration:**
- `pkg/commonchecks/corechecks.go` - Registered as `container_cpu_oscillation`

**Old Host-Level Implementation (Deprecated):**
- `pkg/collector/corechecks/system/cpu/oscillation/` - Still present, marked deprecated

## Design Decisions Summary

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Scope | Per-container only | Host-level is untriageable on K8s; per-container provides triage value |
| Container scale | 20-100 per host | ~50KB memory budget, no batching needed |
| Short-lived containers | Accept no detection | Containers <5min never exit warmup; acceptable |
| Tagging | DD_CHECKS_TAG_CARDINALITY | Consistent with other container metrics |
| Container removal | Immediate state cleanup | Delete detector on WorkloadMeta event |
| Metric emission | All containers, always | Consistent cardinality for dashboarding |
| Metric namespace | `container.cpu.oscillation.*` | Clear separation from system.* host metrics |
| Error handling | Skip and continue | Log warning, continue with other containers |
| Default state | Disabled | Explicit opt-in via config |

## Edge Cases

| Edge Case | Handling |
|-----------|----------|
| Cgroup read failure | Skip container, log warning, continue |
| Container removed mid-sample | Graceful via WorkloadMeta events |
| Container restart | New container ID = new detector (fresh warmup) |
| No containers | Check runs, emits no metrics (not error) |
| Short-lived container (<5min) | Never triggers detection (acceptable) |
