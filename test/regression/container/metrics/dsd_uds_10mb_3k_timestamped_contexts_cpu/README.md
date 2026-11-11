# DogStatsD Timestamped Contexts - CPU

## Overview

This experiment tests the Datadog Agent's CPU performance under a DogStatsD
workload consisting entirely of timestamped counts and gauges over a fixed,
bounded set of contexts. It establishes a baseline for the timestamp
passthrough path, where every submitted metric carries an explicit timestamp
rather than being aggregated into the current flush interval.

A sibling experiment, `dsd_uds_10mb_3k_timestamped_contexts_memory`, measures
the same workload for memory.

## What It Tests

- **Throughput**: 10 MiB/s of DogStatsD traffic via Unix Domain Socket.
- **Message Composition**: 100% metrics, split 208:66 between counts and
  gauges (no timers, distributions, sets, or histograms).
- **Contexts**: A fixed 3,000 contexts, so the tag set stays bounded and
  repeats across the run rather than growing unbounded.
- **Timestamps**: Every metric carries an explicit timestamp within a fixed
  historical range, forcing the timestamp passthrough path (`noAggregationStreamWorker`)
  instead of the normal aggregation path. That path builds and serializes a
  `Serie` directly per sample; it does not use a context resolver or cache.

This is intended to catch regressions in the timestamp passthrough path
specifically — tag enrichment, serie construction, and serialization for
timestamped samples — as opposed to the general aggregated DogStatsD
ingestion path already covered by `quality_gate_metrics_logs`.
