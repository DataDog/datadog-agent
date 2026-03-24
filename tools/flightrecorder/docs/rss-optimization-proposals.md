# RSS Optimization Proposals

Ranked by expected impact. All estimates are for the dogstatsd-p95 scenario
(50K contexts, ~8K samples/s, 10K rows per flush).

---

## P0 — Merge memory spike (target: eliminate 297 MB spike)

### Proposal 1: Cap flush files per merge pass

**Problem**: The first merge at +300s processes ~240 flush files at once,
feeding ~624 MB of uncompressed data through the Vortex compression pipeline.

**Fix**: Add `max_files_per_pass: usize` back to MergeConfig. Cap at 20-30
files per pass (~50-80 MB input). The janitor runs every 30s, so multiple
small merges will process the backlog incrementally.

**Expected savings**: Spike drops from 368 MB → ~120-150 MB (pipeline
processes 80 MB input instead of 624 MB).

**Effort**: Small — re-add the field, slice candidates.

### Proposal 2: Use a lighter merge strategy

**Problem**: `compact_strategy()` uses `with_concurrency(4)` for compression,
2 MB BufferedStrategy buffers, DictStrategy with 1 MB dictionaries, and
ZonedStrategy for stats — all of which consume memory proportional to
column count (13 columns).

**Fix**: Create a `merge_strategy()` that uses:
- `CompressingStrategy.with_concurrency(1)` (not 4)
- `BufferedStrategy(512 KB)` (not 2 MB)
- No DictStrategy (flush files already have DictArrays; merge just re-compresses)
- No ZonedStrategy (stats are less useful for flight recorder data)

```rust
pub fn merge_strategy() -> Arc<dyn LayoutStrategy> {
    let flat = Arc::new(FlatLayoutStrategy::default());
    let chunked = ChunkedLayoutStrategy::new(flat.clone());
    let buffered = BufferedStrategy::new(chunked, 512 * 1024);
    let compressing = CompressingStrategy::new_btrblocks(buffered, true)
        .with_concurrency(1);
    let compress_flat = CompressingStrategy::new_btrblocks(flat, false)
        .with_concurrency(1);
    let validity = CollectStrategy::new(compress_flat);
    let table = TableStrategy::new(Arc::new(validity), Arc::new(compressing));
    Arc::new(table)
}
```

**Expected savings**: ~60-80 MB less pipeline overhead (1 concurrent task
instead of 4, smaller buffers). Combined with Proposal 1, spike would be
~80-100 MB.

**Effort**: Medium — new strategy function, verify output is still readable.

### Proposal 3: Force jemalloc to return pages after merge

**Problem**: After the merge spike, jemalloc holds dirty pages for 10s+
but never fully returns them. RSS stays at 177 MB instead of dropping
back to 72 MB.

**Fix**: After each merge pass, call `jemalloc_sys::mallctl("arena.N.purge")`
or set `dirty_decay_ms:0,muzzy_decay_ms:0` in `_RJEM_MALLOC_CONF` for the
merge phase. Alternatively, use `malloc_trim(0)` equivalent.

```rust
// After merge completes:
unsafe {
    tikv_jemalloc_sys::mallctl(
        b"arena.0.purge\0".as_ptr() as *const _,
        std::ptr::null_mut(), std::ptr::null_mut(),
        std::ptr::null(), 0
    );
}
```

**Expected savings**: RSS drops from 177 MB back to ~80 MB within seconds
after merge completes. Doesn't reduce peak but dramatically improves mean.

**Effort**: Small — one function call after merge_pass returns.

---

## P1 — Context map memory (target: reduce 33 MB → 15 MB)

### Proposal 4: Intern context map strings with a shared pool

**Problem**: `context_map: HashMap<u64, (String, DecomposedTags<7>)>` stores
50K entries, each with 9 owned Strings (name + 7 reserved + overflow).
At ~472 bytes/entry + HashMap overhead = ~33 MB.

**Fix**: Store `u32` codes into a shared string pool instead of owned Strings.

```rust
struct ContextPool {
    strings: Vec<String>,
    dedup: HashMap<String, u32>,
}

struct ContextEntry {
    name_code: u32,
    reserved_codes: [u32; 7],
    overflow_code: u32,
}

// context_map: HashMap<u64, ContextEntry>
// ContextEntry: 4 + 28 + 4 = 36 bytes (vs 472 bytes)
```

With 50K contexts but only ~5K unique tag values across all contexts, the
pool would hold ~5K strings (~200 KB) and each context entry shrinks to
36 bytes → 50K × 36 = 1.8 MB + HashMap overhead = ~5 MB.

**Expected savings**: 33 MB → 5 MB = **28 MB saved** (baseline RSS drops
from 72 MB to 44 MB).

**Effort**: Medium-large — new ContextPool struct, modify push() to look up
codes during flush.

### Proposal 5: Expire stale context map entries

**Problem**: Context map grows to 50K entries and stays there forever.
Some contexts may become stale if the agent stops sending them.

**Fix**: Track last-used timestamp per entry. On each merge pass, evict
entries not seen in the last N flushes.

