# Metrics Viewer - Executive Summary

## Requirements Summary

Engineers need to visually explore container metrics to diagnose issues like CPU
throttling, memory pressure, and IO bottlenecks. The viewer loads parquet files
containing fine-grained metrics and provides interactive charts with searching,
zooming, and analytical studies. The Periodicity Study detects periodic patterns
that often indicate throttling or resource contention. The Changepoint Study
identifies abrupt shifts in metric behavior, surfacing deployment impacts or
incident onset. Both studies surface insights that would require tedious manual
scanning to discover.

The viewer runs as a sidecar in the DaemonSet, accessible via kubectl
port-forward without copying files locally. Engineers see pod names instead of
opaque container IDs thanks to Kubernetes API integration. Fast startup (under 5
seconds) is achieved via an index file maintained by the collector, avoiding
expensive scans of thousands of parquet files.

## Technical Summary

Library module at `src/metrics_viewer/` provides reusable components. CLI binary
`fgm-viewer` wraps the library for standalone and in-cluster use. Axum backend
serves REST API for metrics discovery, container filtering, timeseries data, and
study analysis. Frontend uses uPlot for canvas-based rendering with smooth
pan/zoom on large datasets. Study trait abstraction enables extensible analysis;
periodicity detection uses sliding-window autocorrelation; changepoint detection
uses Bayesian Online Changepoint Detection via the `augurs` crate.

The collector maintains `index.json` with container metadata including pod names
from the Kubernetes API. The viewer loads this index instantly at startup and
derives metric names from parquet schema. Sidecar deployment ensures viewer and
collector operate independently with shared volume access.

## Status Summary

### Core Viewer

| Requirement | Status | Notes |
|-------------|--------|-------|
| **REQ-MV-001:** View Metrics Timeseries | ✅ Complete | uPlot chart with empty state instructions |
| **REQ-MV-002:** Select Metrics to Display | ✅ Complete | `/api/metrics` endpoint, metric dropdown |
| **REQ-MV-003:** Search and Select Containers | ✅ Complete | Search box with debounce; Top N deprecated |
| **REQ-MV-004:** Zoom and Pan Through Time | ✅ Complete | Drag zoom, scroll wheel zoom, reset button |
| **REQ-MV-005:** Navigate with Range Overview | ✅ Complete | Second uPlot instance as overview |
| **REQ-MV-006:** Detect Periodic Patterns | ✅ Complete | Per-container study initiation via icon button |
| **REQ-MV-007:** Visualize Periodicity Patterns | ✅ Complete | Results panel with restore selection option |
| **REQ-MV-008:** Automatic Y-Axis Scaling | ✅ Complete | uPlot auto-ranges Y-axis to visible data |
| **REQ-MV-009:** Graceful Empty Data Display | ✅ Complete | Y-axis minimum range, no-data detection |

### Cluster Deployment

| Requirement | Status | Notes |
|-------------|--------|-------|
| **REQ-MV-010:** Access Viewer From Cluster | ✅ Complete | Sidecar deployed, port-forward works |
| **REQ-MV-011:** View Node-Local Metrics | ✅ Complete | 204 metrics, 21 containers visible |
| **REQ-MV-012:** Fast Startup via Index | ✅ Complete | Startup in seconds, not 30+ minutes |
| **REQ-MV-013:** Viewer Operates Independently | ✅ Complete | Read-only volume, skips in-progress files |

### Metadata Display

| Requirement | Status | Notes |
|-------------|--------|-------|
| **REQ-MV-014:** Display Pod Names | ✅ Complete | Viewer shows pod names via `/api/containers` |
| **REQ-MV-015:** Enrich with Kubernetes Metadata | ✅ Complete | `kube-rs` client with in-cluster config |
| **REQ-MV-016:** Persist Metadata in Index | ✅ Complete | `index.json` schema v2 with pod_name, namespace |

### Studies

| Requirement | Status | Notes |
|-------------|--------|-------|
| **REQ-MV-017:** Detect Changepoints in Metrics | ✅ Complete | BOCPD via `augurs-changepoint` crate |
| **REQ-MV-018:** Visualize Changepoint Locations | ✅ Complete | Solid vertical lines with direction arrows |
| **REQ-MV-019:** Container List Sorted by Recency | ✅ Complete | Replaces Top N; 0ms via index.json |

### Multi-Panel Comparison

| Requirement | Status | Notes |
|-------------|--------|-------|
| **REQ-MV-020:** View Multiple Metrics Simultaneously | ✅ Complete | Up to 5 panels stacked vertically |
| **REQ-MV-021:** Panel Cards in Sidebar | ✅ Complete | Panel cards with metric + study inline |
| **REQ-MV-022:** Add Panels via Inline Autocomplete | ✅ Complete | Inline autocomplete for new panels |
| **REQ-MV-023:** Edit Panel Metric Inline | ✅ Complete | Click metric name for autocomplete |
| **REQ-MV-024:** Remove Panels via Sidebar | ✅ Complete | "×" button, min 1 panel enforced |
| **REQ-MV-025:** Synchronized Time Axis Across Panels | ✅ Complete | uPlot sync instance shared across panels |
| **REQ-MV-026:** Shared Container Selection Across Panels | ✅ Complete | Container list applies to all panels |
| **REQ-MV-027:** Panel-Specific Y-Axis Scaling | ✅ Complete | Each panel auto-scales independently |
| **REQ-MV-028:** Shared Range Overview in Multi-Panel Mode | ✅ Complete | Single overview below all panels |
| **REQ-MV-029:** Add Study to Panel | ✅ Complete | Per-panel study via inline autocomplete |
| **REQ-MV-030:** Study Visualization on Chart | ✅ Complete | Chart annotations with tooltips |
| **REQ-MV-031:** Studies Do Not Consume Panel Slots | ✅ Complete | Studies are overlays on existing panels |

### Dashboard System

| Requirement | Status | Notes |
|-------------|--------|-------|
| **REQ-MV-032:** Filter Containers by Labels | ✅ Complete | API: `labels=key:value` query param |
| **REQ-MV-033:** Load Dashboard Configuration | ✅ Complete | `?dashboard=url` or `?dashboard_inline=base64` |
| **REQ-MV-034:** Filter Containers via Dashboard | ✅ Complete | Apply label_selector, namespace from JSON |
| **REQ-MV-035:** Automatic Time Range from Containers | ✅ Complete | Compute from first_seen/last_seen |
| **REQ-MV-036:** Configure Panels from Dashboard | ✅ Complete | Create panels from JSON spec |

**Progress:** 36 of 36 complete

## Terminology Note

The feature previously called "Oscillation Study" was renamed to "Periodicity
Study" in December 2024. The term "periodicity" more accurately describes what
the autocorrelation algorithm detects: recurring patterns at regular intervals,
which may be discrete spikes (like cron jobs or GC cycles) rather than smooth
sinusoidal oscillations.
