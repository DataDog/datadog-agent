# Flight Recorder Sidecar

A Rust sidecar that captures raw DogStatsD samples, logs, and trace stats
**before aggregation** and writes them to compressed Parquet files on local
disk. The recorder taps the agent's hook system pre-concentrator, so no
information is lost to 10-second bucketing or aggregation.

## Architecture

```
Agent process (Go)                        Sidecar process (Rust)
                                          +-----------------------+
DogStatsD UDP ─┐                          | Tokio async runtime   |
               ├─ Hook ─ FlatBuffers ──>  |  read_frame()         |
Log pipeline  ─┘    (Unix socket)         |  route by type        |
                                          |  send_frame() ~5-10ns |
Trace stats ───── Hook ─ FlatBuffers ──>  +----------|------------+
                                                     | rtrb SPSC ring
                                          +----------|------------+
                                          | std::thread per writer |
                                          |  decode FlatBuffers    |
                                          |  accumulate rows       |
                                          |  flush to Parquet      |
                                          +------------------------+
                                               |
                                          Parquet files + contexts.bin
```

### Data flow

1. **Go agent** subscribes to hooks in the DogStatsD, log, and trace-stats
   pipelines. Each hook callback encodes samples as FlatBuffers and sends
   them over a Unix socket to the sidecar.

2. **Sidecar async runtime** accepts connections, reads length-prefixed
   FlatBuffers frames, and routes each frame to the appropriate writer
   thread by peeking at the `SignalPayload` union discriminant.

3. **Dedicated writer threads** (one per signal type: metrics, logs,
   trace_stats) receive frames through a lock-free SPSC ring buffer
   ([rtrb](https://crates.io/crates/rtrb)). Each thread owns its writer
   exclusively - no mutex, no async, no Tokio involvement. The thread
   decodes the FlatBuffers, accumulates rows in columnar buffers, and
   flushes to Parquet when a row-count or time threshold is reached.

4. **Telemetry** counters (buffered rows, flush count/bytes/duration) are
   exported via lock-free atomics (`WriterTelemetry`) and read by a
   background DogStatsD reporter without any locking.

### Signal partitioning

Each agent binary connects to the sidecar with a single socket:

| Connection | Sends | Writer thread |
|---|---|---|
| Core agent | MetricBatch, LogBatch | `fr-metrics`, `fr-logs` |
| Trace agent | TraceStatsBatch | `fr-traces` |

Multiple connections **never write to the same signal pipeline**. This
guarantees the rtrb ring is SPSC (single producer, single consumer) and
eliminates all contention on the hot path.

## Design trade-offs

### Why dedicated threads instead of async?

The Parquet write path (`ArrowWriter::write`, file rotation, Snappy
compression) is synchronous `std::fs` I/O. Running this on the Tokio async
runtime blocks worker threads, starving `read_frame()` calls. This causes
the Unix socket kernel buffer to fill up, which blocks the Go agent's
`WriteTo()` call, which blocks the flush goroutine, which causes the
agent-side ring buffers to overflow and drop 98%+ of data.

Dedicated threads completely decouple async socket I/O from synchronous
disk I/O. The async handler just pushes a `Vec<u8>` into the rtrb ring
(~5-10ns) and immediately returns to read the next frame.

### Why rtrb instead of tokio::sync::mpsc?

The SPSC guarantee (one connection per signal type) means we don't need a
multi-producer channel. `rtrb` is a true lock-free ring buffer with ~5-10ns
push latency vs ~50-100ns for `tokio::sync::mpsc`. At 4K frames/sec the
difference is negligible, but rtrb is the right primitive for an SPSC hot
path.

The ring capacity is 512 slots. Each slot holds a `Vec<u8>` (24 bytes for
ptr/len/cap - the frame data itself is heap-allocated and moved, not
copied). With a 1-second write deadline on the Go agent, the producer
sends at most ~10 frames/sec - the ring is effectively never full.

### Why not reset the bloom filter on reconnect?

Context keys are deterministic hashes of `(metric_name, tags)`. The same
metric from a reconnecting agent produces the same key. Resetting the
sidecar's bloom filter on every new connection forces all contexts to be
re-sent, creating a thundering-herd burst of file I/O that overwhelms the
writer during the critical warm-up window.

Instead, the bloom filter persists across connections within the same
sidecar process. Duplicate contexts from reconnecting agents are silently
deduplicated. On sidecar **process** restart, the bloom filter starts empty
and agents naturally re-send all contexts (the Go agent creates a fresh
bloom filter on each `activate()`).

The `contexts.bin` file uses append mode so it survives sidecar restarts.
Duplicate entries are harmless - the hydrate tool deduplicates on read.

### Why context-key mode only (no inline mode)?

Inline mode stored full metric names and tags in every Parquet file, requiring
a `HashMap<u64, (String, String)>` that grew with cardinality and 9 extra
`StringInterner` columns per flush. Context-key mode stores only a u64 key
per row and writes context definitions once to `contexts.bin` via a bloom
filter. This reduces per-row cost from ~120 bytes to ~32 bytes and eliminates
the HashMap entirely.

## File layout

```
/data/signals/
  metrics-{timestamp_ms}.parquet   # context_key, value, timestamp_ns, sample_rate, source
  logs-{timestamp_ms}.parquet      # hostname, source, status, tags..., content, timestamp_ns
  trace_stats-{timestamp_ms}.parquet  # service, name, resource, ..., 18 columns
  contexts.bin                     # append-only: [context_key, name, tags] per unique metric
```

Files rotate every 60 seconds. Snappy compression with dictionary encoding.
A janitor thread enforces retention (default 3 hours) and a disk cap.

## Configuration

| Env var | Default | Description |
|---|---|---|
| `DD_FLIGHTRECORDER_SOCKET_PATH` | `/var/run/flightrecorder/pipeline.sock` | Unix socket path |
| `DD_FLIGHTRECORDER_OUTPUT_DIR` | `/data/signals` | Parquet output directory |
| `DD_FLIGHTRECORDER_FLUSH_ROWS` | `5000` | Row count threshold for Parquet flush |
| `DD_FLIGHTRECORDER_FLUSH_INTERVAL_SECS` | `15` | Time threshold for Parquet flush |
| `DD_FLIGHTRECORDER_RETENTION_HOURS` | `3` | Hours to retain signal files |
| `DD_FLIGHTRECORDER_MAX_DISK_MB` | `5120` | Disk cap in MB (0 = time-only) |
| `DD_FLIGHTRECORDER_STATSD_HOST` | `127.0.0.1` | DogStatsD host for telemetry |
| `DD_FLIGHTRECORDER_STATSD_PORT` | `8125` | DogStatsD port |

## Building & testing

```bash
cd tools/flightrecorder
cargo build --release
cargo test
cargo bench --bench writer_bench
```

## Go agent transport

The Go flightrecorder component (`comp/flightrecorder/impl/`) discovers the
sidecar socket with exponential backoff, subscribes to hooks, and sends
FlatBuffers frames through a ring-buffered batcher with double-buffer
swapping. A **1-second write deadline** on the Unix socket prevents the
flush goroutine from blocking indefinitely if the sidecar falls behind.
