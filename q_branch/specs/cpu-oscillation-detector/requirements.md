# CPU Oscillation Detector

## User Story

As an SRE, I need to be alerted when a host's CPU usage is oscillating rapidly so that I can identify thrashing systems, restart loops, or misconfigured autoscaling before they cause cascading failures.

## Requirements

### REQ-COD-001: Detect Rapid CPU Cycling

WHEN CPU usage alternates direction (rising then falling, or falling then rising) more than 6 times within a 60-second window
AND the amplitude of these swings exceeds 4x the host's baseline standard deviation
AND the amplitude exceeds the configured minimum amplitude threshold (if set)
THE SYSTEM SHALL emit a binary signal indicating oscillation is detected

WHEN CPU usage shows fewer direction changes OR swing amplitude is within normal variance
THE SYSTEM SHALL indicate no oscillation detected

**Rationale:** Rapid CPU oscillation often indicates unhealthy system states (restart loops, thrashing, retry storms) that are invisible in standard 15-60 second aggregated metrics. SREs need early warning of these patterns to intervene before systems become unresponsive.

---

### REQ-COD-002: Establish Host-Specific Baseline

WHEN the detector has been running for less than 5 minutes on a host
THE SYSTEM SHALL learn the host's normal CPU variance without flagging oscillations

WHEN new CPU samples arrive after the warmup period
THE SYSTEM SHALL update the baseline variance using exponential decay (weighting recent samples more heavily)

**Rationale:** Different hosts have different workload characteristics. A CI runner legitimately varies more than a database server. Users need detection tuned to each host's normal behavior, not arbitrary global thresholds.

---

### REQ-COD-003: Report Oscillation Characteristics

WHEN oscillation is detected
THE SYSTEM SHALL report the current swing amplitude (peak-to-trough percentage)

WHEN oscillation is detected
THE SYSTEM SHALL report the detected cycle frequency

**Rationale:** Knowing the severity and speed of oscillation helps SREs prioritize response. A 50% amplitude oscillation every 5 seconds is more urgent than a 10% swing every 30 seconds.

---

### REQ-COD-004: Minimal Performance Impact

THE SYSTEM SHALL sample CPU at 1Hz without adding more than 1% CPU overhead to the Agent process

THE SYSTEM SHALL maintain detection state using fixed memory (no unbounded growth)

**Rationale:** The detector must not itself become a resource problem. Users trust the Agent to be lightweight; detection that causes performance issues defeats the purpose.

---

### REQ-COD-005: Configurable Detection Sensitivity

WHERE oscillation detection is enabled
THE SYSTEM SHALL allow users to configure the amplitude multiplier threshold (default: 4.0x baseline)

WHERE oscillation detection is enabled
THE SYSTEM SHALL allow users to configure a minimum amplitude threshold in absolute percentage points (default: 0, meaning no floor)

WHERE oscillation detection is disabled
THE SYSTEM SHALL not perform 1Hz sampling or oscillation analysis

**Rationale:** Different environments have different tolerance for false positives vs. missed detections. The amplitude multiplier provides relative sensitivity (compared to baseline), while the minimum amplitude threshold provides an absolute floor—useful for filtering out low-CPU hosts where proportionally-large-but-absolutely-small swings aren't actionable. Users need the ability to tune sensitivity or disable the feature entirely if it doesn't suit their workload.

---

## Decisions

1. **Aggregate CPU only (not per-core):** Per-core detection is non-deterministic because work scheduling across cores is unpredictable. Future consideration: detecting "quiet cores" (work pinned to single core when others are idle) - out of scope for this iteration.

2. **High-variance workloads (batch, CI):** Unknown how to handle. This implementation is explicitly a data-gathering exercise to understand what oscillation patterns look like in the wild. We'll tune based on staging observations.

3. **Baseline persistence across restarts:** Out of scope for this iteration. Future work could leverage the Auditer/registry.json pattern for state persistence.
