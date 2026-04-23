---
title: "Graceful Restart Invariants"
kind: "invariant"
summary: "Partial restart must preserve persistent components, flush offsets, and avoid duplicate retry loops or dropped state."
source_paths:
  - "comp/logs/agent/agentimpl/agent_restart.go"
  - "comp/logs/agent/agentimpl/agent.go"
owns_globs:
  - "comp/logs/agent/agentimpl/**"
related_pages:
  - "components/restart-lifecycle.md"
  - "components/auditor.md"
  - "playbooks/review-checklist.md"
last_ingested_sha: "HEAD"
---

Partial restart is a lifecycle optimization that ensures the durability of persistent components while allowing for the rebuilding of transport-facing elements. This process must adhere to strict guarantees to maintain system integrity and prevent data loss.

### Hard Rules

- **Persistent State Preservation**: Persistent state must remain unchanged during a restart unless explicitly reinitialized.
- **Clean Stop for Transient Components**: All transient components must stop cleanly before new instances are initiated.
- **Auditor Flush Timing**: The auditor must flush its state after transient delivery components have stopped to ensure no data is lost.
- **Retry Loop Management**: Any existing retry loops should be canceled or replaced to prevent duplicate processing of logs.

### Review Considerations

- **Asynchronous Changes**: Any modifications that make the restart process "more async" or "less blocking" require additional scrutiny, particularly regarding offset flushing and goroutine cleanup.

For more detailed information on the restart lifecycle, see [Restart Lifecycle](../components/restart-lifecycle.md) and for auditor behavior, refer to [Auditor Delivery and Persistence](auditor-delivery.md). 

### Related Invariants

- For guarantees on delivery and ordering, refer to [Delivery Invariants](../invariants/delivery.md).
- For insights on component lifecycle management, see [Component Lifecycle](../components/lifecycle.md).
