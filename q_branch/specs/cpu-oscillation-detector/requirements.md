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

### REQ-COD-001: Detect Rapid CPU Cycling Per Container

WHEN a container's CPU usage alternates direction (rising then falling, or falling then rising) more than 6 times within a 60-second window
AND the amplitude of these swings exceeds 4x the container's baseline standard deviation
AND the amplitude exceeds the configured minimum amplitude threshold (if set)
THE SYSTEM SHALL emit a binary signal indicating oscillation is detected for that container

WHEN a container's CPU usage shows fewer direction changes OR swing amplitude is within normal variance
THE SYSTEM SHALL indicate no oscillation detected for that container

**Rationale:** Rapid CPU oscillation often indicates unhealthy container states (restart loops, thrashing, retry storms) that are invisible in standard 15-60 second aggregated metrics. Per-container detection enables SREs to immediately identify which workload is causing problems on hosts running dozens of containers.

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

WHEN oscillation is detected for a container
THE SYSTEM SHALL report the current swing amplitude (peak-to-trough percentage) for that container

WHEN oscillation is detected for a container
THE SYSTEM SHALL report the detected cycle frequency for that container

**Rationale:** Tags enable the critical triage path: alert fires -> read K8s tags -> navigate to that workload's logs/events. Without proper tagging, per-container detection has no value over host-level detection.

---

### REQ-COD-004: Minimal Performance Impact at Scale

THE SYSTEM SHALL sample CPU for each container at 1Hz without adding more than 1% CPU overhead to the Agent process

THE SYSTEM SHALL maintain detection state using fixed memory per container (~500 bytes per container)

THE SYSTEM SHALL support 100 containers per host within a ~50KB memory budget for all detector state

THE SYSTEM SHALL NOT require batching or special handling for typical container counts (20-100)

**Rationale:** The detector must not itself become a resource problem. With 20-100 containers per host being typical, the implementation must scale linearly with predictable memory usage.

---

### REQ-COD-005: Configurable Detection with Default Disabled

WHERE oscillation detection is enabled via configuration
THE SYSTEM SHALL allow users to configure the amplitude multiplier threshold (default: 4.0x baseline)

WHERE oscillation detection is enabled
THE SYSTEM SHALL allow users to configure a minimum amplitude threshold in absolute percentage points (default: 0, meaning no floor)

WHERE oscillation detection is disabled (default state)
THE SYSTEM SHALL not perform 1Hz sampling or oscillation analysis

THE SYSTEM SHALL default to disabled (explicit opt-in required)

**Rationale:** This is a new, experimental feature. Default-disabled ensures users explicitly opt-in. Different environments have different tolerance for false positives vs. missed detections. The amplitude multiplier provides relative sensitivity (compared to baseline), while the minimum amplitude threshold provides an absolute floor.

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

3. **Short-lived containers (<5min warmup) will not trigger detection:** Containers that don't survive the warmup period never establish a baseline and thus cannot detect oscillation. This is acceptable since short-lived containers don't oscillate meaningfully (they either succeed or fail quickly).

4. **Container restart = new detector:** A restarting container gets a new container ID, which means a fresh detector with new warmup. This is correct behavior since the container's workload characteristics may have changed.

5. **Use existing container metrics infrastructure:** Leverage WorkloadMeta for container discovery/lifecycle and the standard metrics provider for CPU stats, consistent with other container checks.

6. **Metric namespace: `container.cpu.oscillation.*`:** Clear separation from `system.*` host-level metrics indicates these are per-container metrics.

7. **High-variance workloads (batch, CI):** Unknown how to handle. This implementation is explicitly a data-gathering exercise to understand what oscillation patterns look like in the wild. We'll tune based on staging observations.

8. **Baseline persistence across restarts:** Out of scope for this iteration. Future work could leverage the Auditer/registry.json pattern for state persistence.

9. **Correlation with host-level metrics:** Out of scope. Users can aggregate per-container metrics at query time in Datadog if they need host-level views.
