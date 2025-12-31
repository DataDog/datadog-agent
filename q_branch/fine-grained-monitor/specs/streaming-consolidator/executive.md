# Streaming Parquet Consolidator - Executive Summary

## Requirements Summary

Engineers using the metrics viewer experience 20-30 second delays when
selecting metrics because the viewer must open 500+ small parquet files. The
streaming consolidator merges these files into fewer, larger files while the
monitor continues running. This reduces file I/O overhead and enables sub-2-second
metric selection. The consolidator uses a streaming approach to maintain bounded
memory usage (~80MB) regardless of data volume, and implements transactional
safety to ensure source files are never deleted until consolidated data is
durably written.

## Technical Summary

The consolidator runs as a sidecar container sharing the `/data` volume with
the monitor. Every 5 minutes, it scans for partitions with 10+ files older than
5 minutes. For each eligible partition, it streams data one row group at a time
from source files to a new consolidated file. Output files are written to a
`.tmp` suffix and atomically renamed on completion. Source files are deleted
only after successful rename. The consolidator uses the same parquet/arrow
crates as the viewer, ensuring format compatibility. It deploys as a separate
binary (`fgm-consolidator`) in the same Docker image.

## Status Summary

| Requirement | Status | Notes |
|-------------|--------|-------|
| **REQ-CON-001:** Reduce Query Latency | ⚠️ Pending Verification | Implementation complete, awaiting latency measurement |
| **REQ-CON-002:** Streaming Consolidation | ✅ Complete | Row-group streaming with bounded memory |
| **REQ-CON-003:** Safe File Lifecycle | ✅ Complete | Atomic rename via `.tmp` suffix |
| **REQ-CON-004:** Preserve Data Fidelity | ✅ Complete | Schema validation, all columns preserved |
| **REQ-CON-005:** Background Operation | ✅ Complete | Runs as sidecar, graceful shutdown |
| **REQ-CON-006:** Configurable Policy | ✅ Complete | CLI args for all thresholds |

**Progress:** 5 of 6 complete (REQ-CON-001 pending latency verification)
