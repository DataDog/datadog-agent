# Metrics Viewer - Executive Summary

## Requirements Summary

Engineers need to visually explore container metrics to diagnose issues like CPU
throttling, memory pressure, and IO bottlenecks. The viewer loads parquet files
containing fine-grained metrics and provides interactive charts with filtering,
searching, and zooming. The Oscillation Study detects periodic patterns that
often indicate throttling or resource contention, surfacing insights that would
require tedious manual scanning to discover.

## Technical Summary

Library module at `src/metrics_viewer/` provides reusable components. CLI binary
`fgm-viewer` wraps the library for standalone use. Axum backend serves REST API
for metrics discovery, container filtering, timeseries data, and study analysis.
Frontend uses uPlot for canvas-based rendering with smooth pan/zoom on large
datasets. Study trait abstraction enables extensible analysis; oscillation
detection uses sliding-window autocorrelation (60-sample windows, 50% overlap).

## Status Summary

| Requirement | Status | Notes |
|-------------|--------|-------|
| **REQ-MV-001:** View Metrics Timeseries | ✅ Complete | uPlot chart with empty state instructions |
| **REQ-MV-002:** Select Metrics to Display | ✅ Complete | `/api/metrics` endpoint, metric dropdown |
| **REQ-MV-003:** Filter Containers by Attributes | ✅ Complete | QoS class and namespace filter dropdowns |
| **REQ-MV-004:** Search and Select Containers | ✅ Complete | Search box with debounce, Top N buttons |
| **REQ-MV-005:** Zoom and Pan Through Time | ✅ Complete | Drag zoom, scroll wheel zoom, reset button |
| **REQ-MV-006:** Navigate with Range Overview | ✅ Complete | Second uPlot instance as overview |
| **REQ-MV-007:** Detect Periodic Oscillations | ✅ Complete | Study trait with oscillation implementation |
| **REQ-MV-008:** Visualize Oscillation Patterns | ✅ Complete | Period markers, region shading via uPlot hooks |
| **REQ-MV-009:** Rescale Y-Axis to Visible Data | ✅ Complete | Rescale Y button with `setScale()` |

**Progress:** 9 of 9 complete
