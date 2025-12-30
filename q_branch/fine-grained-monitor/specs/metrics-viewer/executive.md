# Metrics Viewer - Executive Summary

## Requirements Summary

Engineers need to visually explore container metrics to diagnose issues like CPU
throttling, memory pressure, and IO bottlenecks. The viewer loads parquet files
containing fine-grained metrics and provides interactive charts with filtering,
searching, and zooming. The Periodicity Study detects periodic patterns that
often indicate throttling or resource contention, surfacing insights that would
require tedious manual scanning to discover. Studies are initiated per-container
to ensure focused analysis and avoid visual noise from multiple overlapping
periodic patterns.

## Technical Summary

Library module at `src/metrics_viewer/` provides reusable components. CLI binary
`fgm-viewer` wraps the library for standalone use. Axum backend serves REST API
for metrics discovery, container filtering, timeseries data, and study analysis.
Frontend uses uPlot for canvas-based rendering with smooth pan/zoom on large
datasets. Study trait abstraction enables extensible analysis; periodicity
detection uses sliding-window autocorrelation (60-sample windows, 50% overlap).

## Status Summary

| Requirement | Status | Notes |
|-------------|--------|-------|
| **REQ-MV-001:** View Metrics Timeseries | ✅ Complete | uPlot chart with empty state instructions |
| **REQ-MV-002:** Select Metrics to Display | ✅ Complete | `/api/metrics` endpoint, metric dropdown |
| **REQ-MV-003:** Filter Containers by Attributes | ⚠️ Deprecated | Removed - search (REQ-MV-004) sufficient |
| **REQ-MV-004:** Search and Select Containers | ✅ Complete | Search box with debounce, Top N buttons |
| **REQ-MV-005:** Zoom and Pan Through Time | ✅ Complete | Drag zoom, scroll wheel zoom, reset button |
| **REQ-MV-006:** Navigate with Range Overview | ✅ Complete | Second uPlot instance as overview |
| **REQ-MV-007:** Detect Periodic Patterns | ✅ Complete | Per-container study initiation via icon button |
| **REQ-MV-008:** Visualize Periodicity Patterns | ✅ Complete | Results panel with restore selection option |
| **REQ-MV-009:** Automatic Y-Axis Scaling | ✅ Complete | uPlot auto-ranges Y-axis to visible data |
| **REQ-MV-010:** Graceful Empty Data Display | ✅ Complete | Y-axis minimum range, no-data detection |

**Progress:** 10 of 10 complete

## Terminology Note

The feature previously called "Oscillation Study" was renamed to "Periodicity
Study" in December 2024. The term "periodicity" more accurately describes what
the autocorrelation algorithm detects: recurring patterns at regular intervals,
which may be discrete spikes (like cron jobs or GC cycles) rather than smooth
sinusoidal oscillations. If you encounter references to "oscillation detection"
in older documentation or discussions, they refer to this same feature.
