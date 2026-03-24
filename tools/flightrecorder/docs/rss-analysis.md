# Flight Recorder RSS Analysis

## Bench configuration

- Scenario: `dogstatsd-p95` (50K contexts, ~8K DSD samples/s)
- Duration: 600s + 30s warmup
- Commit: `5f382d0ad5` (decomposed tags + fast flush + flush/merge separation)

## RSS timeline (sampled every 30s)

```
+   0s:    0.0 MB   (startup)
+  30s:   49.1 MB   (warmup, contexts filling in)
+  60s:   70.1 MB   (steady state begins)
+  90s:   71.9 MB
+ 120s:   73.6 MB
+ 150s:   71.7 MB
+ 180s:   74.2 MB
+ 210s:   75.7 MB
+ 240s:   76.8 MB
+ 270s:   73.8 MB
+ 300s:   71.4 MB   ← steady state: ~72 MB
+ 330s:  368.4 MB   ← MERGE SPIKE (+297 MB!)
+ 360s:  287.0 MB   ← still elevated (jemalloc hasn't returned pages)
+ 390s:  244.2 MB
+ 420s:  224.6 MB
+ 450s:  212.8 MB
+ 480s:  199.5 MB
+ 510s:  188.0 MB
+ 540s:  191.7 MB
+ 570s:  183.0 MB
+ 600s:  177.8 MB   ← still 2.5x baseline, never recovered
+ 630s:  317.8 MB   ← second merge spike
```

## Key observations

### 1. Steady-state RSS is excellent: ~72 MB

From +60s to +300s (before the first merge), RSS is stable at 70-77 MB.
This proves the decomposed-tag + fast-flush approach works well for the
flush path. The 72 MB baseline includes:

- Context map: ~33 MB (50K entries × 7 reserved strings + name + overflow)
- Buffered columns + interners: ~2 MB (10K rows × 9 interners)
- FlatBuffers frame buffer: ~1 MB
- jemalloc metadata + binary: ~10 MB
- File-backed mappings: ~11 MB
- Misc: ~15 MB

### 2. Merge spike is the sole problem: +297 MB at +330s

The first merge triggers at ~300s (merge_interval_secs=300). At that point,
~240 flush files (~2.6 MB each, ~624 MB total) have accumulated. The merge
reads all 240 files and writes them through `compact_strategy()` which runs:

- `CompressingStrategy.with_concurrency(4)`: 4 parallel compression tasks
- `BufferedStrategy(2 MB)`: 2 MB buffering per column
- `DictStrategy`: dictionary encoding with up to 1 MB dictionaries
- `ZonedStrategy`: per-block statistics computation

With 13 metric columns × 2 MB buffers + 4 concurrent compressions, the
Vortex pipeline alone accounts for ~40 MB. But the 368 MB spike suggests
the issue is larger — likely the streaming merge is reading chunks faster
than the compression pipeline can write them, causing backpressure buffering.

### 3. jemalloc never returns pages: RSS stays at 177-200 MB

After the merge spike at +330s, RSS drops gradually but never returns to
the 72 MB baseline. This is classic jemalloc behavior — `dirty_decay_ms`
defaults to 10s but large allocations may not be returned to the OS for
much longer. The RSS at +600s is still 177 MB, 2.5x the steady state.

### 4. Second merge at +630s spikes again to 317 MB

The second merge at +630s triggers another spike, but slightly lower (317
vs 368 MB) because fewer flush files accumulated (5 min worth vs 5 min).

## Where the memory goes during merge

| Component | Estimated size | Notes |
|-----------|---------------|-------|
| Vortex BufferedStrategy | 13 cols × 2 MB = 26 MB | Per-column buffering in compact_strategy |
| Vortex CompressingStrategy | 4 × ~10 MB = 40 MB | 4 concurrent compression tasks |
| Vortex DictStrategy | 13 × 1 MB = 13 MB | Dictionary values per column |
| Input stream buffering | ~20-50 MB | Chunks read from flush files |
| Output file buffering | ~5 MB | Compressed output waiting for disk |
| **Total Vortex pipeline** | **~100-130 MB** | |
| jemalloc fragmentation | ~100-170 MB | Retained dirty pages |
| **Total observed spike** | **~297 MB above baseline** | |

## Recommendations

See `rss-optimization-proposals.md` for concrete proposals ranked by impact.
