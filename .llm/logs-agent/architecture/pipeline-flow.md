---
title: "Pipeline Flow"
kind: "architecture"
summary: "Tailer-to-auditor data flow, including processor, strategy, sender, destination, and retry boundaries."
source_paths:
  - "pkg/logs/README.md"
  - "pkg/logs/sender/sender.go"
  - "pkg/logs/sender/worker.go"
  - "pkg/logs/client/http/destination.go"
owns_globs:
  - "pkg/logs/sender/**"
  - "pkg/logs/client/**"
  - "pkg/logs/processor/**"
related_pages:
  - "components/sender.md"
  - "components/processor.md"
  - "components/auditor.md"
  - "invariants/sender-destination-semantics.md"
  - "invariants/auditor-delivery.md"
last_ingested_sha: "HEAD"
---

# Pipeline Flow

The **Pipeline Flow** page outlines the data delivery path from the tailer to the auditor within the logs-agent architecture. It details the roles of each component involved in processing and delivering log messages, emphasizing reliability and ordering guarantees.

## Delivery Path

The primary flow of data is as follows:

- **Tailer/Decoder** → **Processor** → **Strategy** → **Sender** → **Destination** → **Auditor**

## Component Responsibilities

- **Processor**: 
  - Mutates messages and applies processing rules.
  - Prepares messages for encoding.

- **Strategy**: 
  - Batches or frames messages into payloads.

- **Sender**: 
  - Multiplexes payloads onto worker queues.
  - Coordinates between reliable and unreliable destinations.

- **Destination**: 
  - Manages transport, concurrency, retries, and successful delivery signaling.

- **Auditor**: 
  - Consumes successful payload acknowledgments.
  - Converts them into durable offsets or timestamps.

## Review Considerations

- **Durable Delivery**: 
  - Delivery is only complete when a reliable destination acknowledges success and the auditor records the payload.
  
- **Unreliable Destinations**: 
  - Operate on a best-effort basis and should not impact progress accounting.

- **Risk of Duplication**: 
  - Changes that decouple destination success from auditor updates can lead to duplicate payloads after a restart.

- **Input Ordering**: 
  - Any modifications allowing input drift across pipelines can cause ordering regressions.

## Common Failure Modes

- Acknowledging payloads prematurely before a reliable send completes.
- Misinterpreting buffered sends as durable sends.
- Confusing MRF (Multi-Resource Forwarding) and non-MRF traffic due to routing changes.
- Altering shutdown sequences such that in-flight payloads are not flushed to the auditor before a restart.

## Related Pages

- [Sender and Destinations](../components/sender.md)
- [Auditor Component](../components/auditor.md)
- [Auditor Delivery and Persistence](../invariants/auditor-delivery.md)

This page serves as a foundational reference for understanding the logs-agent's data flow and the critical guarantees that must be maintained throughout the logging process. For a deeper understanding of the sender's role, refer to the [Sender and Destinations](../components/sender.md) page.
