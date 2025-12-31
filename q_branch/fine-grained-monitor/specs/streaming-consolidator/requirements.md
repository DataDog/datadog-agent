# Streaming Parquet Consolidator

## User Story

As an engineer using the metrics viewer, I need metric selection to respond in
under 2 seconds so that I can quickly explore different metrics without waiting
30 seconds between each selection.

## Requirements

### REQ-CON-001: Reduce Query Latency Through File Consolidation

WHEN the viewer queries metrics data
THE SYSTEM SHALL read from consolidated files containing multiple rotation
periods rather than hundreds of individual small files

WHEN consolidation reduces file count from N files to M files (where M << N)
THE SYSTEM SHALL achieve proportional reduction in file I/O overhead during
metric queries

**Rationale:** Query latency is dominated by file I/O overhead. Opening 500
files takes ~20 seconds; opening 5 files should take ~200ms. Engineers need
fast metric exploration to effectively debug container behavior.

---

### REQ-CON-002: Memory-Efficient Streaming Consolidation

WHEN consolidating parquet files
THE SYSTEM SHALL stream data one row group at a time rather than loading all
source data into memory simultaneously

WHEN processing a row group
THE SYSTEM SHALL write the row group to the output file before reading the
next row group from any source file

**Rationale:** Data volumes can reach gigabytes. Loading all data into memory
would cause OOM errors or require excessive memory limits. Streaming keeps
memory usage bounded to approximately one row group size (typically 64-128MB).

---

### REQ-CON-003: Safe File Lifecycle Management

WHEN consolidation completes successfully
THE SYSTEM SHALL delete source files only after the consolidated file is fully
written and closed with a valid parquet footer

WHEN consolidation fails or is interrupted
THE SYSTEM SHALL leave source files intact and remove any partial output file

WHEN selecting files for consolidation
THE SYSTEM SHALL exclude files modified within the last 5 minutes to avoid
consolidating files still being written by the monitor

**Rationale:** Data integrity is paramount. Source files must remain readable
until consolidated data is durably stored. The 5-minute buffer ensures the
monitor has completed writing and closed the file with a valid footer.

---

### REQ-CON-004: Preserve Data Fidelity

WHEN consolidating parquet files
THE SYSTEM SHALL preserve all columns, data types, and values from source files
without modification

WHEN source files contain overlapping time ranges
THE SYSTEM SHALL include all data points without deduplication

**Rationale:** The consolidator is a storage optimization, not a data
transformation. All original data must be queryable from consolidated files.
Deduplication would require domain knowledge about metric semantics.

---

### REQ-CON-005: Continuous Background Operation

WHEN deployed alongside the fine-grained-monitor
THE SYSTEM SHALL run continuously, checking for consolidation opportunities at
a configurable interval (default: 5 minutes)

WHEN no files are eligible for consolidation
THE SYSTEM SHALL wait until the next check interval without consuming
significant resources

WHEN the consolidator process receives SIGTERM or SIGINT
THE SYSTEM SHALL complete any in-progress consolidation before exiting, or
abort cleanly leaving source files intact

**Rationale:** The consolidator runs as a sidecar container alongside the
monitor. It must operate autonomously without manual intervention and shut down
gracefully during pod termination.

---

### REQ-CON-006: Configurable Consolidation Policy

WHEN determining which files to consolidate
THE SYSTEM SHALL consolidate files within the same date partition
(dt=YYYY-MM-DD) to maintain partition boundaries

WHEN the number of eligible files in a partition exceeds a threshold
(default: 10 files)
THE SYSTEM SHALL trigger consolidation for that partition

WHEN consolidated file size would exceed a limit (default: 500MB)
THE SYSTEM SHALL create multiple consolidated files rather than one oversized
file

**Rationale:** Date partitioning enables time-based retention policies.
Consolidation thresholds balance query performance against consolidation
overhead. Size limits ensure files remain manageable for queries and transfers.
