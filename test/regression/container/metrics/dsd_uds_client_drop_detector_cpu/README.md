# DogStatsD Client Drop Detector - CPU

## Overview

This experiment measures the CPU cost of the DogStatsD client drop detector in
the full Agent. The workload deliberately produces many final aggregated series
whose names and tags enter the detector's matched path. This complements the
component microbenchmark with the real DogStatsD parser, aggregator, serializer
flush, COAT observer, and detector wiring.

The detector is explicitly enabled so the experiment continues to measure its
cost independently of the Agent's default configuration.

## Workload

- **Transport**: 10 MiB/s over a DogStatsD Unix datagram socket.
- **Metric types**: Non-timestamped counts, which use the normal aggregation
  path and become final rate series.
- **Metric names**: The four exact client-byte telemetry names consumed by the
  detector: bytes sent, total bytes dropped, queue drops, and writer drops.
- **Contexts**: Each metric generator builds 2,500 context templates from a
  pool of 10,000 `context_id` values. Because values are sampled with
  replacement, this produces roughly 8,000-9,000 distinct matching contexts
  across the four metric names. This amplifies the per-final-series observer
  cost while keeping context cardinality bounded.
- **Detector tags**: The Agent's test configuration appends `client:go` and
  `client_transport:uds` to every input. Lading's tag-name and tag-value pools
  are selected independently, so adding these fixed tags in the Agent config
  is the deterministic way to produce a supported pair.
- **Health state**: Sent metrics use a value of 1,000; total drops use 2; queue
  and writer drops use 1. The resulting total-drop ratio remains well below the
  default 1% threshold. This exercises steady-state detector accumulation and
  end-of-flush evaluation without adding one-time issue-reporting noise.

## What It Tests

For matching final series, this experiment covers exact-name dispatch, client
and transport tag extraction, rate-to-byte conversion, COAT counter updates,
per-client-library/transport detector accumulation, and end-of-flush health
evaluation. The captured Agent telemetry makes it possible to verify that the
DogStatsD listener and aggregator processed the workload when investigating a
run.

The experiment is CPU-focused because the detector retains a fixed amount of
state per supported client-library and transport pair rather than per metric
context. General non-matching DogStatsD traffic is already covered by
`quality_gate_metrics_logs`. Agent Health issue creation, confirmation,
recovery, and persistence are correctness behaviors covered by unit and
end-to-end tests rather than this performance experiment.
