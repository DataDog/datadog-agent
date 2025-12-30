# In-Cluster Viewer - Executive Summary

## Requirements Summary

Engineers debugging container behavior need to access the metrics viewer without
manually copying parquet files from pods. The in-cluster viewer provides direct
access via kubectl port-forward, displaying node-local metrics in the familiar
interactive chart interface. Critical requirement: startup must complete within
5 seconds regardless of accumulated file count, achieved via a lightweight index
file that the collector maintains.

## Technical Summary

The viewer runs as a sidecar container in the existing DaemonSet, sharing a
volume with the collector. Both binaries ship in a single container image. The
collector maintains an `index.json` file with container metadata, updated only
when containers change. The viewer loads this index at startup (instant) and
derives metric names from parquet file schema. Data files are loaded on-demand
based on query time range using predictable file naming, avoiding expensive glob
operations over thousands of files.

## Status Summary

| Requirement | Status | Notes |
|-------------|--------|-------|
| **REQ-ICV-001:** Access Viewer From Cluster | ✅ Complete | Sidecar deployed, port-forward works |
| **REQ-ICV-002:** View Node-Local Metrics | ✅ Complete | 204 metrics, 21 containers visible |
| **REQ-ICV-003:** Fast Startup via Index | ✅ Complete | Startup in seconds, not 30+ minutes |
| **REQ-ICV-004:** Viewer Operates Independently | ✅ Complete | Read-only volume, skips in-progress files |

**Progress:** 4 of 4 complete

**Verified:** In-cluster viewer tested in gadget-dev Kind cluster. Index-based
startup successfully loads 204 metrics from parquet schema and 21 containers
from index.json within seconds.
