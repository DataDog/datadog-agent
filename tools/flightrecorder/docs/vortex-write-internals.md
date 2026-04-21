# Vortex Write Internals — Memory Reference

Notes from reading vortex 0.61.0 source code relevant to flightrecorder
memory optimization.

## Write pipeline architecture

```
Caller → ArrayStream → Layout Strategy Pipeline → VortexWrite sink
                                                   ↓
                                              kanal(1) channel
                                                   ↓
                                              Segment Writer → Disk/Vec
```

The pipeline is **truly streaming** — there is no internal Vec<u8> that
buffers the entire file. The `kanal::bounded_async(1)` channel between the
layout task and the write task means at most 1 buffer is queued.

## Strategy stack (compact_strategy)

```
TableStrategy
├── validity: CollectStrategy → CompressingStrategy(concurrency=4) → Flat
└── data:     RepartitionStrategy(block_size=0, 8K rows, no target)
                → ZonedStrategy(block_size=8K, stats)
                    → DictStrategy(max_bytes=1MB, max_len=65535)
                        ├── codes: RepartitionStrategy(1MB min, 1MB target)
                        │            → CompressingStrategy(4)
                        │                → BufferedStrategy(2MB)
                        │                    → ChunkedLayout → Flat
                        ├── values: CompressingStrategy(4) → Flat
                        └── fallback: RepartitionStrategy(1MB min, 1MB target)
                                       → CompressingStrategy(4)
                                           → BufferedStrategy(2MB)
                                               → ChunkedLayout → Flat
```

### Per-column memory budget

For each of the 13 metric columns processed through compact_strategy:
- BufferedStrategy: 2 MB buffer
- DictStrategy: up to 1 MB dictionary values
- CompressingStrategy: up to 4 concurrent tasks buffered

**Worst case per column**: ~4 MB (2 MB buffer + 1 MB dict + 1 MB in-flight)
**13 columns**: ~52 MB in the pipeline alone

### fast_flush_strategy (current flush path)

```
TableStrategy
├── validity: Flat
└── data:     Flat
```

No compression, no buffering, no dictionary encoding. Each column is
serialized directly to the output. Memory cost: just the column data
itself.

## Key tuning parameters

| Parameter | Where | Default | Effect |
|-----------|-------|---------|--------|
| `CompressingStrategy::with_concurrency(N)` | `compressed.rs` | `available_parallelism()` | N tasks in parallel, each buffering a chunk |
| `BufferedStrategy::new(child, size)` | `buffered.rs` | 2 MB | Buffers chunks until 2× size, flushes 1× size |
| `RepartitionStrategy.block_size_minimum` | `repartition.rs` | 0 or 1 MB | Min bytes before flushing a block |
| `RepartitionStrategy.block_size_target` | `repartition.rs` | None or 1 MB | Target block size for coalescing |
| `RepartitionStrategy.block_len_multiple` | `repartition.rs` | 8192 | Row count granularity |
| `DictLayoutConstraints.max_bytes` | `dict/writer.rs` | 1 MB | Max dictionary size |
| `DictLayoutConstraints.max_len` | `dict/writer.rs` | u16::MAX | Max unique values (>255 → u16 codes) |
| `ZonedLayoutOptions.block_size` | `zoned/writer.rs` | 8192 | Stats block size |
| `ZonedLayoutOptions.concurrency` | `zoned/writer.rs` | `available_parallelism()` | Parallel stats computation |

## VortexWrite trait — where can we write to?

Implemented for:
- `Vec<u8>` — in-memory buffer (current flush path)
- `tokio::fs::File` — direct async file write (current merge path)
- `Cursor<T>` — cursor over mutable slice
- `AsyncWriteAdapter<W>` — wraps any `AsyncWrite`

**Recommendation**: Switch flush from `Vec<u8>` to `tokio::fs::File` to
avoid the 2-3 MB write_buf allocation. Already proven in merge.rs.

## Writer push API

`VortexWriteOptions::writer(write, dtype)` returns a `Writer` that
supports incremental writes:

```rust
let mut writer = options.writer(&mut file, dtype);
writer.push(chunk).await?;
writer.push(chunk).await?;
let summary = writer.finish().await?;
```

Uses `kanal::bounded_async(1)` internally — backpressure is automatic.
Could be used to avoid materializing the full 10K-row StructArray.

## CollectStrategy warning

`CollectStrategy` loads **the entire stream into memory** for the columns
it handles (validity). In compact_strategy, it's only used for validity
arrays (which are small for NonNullable data), but be aware if switching
to nullable columns.
