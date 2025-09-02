# Quality Gate Metrics & Logs Experiment

## Overview

This quality gate experiment tests the Datadog Agent's performance and resource
consumption under a combined workload of high-volume DogStatsD metrics and
continuous log collection. It validates that the agent can handle both data
streams simultaneously while staying within defined memory and CPU bounds.

## What It Tests

This experiment generates:

1. **DogStatsD Metrics**: High-volume metrics traffic via Unix Domain Socket
2. **Log Collection**: Continuous log generation with file rotation, in a
   Kubernetes fashion

This combination is intended to model a high-end production scenario.

## Parameter Derivation

### DogStatsD Configuration

- **Throughput**: 100 MiB/s is chosen to represent a high-end production use
  case
- **Message Composition**:
  - 90% metrics (87% counters, 8% gauges, 5% distributions)
  - 5% events
  - 5% service checks
- **Tag Configuration**: 2-50 tags per message with 3-150 character lengths
- **Multivalue Packing**: 8% probability with 2-32 values per pack

These sourcing for these parameters come from the former `uds_dogstatsd_to_api`
experiment and should be regularly re-examined in-light of customer data and
product goals.

### Logs Configuration

- **Rate**: 500 KiB/s constant load
- **Rotation**: 5 rotations with 50 MiB maximum per log file
- **Concurrent Logs**: 2 active log files
- **Format**: ASCII text logs

The log generation pattern simulates typical application logging with rotation,
representing services that continuously generate logs. Rotation happens in the
manner of Kubernetes.
