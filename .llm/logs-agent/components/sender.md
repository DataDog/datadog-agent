---
title: "Sender and Destinations"
kind: "component"
summary: "Reliable vs unreliable destination behavior, sender workers, buffering, and retry interaction."
source_paths:
  - "pkg/logs/sender/sender.go"
  - "pkg/logs/sender/worker.go"
  - "pkg/logs/client/destinations.go"
  - "pkg/logs/client/http/destination.go"
owns_globs:
  - "pkg/logs/sender/**"
  - "pkg/logs/client/**"
related_pages:
  - "invariants/sender-destination-semantics.md"
  - "invariants/auditor-delivery.md"
  - "architecture/pipeline-flow.md"
last_ingested_sha: "HEAD"
---

## Sender and Destinations

### Overview
The sender component of the logs agent is responsible for distributing log payloads to various destinations, ensuring that reliable destinations adhere to strict delivery guarantees while allowing for more flexible handling of unreliable destinations. This architecture is designed to optimize performance while maintaining data integrity.

### Delivery Guarantees

- **Reliable Destinations:**
  - Must accept payloads before the worker can proceed.
  - Can block progress if all reliable destinations are unhealthy.
  - Successfully sent payloads are forwarded to the auditor sink for persistence.

- **Unreliable Destinations:**
  - Operate on a best-effort basis.
  - Do not block progress, allowing the sender to continue processing.
  - Do not update the auditor, which can lead to potential data loss if relied upon for durability.

### Backpressure and Buffering
- The sender can buffer payloads if a previously successful destination becomes temporarily unhealthy, allowing for retries without losing data.
- Configurable parameters include:
  - Number of workers per queue.
  - Number of queues, adjustable for legacy or special modes.

### MRF Handling
- MRF (Multi-Resource Forwarding) destinations are specifically designed to handle MRF payloads, ensuring that only relevant data is sent to these destinations.

### Auditor Updates
- The sender ensures that updates to the auditor are only made when payloads are successfully sent to reliable destinations, preserving the integrity of the audit trail.

### Review Trap
- A critical review trap exists if changes inadvertently allow an unreliable path to affect durable progress. This could mislead the logs agent into believing data was committed when only a best-effort side path accepted it.

### Related Pages
- For more details on the semantics of sender and destination interactions, see [Sender and Destination Semantics](../invariants/sender-destination-semantics.md).
- For information on how auditor delivery and persistence are managed, refer to [Auditor Delivery and Persistence](../invariants/auditor-delivery.md).
- To understand the overall flow of data through the logs agent, check out [Pipeline Flow](../architecture/pipeline-flow.md). 

This architecture ensures that the logs agent can efficiently manage log data while adhering to strict delivery and persistence guarantees, crucial for maintaining the reliability of log processing systems.
