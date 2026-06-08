# Lookback Component — Design

## Why

The Datadog Agent aggregates raw metric samples before forwarding them to the backend.
For DogStatsD metrics, individual samples received at up to thousands per second are
reduced to a single aggregated point per 10-second flush window. Once flushed and sent,
the sub-flush raw samples are discarded — they are never stored and cannot be recovered.

The **lookback component** captures these raw, pre-aggregation samples on disk as they
flow through the metrics pipeline. The stored data can then be replayed to the backend
at arbitrary granularity — sub-second, 1s, 5s — for any past time window within the
retention period.

Primary use cases:

- **Higher-fidelity replay**: recover 1s granularity from a DogStatsD metric that was
  originally aggregated to 10s. Useful for debugging spikes or anomalies that were
  smoothed away during normal aggregation.
- **Backend-triggered replay**: an operator pushes a Remote Config payload from the
  Datadog UI; the agent queries its WAL and forwards the raw samples to the backend,
  making them available in the metrics platform without restarting or reconfiguring
  the agent.

---

## Pipeline integration

The lookback component subscribes to the agent's **metrics pipeline hook** — a typed
pub/sub channel that publishes `[]hook.MetricSampleSnapshot` batches. Each snapshot
carries a raw pre-aggregation sample: metric name, value, tags, nanosecond timestamp,
and a pre-computed context key (murmur3 hash of name+hostname+tags).

The subscription is registered via Fx's `group:"hook"` value group, which means the
component receives samples from **all** pipeline producers:

- **DogStatsD time sampler** — high-rate counters, gauges, histograms. Each sample
  arrives with its `ContextKey` pre-computed. This is the primary target.
- **Check sampler** and **no-aggregation worker** — `ContextKey = 0` in snapshots.
  The lookback computes a synthetic key from `murmur3(name + "|" + sortedTags)`.

On every received batch, for each snapshot the component:

1. Computes the context key (using the pre-computed value, or the synthetic one for
   `ContextKey = 0` sources).
2. Writes the context mapping (key → name, tags) to the context store, gated by a
   bloom filter — O(1), lock-free, ~3 ns on the hot path.
3. Appends a 24-byte WAL record to the appropriate shard — buffered in memory,
   flushed to disk when the per-shard write buffer fills (64 KB default).

---

## Data structures

### WAL — append-only sharded binary log

Raw samples are stored as fixed-size 24-byte records:

```
contextKey  uint64   (8 bytes, big-endian)
tsNs        int64    (8 bytes, Unix nanoseconds)
value       float64  (8 bytes, IEEE 754 big-endian)
```

The WAL is **sharded** by `contextKey % N` (N = 16 by default). Each shard is an
independent directory containing:

- An **active file** (`<window_start_unix_s>.wal`) — currently being written.
- Zero or more **sealed files** — previous rotation windows, read-only.

Sharding provides two benefits:

- **Write parallelism**: different context keys go to different shards with independent
  mutexes; concurrent writes do not contend.
- **Read locality**: a `Flush` query for a specific metric only reads the shard(s)
  that hold its context key.

Each shard keeps a **64 KB in-memory write buffer** that is flushed to disk when full
or on rotation, amortising the syscall cost of individual appends.

**Rotation** is triggered every `rotation_interval` (default 5 minutes) by a background
goroutine. On rotation, the active file is flushed, fsync'd, and closed; a new active
file is opened. File names encode the window-start Unix second, giving lexicographic
sort order for free.

**Retention** is enforced after each rotation:

1. Delete sealed files older than `max_age` (default 24h).
2. If total disk usage still exceeds `max_disk_bytes` (default 1 GB), delete oldest
   files until under the limit.

### Context store — sharded bloom-filtered flat files

To answer queries like `Flush("system.cpu.user", ["env:prod"])`, the component needs a
mapping from metric name + optional tag filter to the set of context keys stored in the
WAL.

The context store is a set of **16 append-only binary shard files**
(`contexts-00.bin` … `contexts-15.bin`), routed by `murmur32(metricName) % 16`.
A `Flush(name, …)` query reads exactly one shard, giving O(file_size / 16) scan cost
(~500 ms on a cluster with ~2000 unique metric names / 16 MB total store).

Each entry in a shard file is a variable-length binary record:

```
contextKey  uint64
nameLen     uint16
name        []byte
tagsCount   uint16
  tagLen    uint16  (per tag)
  tag       []byte
```

**Writes are gated by a lock-free bloom filter** (`contextSet`) of 9.6 M bits (~1.2 MB
fixed). The filter uses k=3 hash probes with atomic CAS — a bloom hit (key already
written) costs ~3 ns and acquires no lock. A bloom miss (new key, rare in steady state)
triggers an append to the shard file under a per-shard mutex. At 200k unique contexts,
the false-positive rate is ~0.5%, meaning at most 0.5% of new-key appends are
redundant; correctness is unaffected.

