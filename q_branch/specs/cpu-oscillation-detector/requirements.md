# Per-Container CPU Oscillation Detector

## User Story

As an SRE operating Kubernetes clusters with 20-100 containers per node across multiple teams' workloads, I need to be alerted when a specific container's CPU usage is oscillating rapidly so that I can quickly identify WHICH workload is misbehaving and navigate directly to that workload's logs/events to investigate restart loops, misconfigured autoscaling, or other issues.

## Personas

**SRE on K8s**: Operates clusters with 20-100 containers per node across multiple teams' workloads. Needs to quickly identify WHICH workload is misbehaving when CPU oscillation occurs. Primary triage path: identify workload via K8s tags (namespace, deployment, container_name) -> check its logs/events.

## User Journeys

### Primary Journey: Triage Oscillating Container
1. **Trigger**: Alert fires on `container.cpu.oscillation.detected == 1`
2. **Identify**: Read K8s tags (kube_namespace, kube_deployment, container_name) from metric
3. **Investigate**: Navigate to that workload's logs/events in Datadog
4. **Resolve**: Fix the root cause (restart loop, misconfiguration, etc.)

### Secondary Journey: Build Dashboard
1. **Trigger**: Want visibility into oscillation patterns across fleet
2. **Build**: Dashboard showing oscillation metrics by namespace/deployment
3. **Use**: Spot patterns (certain workloads always oscillate) for proactive fixes

## Requirements

### REQ-COD-001: Detect Periodic CPU Oscillation Per Container

WHEN a container's CPU usage exhibits periodic behavior (autocorrelation peak > 0.5 at some lag τ)
AND the period τ is within the detectable range (2-30 seconds)
AND the amplitude of CPU swings exceeds the configured minimum amplitude threshold
THE SYSTEM SHALL emit a binary signal indicating oscillation is detected for that container

WHEN a container's CPU usage shows no significant periodicity (autocorrelation peak < 0.5)
OR the detected period is outside the valid range
OR amplitude is below the minimum threshold
THE SYSTEM SHALL indicate no oscillation detected for that container

THE SYSTEM SHALL report the periodicity score (0.0-1.0) indicating strength of the periodic pattern

THE SYSTEM SHALL report the detected period in seconds (time between CPU peaks)

**Rationale:** Autocorrelation-based detection finds true periodic patterns rather than counting random direction changes. Random noise produces many direction reversals but low autocorrelation; true oscillations (batch processing, health check loops, retry storms) show strong autocorrelation peaks at the oscillation period. This eliminates false positives from containers with naturally variable but non-periodic CPU usage.

---

### REQ-COD-002: Establish Container-Specific Baseline

WHEN the detector has been tracking a container for less than 5 minutes
THE SYSTEM SHALL learn that container's normal CPU variance without flagging oscillations

WHEN new CPU samples arrive for a container after its warmup period
THE SYSTEM SHALL update that container's baseline variance using exponential decay (weighting recent samples more heavily)

WHEN a container is removed (via WorkloadMeta event)
THE SYSTEM SHALL immediately delete that container's detector state

WHEN a container is created (new container ID)
THE SYSTEM SHALL create a new detector with fresh warmup state

**Rationale:** Different containers have different workload characteristics. A CI runner legitimately varies more than a database sidecar. Users need detection tuned to each container's normal behavior, not arbitrary global thresholds.

---

### REQ-COD-003: Report Oscillation Characteristics with Container Tags

WHEN oscillation metrics are emitted for a container
THE SYSTEM SHALL include tags identifying the container using standard DD_CHECKS_TAG_CARDINALITY tagging (kube_namespace, kube_deployment, container_name, etc.)

WHEN metrics are emitted for a container
THE SYSTEM SHALL report the following oscillation characteristics:
- `detected` (0 or 1): Whether oscillation is currently detected
- `periodicity_score` (0.0-1.0): Strength of periodic pattern (peak autocorrelation)
- `period` (seconds): Detected oscillation period (time between peaks)
- `frequency` (Hz): Oscillation frequency (1/period)
- `amplitude` (CPU %): Peak-to-trough CPU swing
- `baseline_stddev` (CPU %): Container's baseline standard deviation

**Rationale:** Tags enable the critical triage path: alert fires -> read K8s tags -> navigate to that workload's logs/events. The periodicity_score allows flexible alerting thresholds without redeploying the Agent. Period/frequency help identify the source (e.g., 30s period suggests health check loops).

---

### REQ-COD-004: Minimal Performance Impact at Scale

THE SYSTEM SHALL sample CPU for each container at 1Hz with O(1) cost per sample

THE SYSTEM SHALL compute autocorrelation at emission time (every 15s) with O(n²) complexity where n=60 samples

