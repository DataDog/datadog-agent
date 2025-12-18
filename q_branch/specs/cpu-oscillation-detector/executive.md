# Per-Container CPU Oscillation Detector - Executive Summary

## Requirements Summary

SREs operating Kubernetes clusters need early warning when specific containers exhibit rapid CPU oscillation - a symptom of restart loops, thrashing, retry storms, or misconfigured autoscaling. Host-level detection is untriageable on modern K8s hosts running 20-100 diverse workloads; when oscillation fires, operators cannot determine which container caused the problem.

Per-container detection solves this by attributing oscillation to specific workloads. When the detector flags oscillation, the metric includes K8s tags (namespace, deployment, container_name), enabling direct triage: alert fires -> read tags -> navigate to that workload's logs/events. Each container has a 60-second warmup period, after which the detector uses autocorrelation to identify true periodic patterns in CPU usage.

**Critical Path**: Alert -> Workload Identification. If tags don't correctly identify the workload, the entire feature value is lost.

## Technical Summary

Implemented as a long-running Datadog Agent check that samples CPU for each running container at 1Hz using the existing container metrics provider (cgroup-based). Each container gets its own `OscillationDetector` instance with a 60-sample ring buffer (~500 bytes per container). Container discovery and lifecycle management via WorkloadMeta subscription; detectors are created on container start and immediately deleted on container stop.

Oscillation is detected via **autocorrelation-based periodicity detection**. At each metric emission (every 15s), the detector computes autocorrelation of the CPU signal at various time lags (2-30 seconds). A peak in autocorrelation indicates the signal repeats with that period. This approach correctly distinguishes true periodic patterns (batch processing, health check loops, retry storms) from random noise that would produce false positives with simpler direction-reversal counting.

**Key detection parameters:**
- `periodicity_score`: Peak autocorrelation value (0.0-1.0), threshold default 0.5
- `period`: Detected oscillation period in seconds
- `amplitude`: Peak-to-trough CPU% swing, threshold default 10%

**Key architectural changes from original host-level design:**
- CPU source: Container metrics provider (cgroup) instead of gopsutil host CPU
- State: `map[containerID]*OscillationDetector` instead of single detector
- Discovery: WorkloadMeta subscription for container lifecycle
- Tagging: Full tagger integration for K8s/ECS context
- Namespace: `container.cpu.oscillation.*` instead of `system.cpu.oscillation.*`
- Algorithm: Autocorrelation-based periodicity detection instead of direction reversal counting

**Scale**: Supports 100 containers per host within ~50KB memory budget. Autocorrelation computation is O(n×k) but only runs every 15s, adding <10ms for 100 containers. Default disabled (explicit opt-in required).

## Status Summary

| Requirement | Status | Notes |
|-------------|--------|-------|
| **REQ-COD-001:** Detect Periodic CPU Oscillation Per Container | ✅ Complete | Autocorrelation algorithm implemented |
| **REQ-COD-002:** Establish Container-Specific Baseline | ✅ Complete | Per-container 60s warmup + lifecycle cleanup via WorkloadMeta |
| **REQ-COD-003:** Report Oscillation Characteristics with Tags | ✅ Complete | Emits periodicity_score, period, frequency, amplitude with full K8s tags |
| **REQ-COD-004:** Minimal Performance Impact at Scale | ✅ Complete | ~500 bytes/container, O(n×k) autocorrelation at emit time |
| **REQ-COD-005:** Configurable Detection (Default Disabled) | ✅ Complete | Config: min_periodicity_score, min/max_period with Nyquist constraints |
| **REQ-COD-006:** Metric Emission for All Containers | ✅ Complete | Emit for all containers regardless of detection state |
| **REQ-COD-007:** Graceful Error Handling | ✅ Complete | Skip and continue on cgroup read failures, WorkloadMeta event-driven lifecycle |

**Progress:** 7 of 7 requirements complete. Autocorrelation-based detection fully implemented.

## Implementation Details

**Implementation Location:**
- `pkg/collector/corechecks/containers/cpu_oscillation/`
  - `oscillation.go` - Main check implementation (long-running)
  - `detector.go` - OscillationDetector algorithm (ring buffer, autocorrelation)
  - `config.go` - Configuration parsing and validation
  - `stub.go` - Stub for non-Linux platforms
  - `detector_test.go` - Unit tests for detector algorithm
  - `oscillation_test.go` - Unit tests for check lifecycle

**Configuration:**
- `conf.d/cpu_oscillation.d/conf.yaml`

**Check Registration:**
- `pkg/commonchecks/corechecks.go` - Registered as `cpu_oscillation`

## Design Decisions Summary

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Scope | Per-container only | Host-level is untriageable on K8s; per-container provides triage value |
| Detection algorithm | Autocorrelation | Direction reversals produce false positives from random noise; autocorrelation detects true periodicity |
| Container scale | 20-100 per host | ~50KB memory budget, no batching needed |
| Short-lived containers | Accept no detection | Containers <60s never exit warmup; acceptable |
| Period range | 2-30 seconds | <2s is noise, >30s is normal variation; this range captures actionable patterns |
| Periodicity threshold | 0.5 default | Signal processing standard: >0.5 indicates moderate to strong periodicity |
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
| Short-lived container (<60s) | Never triggers detection (acceptable) |
| Low-variance container | Low amplitude = no detection (intentional) |
| Random noise (non-periodic) | Low periodicity_score = no detection (key improvement) |
