# In-Cluster Viewer - Design

## Architecture Overview

The in-cluster viewer runs as a sidecar container alongside the existing
fine-grained-monitor collector in the DaemonSet. Both containers share a
volume for parquet file access.

```
┌─────────────────────────────────────────────────────────────┐
│               DaemonSet Pod (per node)                       │
│                                                              │
│  ┌─────────────┐           ┌─────────────┐                  │
│  │  monitor    │           │   viewer    │                  │
│  │  container  │           │  container  │                  │
│  │             │           │  port 8050  │                  │
│  └──────┬──────┘           └──────┬──────┘                  │
│         │ write                   │ read                    │
│         └───────────┬─────────────┘                         │
│                     ▼                                       │
│            ┌───────────────┐                                │
│            │  /data volume │                                │
│            │  *.parquet    │                                │
│            └───────────────┘                                │
└─────────────────────────────────────────────────────────────┘
```

## REQ-ICV-001: Cluster Access Implementation

### Container Configuration

The viewer runs as a second container in the DaemonSet pod:

- **Image:** Same `fine-grained-monitor` image (contains both binaries)
- **Command:** `/usr/local/bin/fgm-viewer`
- **Arguments:** `/data --port=8050 --no-browser`
- **Port:** 8050 (TCP)

### Access Method

Users access via kubectl port-forward:

```bash
kubectl port-forward ds/fine-grained-monitor 8050:8050
```

This connects to an arbitrary pod in the DaemonSet. For specific node access:

```bash
kubectl port-forward pod/fine-grained-monitor-<hash> 8050:8050
```

### Image Changes

The Dockerfile copies both binaries to the final image:

```dockerfile
COPY --from=builder /build/target/release/fine-grained-monitor /usr/local/bin/
COPY --from=builder /build/target/release/fgm-viewer /usr/local/bin/
```

## REQ-ICV-002: Node-Local Data Access

### Shared Volume

Both containers mount the same volume:

| Container | Mount | Access |
|-----------|-------|--------|
| monitor | /data | read-write |
| viewer | /data | read-only |

### Volume Type

Use `hostPath` volume pointing to `/var/lib/fine-grained-monitor`:

- Persists across pod restarts for post-mortem analysis
- Node-local storage (no cross-node access)
- Already configured in existing DaemonSet

### Directory Input Support

The `fgm-viewer` binary accepts a directory path and globs for `*.parquet`:

```rust
// In fgm-viewer.rs
if path.is_dir() {
    let pattern = format!("{}/**/*.parquet", path.display());
    let files: Vec<PathBuf> = glob(&pattern)?.filter_map(Result::ok).collect();
}
```

## REQ-ICV-003: Fast Startup via Index

### Problem

Scanning all parquet files at startup to build metadata (metrics list, container
info) is O(n) with file count. With 90-second rotation and days of accumulated
data, file count reaches thousands, causing 30+ minute startup times.

### Solution: Separate Index File

The collector maintains a lightweight `index.json` that the viewer loads
instantly. Data files are loaded on-demand based on query time range.

```
/data/
  index.json                              # Metadata (~10-50 KB)
  dt=2025-12-30/
    identifier=pod-xyz/
      metrics-20251230T160000Z.parquet    # Pure timeseries data
      metrics-20251230T160130Z.parquet
```

### Index File Schema

```json
{
  "schema_version": 1,
  "updated_at": "2025-12-30T16:05:00Z",

  "containers": {
    "abc123def456": {
      "full_id": "abc123def456789abcdef...",
      "pod_name": "coredns-5dd5756b68-xyz",
      "namespace": "kube-system",
      "qos_class": "Burstable",
      "first_seen": "2025-12-28T10:00:00Z",
      "last_seen": "2025-12-30T16:05:00Z"
    }
  },

  "data_range": {
    "earliest": "2025-12-22T00:00:00Z",
    "latest": "2025-12-30T16:05:00Z",
    "rotation_interval_sec": 90
  }
}
```