THE SYSTEM SHALL maintain detection state using fixed memory per container (~500 bytes per container)

THE SYSTEM SHALL support 100 containers per host within a ~50KB memory budget for all detector state

THE SYSTEM SHALL complete autocorrelation computation for 100 containers in <100ms

THE SYSTEM SHALL NOT require batching or special handling for typical container counts (20-100)

**Rationale:** The detector must not itself become a resource problem. Autocorrelation is O(n²) but with n=60 and only computed every 15s, the CPU cost is negligible (~0.1ms per container). With 20-100 containers per host being typical, the implementation scales acceptably.

---

### REQ-COD-005: Configurable Detection with Default Disabled

WHERE oscillation detection is enabled via configuration
THE SYSTEM SHALL allow users to configure:
- `min_amplitude` (default: 10%): Minimum CPU swing to consider as oscillation
- `min_periodicity_score` (default: 0.5): Minimum autocorrelation peak to detect periodicity
- `min_period` (default: 2s): Minimum detectable oscillation period
- `max_period` (default: 30s): Maximum detectable oscillation period
- `warmup_seconds` (default: 60s): Time before detection is active

WHERE oscillation detection is disabled (default state)
THE SYSTEM SHALL not perform 1Hz sampling or oscillation analysis

THE SYSTEM SHALL default to disabled (explicit opt-in required)

**Rationale:** This is a new, experimental feature. Default-disabled ensures users explicitly opt-in. The min_periodicity_score threshold controls sensitivity (higher = fewer false positives but may miss weak oscillations). Period range limits focus detection on actionable oscillations (too fast = noise, too slow = normal variation).

---

### REQ-COD-006: Metric Emission for All Tracked Containers

WHEN metrics are emitted (every 15 seconds)
THE SYSTEM SHALL emit metrics for ALL running containers, regardless of oscillation state

WHEN a container has not completed warmup
THE SYSTEM SHALL emit `detected=0` for that container (baseline learning in progress)

**Rationale:** Consistent cardinality makes dashboarding and alerting easier. SREs can see all containers and their oscillation state without worrying about missing data points.

---

### REQ-COD-007: Graceful Error Handling

WHEN a cgroup read fails for a specific container
THE SYSTEM SHALL log a warning and skip that container for that sample interval

WHEN a cgroup read fails for a specific container
THE SYSTEM SHALL continue sampling other containers without interruption

WHEN a container is removed between samples
THE SYSTEM SHALL handle the removal gracefully via WorkloadMeta events

WHEN there are no running containers
THE SYSTEM SHALL run normally but emit no metrics (not an error condition)

**Rationale:** In dynamic K8s environments, containers come and go frequently. The detector must handle these transient states without crashing or producing misleading data.

---

## Decisions

1. **Per-container only (not host-level):** Host-level CPU oscillation detection is untriageable on modern K8s hosts running 5+ diverse workloads. Per-container detection provides the triage value by attributing oscillation to specific workloads.

2. **Aggregate CPU per container (not per-core):** Per-core detection is non-deterministic because work scheduling across cores is unpredictable within a container.

3. **Autocorrelation-based detection (not direction reversals):** Direction reversal counting produces excessive false positives because random noise also produces many reversals. Autocorrelation detects true periodicity: a signal correlated with a time-shifted version of itself indicates repeating patterns. This correctly identifies batch processing, health check loops, and retry storms while ignoring random noise.

4. **Short-lived containers (<60s warmup) will not trigger detection:** Containers that don't survive the warmup period never establish a baseline and thus cannot detect oscillation. Warmup reduced from 5 minutes to 60 seconds since autocorrelation doesn't require baseline variance estimation.

5. **Container restart = new detector:** A restarting container gets a new container ID, which means a fresh detector with new warmup. This is correct behavior since the container's workload characteristics may have changed.

6. **Use existing container metrics infrastructure:** Leverage WorkloadMeta for container discovery/lifecycle and the standard metrics provider for CPU stats, consistent with other container checks.

7. **Metric namespace: `container.cpu.oscillation.*`:** Clear separation from `system.*` host-level metrics indicates these are per-container metrics.

8. **Period range 2-30 seconds:** Oscillations faster than 2s are likely measurement noise; slower than 30s are normal workload variation. This range captures the actionable patterns: batch processing (5-10s), health checks (10-30s), retry loops (1-5s).

9. **Periodicity score threshold 0.5:** Based on signal processing standards, autocorrelation > 0.5 indicates moderate to strong periodicity. This can be tuned via configuration.

10. **Baseline persistence across restarts:** Out of scope for this iteration. Future work could leverage the Auditer/registry.json pattern for state persistence.

11. **Correlation with host-level metrics:** Out of scope. Users can aggregate per-container metrics at query time in Datadog if they need host-level views.
