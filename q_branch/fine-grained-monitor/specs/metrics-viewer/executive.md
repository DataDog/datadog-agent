# Metrics Viewer - Executive Summary

## Requirements Summary

Engineers need to visually explore container metrics to diagnose issues like CPU
throttling, memory pressure, and IO bottlenecks. The viewer loads parquet files
containing fine-grained metrics and provides interactive charts with searching,
zooming, and analytical studies. The Periodicity Study detects periodic patterns
that often indicate throttling or resource contention, surfacing insights that
would require tedious manual scanning to discover.

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
periodicity detection uses sliding-window autocorrelation.

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
| **REQ-MV-003:** Search and Select Containers | ✅ Complete | Search box with debounce, Top N buttons |
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

**Progress:** 16 of 16 complete

## Terminology Note

The feature previously called "Oscillation Study" was renamed to "Periodicity
Study" in December 2024. The term "periodicity" more accurately describes what
the autocorrelation algorithm detects: recurring patterns at regular intervals,
which may be discrete spikes (like cron jobs or GC cycles) rather than smooth
sinusoidal oscillations.
