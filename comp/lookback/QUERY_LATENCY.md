# Lookback — Query latency reference

Measured on `stormeagle.us1.staging.dog` (16-node staging cluster).
Agent running `dev-misteriaud-loopback-wal-43aec58e`, 5-min query window.
Context store: 16 shards × ~1 MB = ~16 MB total, ~1 800 unique contexts.

## Exact metric name

| Metric | Latency | Payload |
|---|---|---|
| `kubernetes.memory.rss` | **518 ms** | 150 KB |
| `cilium.bpf.map_pressure` | **532 ms** | 56 KB |

Cost: O(1/16 × context_store_size) — reads one shard file only.

## Wildcard metric name

| Pattern | Matches | Latency | Payload |
|---|---|---|---|
| `kubernetes.*` | 70 metrics, 2 640 buckets | **1.79 s** | 3.2 MB |
| `cilium.bpf.*` | 5 metrics, 288 buckets | **1.78 s** | 390 KB |

Cost: O(context_store_size) — must scan all 16 shards.
Wildcard latency ≈ previous linear-scan latency (unchanged by sharding).

## Before sharding (single `contexts.bin`, 17 MB)

All exact queries: **~1.7 s** regardless of metric name.

## Payload note

Tags are serialized verbatim per bucket. On Kubernetes clusters each context
carries 30+ labels, making the tag list the dominant payload contributor
(~2 KB per bucket for kubernetes.* metrics).
