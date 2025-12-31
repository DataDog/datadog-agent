# Streaming Parquet Consolidator - Design

## Architecture Overview

The consolidator runs as a separate binary (`fgm-consolidator`) deployed as a
sidecar container sharing the `/data` volume with `fine-grained-monitor`. It
periodically scans for small parquet files and merges them into larger
consolidated files using a streaming approach.

```
┌─────────────────────────────────────────────────────────────┐
│ Pod: fine-grained-monitor-xyz                               │
│                                                             │
│  ┌──────────────────────┐    ┌──────────────────────┐      │
│  │ fine-grained-monitor │    │ fgm-consolidator     │      │
│  │                      │    │                      │      │
│  │ Writes 90s files ────┼───►│ Reads old files      │      │
│  │                      │    │ Writes consolidated  │      │
│  └──────────────────────┘    └──────────────────────┘      │
│              │                         │                    │
│              └─────────┬───────────────┘                    │
│                        ▼                                    │
│                 /data (shared volume)                       │
│                 ├── dt=2025-12-30/                          │
│                 │   └── identifier=.../                     │
│                 │       ├── metrics-*.parquet (small)       │
│                 │       └── consolidated-*.parquet (large)  │
│                 └── index.json                              │
└─────────────────────────────────────────────────────────────┘
```

## REQ-CON-001: File Discovery and Selection

### Directory Scanning

The consolidator scans the data directory for parquet files organized in the
existing partition structure:

```
/data/dt={date}/identifier={pod}/metrics-{timestamp}.parquet
```

Files are discovered using glob pattern: `/data/dt=*/identifier=*/*.parquet`

### Eligibility Criteria

A file is eligible for consolidation when:
1. File name starts with `metrics-` (not already consolidated)
2. File modification time is older than 5 minutes (REQ-CON-003)
3. File is in a partition with 10+ eligible files (REQ-CON-006)

### Grouping Strategy

Files are grouped by partition key `(date, identifier)` to maintain partition
boundaries per REQ-CON-006. Each partition is consolidated independently.

## REQ-CON-002: Streaming Merge Algorithm

### Memory Model

The streaming approach maintains bounded memory usage:

```
Peak Memory = sizeof(RecordBatch) + sizeof(ParquetWriter buffers)
            ≈ row_group_size + compression_buffers
            ≈ 64MB + 16MB = ~80MB typical
```

### Merge Process

```rust
fn consolidate_partition(input_files: Vec<PathBuf>, output_path: PathBuf) {
    // 1. Create output writer (REQ-CON-002)
    let writer = ParquetWriter::new(&output_path, schema, compression)?;

    // 2. Stream through each input file
    for input_file in input_files {
        let reader = ParquetReader::open(&input_file)?;

        // 3. Process one row group at a time (REQ-CON-002)
        for row_group in reader.row_groups() {
            let batch = row_group.read_to_arrow()?;
            writer.write_batch(&batch)?;
            // batch is dropped here, freeing memory
        }
    }

    // 4. Close writer to finalize footer (REQ-CON-003)
    writer.close()?;
}
```

### Schema Handling

All input files share the same schema (produced by lading_capture). The
consolidator reads schema from the first input file and validates subsequent
files match. Schema mismatch causes consolidation to abort for that partition.

## REQ-CON-003: Transactional Safety

### Write-Ahead Pattern

To ensure atomicity, the consolidator uses a temporary file pattern:

```
1. Write to: consolidated-{timestamp}.parquet.tmp
2. Close file (writes footer)
3. Rename to: consolidated-{timestamp}.parquet (atomic on POSIX)
4. Delete source files
```

If any step fails, the `.tmp` file is deleted and source files remain intact.

### Interrupt Handling

On SIGTERM/SIGINT:
1. Set shutdown flag
2. Complete current file write (if in progress)
3. Perform rename or cleanup
4. Exit

The consolidator checks the shutdown flag between file operations, not
mid-write, ensuring partial row groups are never written.

### File Locking

