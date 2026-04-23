---
title: "Pipeline Ordering Invariants"
kind: "invariant"
summary: "Each input stays pinned to one pipeline so message order is preserved for that input."
source_paths:
  - "pkg/logs/README.md"
  - "comp/logs-library/pipeline"
  - "pkg/logs/sender/sender.go"
owns_globs:
  - "pkg/logs/**"
  - "comp/logs-library/pipeline/**"
related_pages:
  - "architecture/logs-agent-overview.md"
  - "architecture/pipeline-flow.md"
  - "playbooks/review-checklist.md"
last_ingested_sha: "HEAD"
---

# Pipeline Ordering Invariants

## Overview

This page outlines the critical invariants related to message ordering within the logs agent's pipeline architecture. Each input must remain pinned to a single pipeline to ensure that message order is preserved throughout processing.

## Invariants

- **Single Pipeline Assignment**: Each input is assigned to one pipeline, maintaining a stable ordered path through the system.
- **Fewer Pipelines than Inputs**: The architecture intentionally has fewer pipelines than inputs, which helps in preserving order.
- **Stable Processing Path**: Each input should consistently follow the same route through the processor, strategy, sender, and destinations.

## Review Checklist

When reviewing changes, consider the following questions to ensure ordering guarantees are not compromised:

- Does the change rebalance or reassign an input after startup?
- Does the change create multiple competing output paths for one input?
- Does the change couple retries or batching across unrelated inputs in a way that could reorder messages?

## Why It Matters

- **Downstream Assumptions**: Many downstream systems rely on the assumption that one input is serialized through one pipeline.
- **Throughput vs. Ordering**: Attempts to improve throughput by sharing resources can inadvertently sacrifice ordering guarantees.

For further details on the pipeline architecture, see [Pipeline Flow](../architecture/pipeline-flow.md) and for review guidelines, refer to the [Logs Agent Review Checklist](../playbooks/review-checklist.md). 

### Related Pages

- [Logs Agent Architecture](../architecture/logs-agent-architecture.md)
- [Logs Processing Pipeline](../architecture/logs-processing-pipeline.md)
- [Logs Agent Configuration](../configs/logs-agent-config.md)
