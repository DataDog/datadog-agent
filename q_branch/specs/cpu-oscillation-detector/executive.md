# CPU Oscillation Detector - Executive Summary

## Requirements Summary

SREs need early warning when hosts exhibit rapid CPU oscillation - a symptom of restart loops, thrashing, retry storms, or misconfigured autoscaling. These patterns are visible at 1Hz sampling but invisible in standard 15-60 second aggregated metrics. The detector learns each host's normal CPU variance during a 5-minute warmup, then flags when oscillation amplitude significantly exceeds baseline. Operators get a binary signal (`detected=1`) plus severity metrics (amplitude, frequency) to prioritize investigation before systems become unresponsive.

## Technical Summary

Implemented as a long-running Datadog Agent check that samples aggregate CPU at 1Hz using gopsutil. A 60-sample ring buffer (480 bytes) holds the sliding detection window. Oscillation is detected via zero-crossing count (direction changes) combined with amplitude threshold relative to exponentially-decayed baseline variance. Metrics emitted every 15 seconds after the window fills. No metrics during first 60 seconds; `detected=1` only possible after 5-minute warmup. Fixed memory footprint with no allocations in hot path.

## Status Summary

| Requirement | Status | Notes |
|-------------|--------|-------|
| **REQ-COD-001:** Detect Rapid CPU Cycling | ❌ Not Started | Zero-crossing + amplitude detection |
| **REQ-COD-002:** Establish Host-Specific Baseline | ❌ Not Started | Exponential decay baseline tracker |
| **REQ-COD-003:** Report Oscillation Characteristics | ❌ Not Started | amplitude, frequency, zero_crossings gauges |
| **REQ-COD-004:** Minimal Performance Impact | ❌ Not Started | 1Hz gopsutil sampling, fixed memory |
| **REQ-COD-005:** Configurable Detection Sensitivity | ❌ Not Started | amplitude_multiplier, warmup_seconds config |

**Progress:** 0 of 5 complete