No explicit locking is required because:
1. Monitor writes to `metrics-*.parquet` files
2. Consolidator writes to `consolidated-*.parquet` files
3. 5-minute age buffer ensures monitor has closed files before consolidation

## REQ-CON-004: Data Preservation

### Column Projection

The consolidator reads and writes all columns without filtering:

```rust
let projection = None; // Read all columns
let reader = ParquetRecordBatchReaderBuilder::try_new(file)?
    .with_projection(projection)  // No column filtering
    .build()?;
```

### No Deduplication

Data is written in encounter order. If source files contain duplicate
timestamps (from overlapping collection periods), both are preserved. This
maintains the principle that consolidation is lossless.

### Compression

Output files use the same compression as input (ZSTD level 3 by default). The
consolidator re-compresses data during the streaming process; zero-copy row
group transfer is not implemented in the initial version.

## REQ-CON-005: Operational Loop

### Main Loop Structure

```rust
async fn run(config: Config, shutdown: CancellationToken) {
    let mut interval = tokio::time::interval(config.check_interval);

    loop {
        tokio::select! {
            _ = interval.tick() => {
                if let Err(e) = consolidation_pass(&config).await {
                    tracing::error!(error = %e, "Consolidation pass failed");
                }
            }
            _ = shutdown.cancelled() => {
                tracing::info!("Shutdown requested, exiting");
                break;
            }
        }
    }
}
```

### Consolidation Pass

Each pass:
1. Scan directory for eligible files
2. Group by partition
3. For each partition exceeding threshold:
   - Sort files by timestamp
   - Consolidate into batches respecting size limit
   - Clean up source files on success

### Resource Usage

Between consolidation passes, the process is idle (sleeping in tokio select).
CPU and memory usage is near-zero when not actively consolidating.

## REQ-CON-006: Output File Naming and Sizing

### Naming Convention

Consolidated files use a distinct prefix to differentiate from source files:

```
consolidated-{start_timestamp}-{end_timestamp}.parquet
```

Where timestamps are from the first and last source files included.

### Size Limiting

When estimated output size exceeds the limit:

```rust
fn plan_consolidation(files: Vec<FileInfo>, max_size: u64) -> Vec<Vec<FileInfo>> {
    let mut batches = vec![];
    let mut current_batch = vec![];
    let mut current_size = 0u64;

    for file in files {
        if current_size + file.size > max_size && !current_batch.is_empty() {
            batches.push(std::mem::take(&mut current_batch));
            current_size = 0;
        }
        current_size += file.size;
        current_batch.push(file);
    }

    if !current_batch.is_empty() {
        batches.push(current_batch);
    }

    batches
}
```

This creates multiple output files when necessary, each under the size limit.

## CLI Interface

```
fgm-consolidator

OPTIONS:
    --data-dir <PATH>         Data directory [default: /data]
    --check-interval <SECS>   Seconds between consolidation checks [default: 300]
    --min-files <N>           Minimum files to trigger consolidation [default: 10]
    --max-output-size <MB>    Maximum consolidated file size [default: 500]
    --min-age <SECS>          Minimum file age before consolidation [default: 300]
    --compression-level <N>   ZSTD compression level [default: 3]
    --dry-run                 Show what would be consolidated without doing it
```

## Deployment

### Kubernetes Sidecar

```yaml
containers:
  - name: monitor
    image: fine-grained-monitor:latest
    # ... existing config ...

  - name: consolidator
    image: fine-grained-monitor:latest
    command: ["/usr/local/bin/fgm-consolidator"]
    args:
      - --data-dir=/data
      - --check-interval=300
      - --min-files=10
    volumeMounts:
      - name: data
        mountPath: /data
    resources:
      requests:
        memory: "128Mi"
        cpu: "100m"
      limits:
        memory: "256Mi"
        cpu: "500m"
```

### Shared Binary

The consolidator is built as part of the same Docker image, available as a
separate binary entry point. This simplifies deployment and ensures version
consistency between monitor and consolidator.