At startup, the bloom filter is re-populated by scanning all shard files once, ensuring
no re-writes after a process restart.

**Glob queries** (`Flush("kubernetes.*", …)`) cannot be routed to a single shard and
require scanning all 16 files, giving the same O(total_file_size) cost as the
unsharded design.

---

## Flush algorithm

```
Flush(ctx, name, tags, start, stop, interval):

  1. ctxFile.scan(name, tags)
       → read one context shard (exact) or all 16 (glob)
       → return map[contextKey → {name, sortedTags}]   (= resolver)

  2. keys = keys(resolver)

  3. for each WAL shard:
       sealedFiles = files whose window overlaps [start, stop)
       for each file:
         records += readAllRecords(file)

  4. filter records:
       keep r where r.contextKey ∈ keys AND start ≤ r.tsNs < stop

  5. hash-group sort:
       groups = map[contextKey → []index]  — O(N), groups by key
       sort unique keys                    — O(K log K)
       for each key:
         sort indices by tsNs              — exploits temporal monotonicity

  6. streaming aggregation:
       for each record in sorted order:
         tsBucket = floor(r.tsNs / intervalNs) * intervalNs
         if (key, tsBucket) changed: emit previous bucket
         accumulate: Count++, Sum += value, update Min/Max

  7. return []Bucket{Name, Tags, Ts, Count, Sum, Min, Max}
```

Complexity: O(N + K log K) where N = total filtered records, K = unique context keys
in range. In practice K ≪ N because each context key appears many times. The sort
of unique keys is the dominant cost only for high-cardinality queries.

**Step 5 exploits temporal monotonicity**: within any shard, records for a given
context key arrive in time order (the agent emits metrics in real time). Sorting indices
within each group is therefore O(n log n) where n is typically small (a few hundred
samples per 5-minute window per context).

---

## API surface

### HTTP (via AgentEndpointProvider)

| Endpoint | Method | Description |
|---|---|---|
| `/agent/lookback-flush` | GET | Returns `[]Bucket` as JSON. Does not forward to backend. |
| `/agent/lookback-forward` | POST | Runs flush then sends via `serializer.SendIterableSeries`. Returns `{"forwarded": N}`. |

Both endpoints share query parameters:

| Parameter | Required | Description |
|---|---|---|
| `name` | yes | Exact metric name or glob (`kubernetes.*`, `*.memory.rss`) |
| `tags` | no | Comma-separated inclusion filter — returns contexts whose tag set is a superset |
| `start` | yes | Range start, Unix nanoseconds |
| `stop` | yes | Range stop, Unix nanoseconds (half-open) |
| `interval` | no | Aggregation bucket width as Go duration (`1s`, `500ms`). Default: 1s |

`/agent/lookback-forward` additionally accepts:

| Parameter | Required | Description |
|---|---|---|
| `mtype` | no | `gauge` \| `count` \| `rate`. Default: gauge |

### Remote Config (DEBUG product)

The component registers an `RCListener` on the `DEBUG` RC product (staging only).
When the backend pushes a config targeted at a specific agent hostname, the same
flush+forward pipeline is triggered. The config payload is a JSON object:

```json
{
  "name":        "kubernetes.memory.rss",
  "tags":        "env:prod,region:us",
  "start":       1779285200000000000,
  "stop":        1779285500000000000,
  "interval_ms": 1000
}
```

The apply status (`ApplyStateAcknowledged` / `ApplyStateError`) is reported back to RC.

---

## Component dependencies

| Dependency | Role |
|---|---|
| `aggregator.Demultiplexer` | Provides `Serializer()` used by `/lookback-forward` and the RC handler to send series to the Datadog intake |
| `hostnameinterface.Component` | Fills `Serie.Host` for correct metric attribution in the backend |
| `config.Component` | Reads `lookback.*` configuration keys at startup |
| `log.Component` | Structured logging |
| `hook.Hook[[]MetricSampleSnapshot]` | Metrics pipeline hook — source of raw pre-aggregation samples (consumed via Fx `group:"hook"`) |
| `api.AgentEndpointProvider` (×2) | Registers `/lookback-flush` and `/lookback-forward` on the agent's internal API server |
| `rcclienttypes.ListenerProvider` | Registers the DEBUG product callback on the RC client (output group, no inbound dependency) |

---

## Configuration

| Key | Default | Description |
|---|---|---|
| `lookback.enabled` | `false` | Enable the component |
| `lookback.dir` | `/tmp/datadog-lookback` | Base directory for all WAL and context store files |
| `lookback.num_shards` | `16` | WAL shard count |
| `lookback.rotation_interval` | `5m` | WAL rotation period |
| `lookback.max_age` | `24h` | Retention: delete sealed files older than this |
| `lookback.max_disk_bytes` | `1 GB` | Retention: delete oldest files if total exceeds this |
| `lookback.write_buffer_size` | `64 KB` | Per-shard in-memory write buffer |