**Note:** Metric names are derived from parquet file schema, not stored in index.
This avoids hard-coding and ensures the metric list expands naturally as new
metrics are added to the collector.

### Collector Index Management

```
On startup:
  - Load existing index.json or create empty
  - Track known_containers: HashSet<ContainerId>

On each collection cycle (every 1s):
  - current_containers = containers observed this cycle

  If current_containers != known_containers:
    - New containers: Add to index with first_seen = now
    - Gone containers: Update last_seen timestamp
    - Write index atomically (write .tmp, rename)
    - known_containers = current_containers

On rotation (every 90s):
  - Write parquet file with predictable name: metrics-{ISO8601}Z.parquet
  - Update index.data_range.latest
  - Update last_seen for all active containers
```

Container churn is infrequent (minutes/hours), so index writes are rare.

### Viewer Startup Sequence

```
1. Attempt to load /data/index.json
2. If index exists:
   - Load container metadata from index
   - Read metric names from schema of most recent parquet file
   - Start server immediately
3. If index missing:
   - Poll for index.json every 5 seconds
   - Timeout after 3 minutes with error message
   - If parquet files exist but no index, rebuild index from files (fallback)
4. Serve UI on port 8050
```

### Time-Range Based File Discovery

Instead of globbing all files, the viewer computes file paths from time range:

```rust
fn find_files_for_range(data_dir: &Path, start: DateTime, end: DateTime) -> Vec<PathBuf> {
    // Predictable naming: /data/dt={date}/identifier={id}/metrics-{timestamp}Z.parquet
    // Compute expected file timestamps based on rotation interval (90s)
    // Return paths that fall within [start, end]
}
```

This avoids expensive glob operations over thousands of files.

### Atomic Index Writes

```rust
fn write_index(path: &Path, index: &Index) -> Result<()> {
    let tmp_path = path.with_extension("json.tmp");
    let json = serde_json::to_string_pretty(index)?;
    std::fs::write(&tmp_path, json)?;
    std::fs::rename(&tmp_path, path)?;  // Atomic on POSIX
    Ok(())
}
```

### Edge Case: No Data Files

When viewer starts with no index and no parquet files:

1. Display "Waiting for metrics data..." page
2. Poll every 5 seconds for either index.json or parquet files
3. After 3 minutes, display timeout error with troubleshooting guidance
4. If parquet files appear before index, rebuild index from files

### Currently-Writing Files

The collector's in-progress parquet file (not yet rotated) is excluded from
queries. Users see data with 0-90 second lag, which is acceptable for debugging
use cases.

## REQ-ICV-004: Container Independence

### Sidecar Pattern

Using Kubernetes sidecar pattern ensures:

- Containers share pod lifecycle but run independently
- Shared volumes enable data exchange
- Resource limits apply per-container
- Restart policies apply per-container

### Resource Allocation

| Container | Memory Request | Memory Limit | CPU Request | CPU Limit |
|-----------|---------------|--------------|-------------|-----------|
| monitor | 64Mi | 256Mi | 100m | 500m |
| viewer | 32Mi | 128Mi | 10m | 100m |

### Failure Isolation

- Monitor crash: Viewer continues serving existing data
- Viewer crash: Monitor continues collecting (Kubernetes restarts viewer)
- Both share termination grace period for clean shutdown

## File Changes Summary

| File | Change |
|------|--------|
| `Dockerfile` | Add `fgm-viewer` binary to image |
| `deploy/daemonset.yaml` | Add viewer container, expose port 8050 |
| `src/main.rs` | Add index management to collector |
| `src/index.rs` (new) | Index data structures and I/O |
| `src/bin/fgm-viewer.rs` | Load index instead of scanning files |
| `src/metrics_viewer/lazy_data.rs` | Query-time file discovery, load from index |
| `src/metrics_viewer/server.rs` | Read metrics from parquet schema |