**Expected savings**: Depends on churn. If 20% of contexts go stale,
saves ~6 MB. Less impactful than Proposal 4.

**Effort**: Small — add `last_seen: u32` (flush counter) to entries.

---

## P2 — Flush path memory (target: reduce 2-3 MB transient)

### Proposal 6: Write flush directly to file instead of Vec<u8>

**Problem**: Flush path buffers entire Vortex output in `write_buf: Vec<u8>`
(~2-3 MB for 10K rows), then writes to disk. The merge path already writes
directly to `tokio::fs::File`.

**Fix**: Use the same pattern as merge — write directly to a temp file,
then rename. Eliminates the write_buf entirely.

```rust
let mut file = tokio::fs::File::create(&tmp_path).await?;
VortexWriteOptions::new(session)
    .with_strategy(strategy)
    .write(&mut file, st.into_array().to_array_stream())
    .await?;
tokio::fs::rename(&tmp_path, &final_path).await?;
```

**Expected savings**: ~2-3 MB per flush (eliminates write_buf allocation).

**Effort**: Small — already proven pattern in merge.rs.

### Proposal 7: Intern tags_overflow instead of Vec<String>

**Problem**: `tags_overflow: Vec<String>` is NOT interned. Each row gets
its own String even if the overflow pattern is identical. With 10K rows
and ~500 unique patterns, we have 9,500 redundant allocations.

**Fix**: Replace `tags_overflow: Vec<String>` with a 10th `StringInterner`.
On flush, build VarBinArray from the interner's values + codes (like other
tag columns but as VarBinArray instead of DictArray, since the merge
pipeline handles it).

**Expected savings**: ~200-400 KB per flush.

**Effort**: Small — same pattern as existing tag interners.

### Proposal 8: Avoid sorted column copies during flush

**Problem**: During flush, we create `Vec<usize>` for sort order, then
build sorted copies of every column:
```rust
let sorted_overflow: Vec<Vec<u8>> = order
    .iter()
    .map(|&i| tags_overflow[i].as_bytes().to_vec())
    .collect();
```

This clones 10K strings for the overflow column alone.

**Fix**: Build VarBinArray with an indirection layer — pass the sort
permutation directly to the array builder instead of materializing sorted
copies. For PrimitiveArrays, use gather-by-index.

**Expected savings**: ~1-2 MB per flush (avoids cloned Vecs).

**Effort**: Medium — need to verify Vortex API supports this.

---

## P3 — Structural improvements

### Proposal 9: Use Vortex Writer push API for incremental flush

**Problem**: Currently we build a complete StructArray in memory (all 13
columns materialized), then stream it to the writer. This means all column
data exists twice briefly (in the interner + in the StructArray).

**Fix**: Use `VortexWriteOptions::writer()` push API to send small chunks
(e.g., 1K rows at a time) instead of materializing the full 10K-row array.

```rust
let mut writer = VortexWriteOptions::new(session)
    .with_strategy(strategy)
    .writer(&mut file, dtype);

for chunk in column_chunks(1000) {
    writer.push(chunk).await?;
}
writer.finish().await?;
```

**Expected savings**: ~1-2 MB (columns exist in small 1K-row chunks instead
of full 10K-row arrays).

**Effort**: Large — requires restructuring the flush path to emit chunks
incrementally from interners.

### Proposal 10: Reduce StringInterner double-clone

**Problem**: `intern()` clones the string twice when inserting a new value:
```rust
self.values.push(s.to_string());  // clone 1
self.map.insert(s.to_string(), c); // clone 2
```

**Fix**: Clone once, use `Rc<str>` or store index into values vec:
```rust
let owned = s.to_string();
self.map.insert(owned.clone(), c);
self.values.push(owned);
```

**Expected savings**: ~100-200 KB (one fewer allocation per unique string).

**Effort**: Trivial.

---

## Summary

| # | Proposal | Target | Savings | Effort | Priority |
|---|----------|--------|---------|--------|----------|
| 1 | Cap files per merge pass | Merge spike | ~200 MB peak | Small | P0 |
| 2 | Lighter merge strategy | Merge spike | ~60-80 MB peak | Medium | P0 |
| 3 | jemalloc purge after merge | Post-merge RSS | ~100 MB mean | Small | P0 |
| 4 | Context map string pool | Baseline RSS | ~28 MB | Medium | P1 |
| 5 | Expire stale contexts | Baseline RSS | ~6 MB | Small | P1 |
| 6 | Flush directly to file | Flush transient | ~2-3 MB | Small | P2 |
| 7 | Intern tags_overflow | Flush transient | ~300 KB | Small | P2 |
| 8 | Avoid sorted copies | Flush transient | ~1-2 MB | Medium | P2 |
| 9 | Push API for flush | Flush transient | ~1-2 MB | Large | P3 |
| 10 | Fix interner double-clone | Baseline | ~200 KB | Trivial | P3 |

**Recommended implementation order**: 1 → 3 → 6 → 4 → 2

Proposals 1 + 3 alone would bring mean RSS from 163 MB to ~80 MB and
peak from 460 MB to ~150 MB. Adding Proposal 4 would drop baseline to
~44 MB, matching the original pre-inline target.
