# DogStatsD Timestamped Contexts - Memory

## Overview

This experiment tests the Datadog Agent's memory usage under a DogStatsD
workload consisting entirely of timestamped counts and gauges over a fixed,
bounded set of contexts. It establishes a baseline for the timestamp
passthrough path, where every submitted metric carries an explicit timestamp
rather than being aggregated into the current flush interval.

A sibling experiment, `dsd_uds_10mb_3k_timestamped_contexts_cpu`, measures
the same workload for CPU.

## What It Tests

- **Throughput**: 10 MiB/s of DogStatsD traffic via Unix Domain Socket.
- **Message Composition**: 100% metrics, split 208:66 between counts and
  gauges (no timers, distributions, sets, or histograms).
- **Contexts**: A fixed 3,000 contexts, so the workload exercises context
  resolution/caching rather than unbounded context growth.
- **Timestamps**: Every metric carries an explicit timestamp within a fixed
  historical range, forcing the timestamp passthrough path instead of the
  normal aggregation path.

This is intended to catch regressions in how the Agent resolves and caches
contexts for timestamped metrics specifically, as opposed to the general
DogStatsD ingestion path already covered by `quality_gate_metrics_logs`.
