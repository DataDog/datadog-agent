---
title: "Auditor Delivery and Persistence"
kind: "invariant"
summary: "Only successfully delivered reliable payloads should advance auditor state, and restart paths must flush that state safely."
source_paths:
  - "pkg/logs/README.md"
  - "pkg/logs/sender/worker.go"
  - "comp/logs/auditor/impl/auditor.go"
  - "comp/logs/agent/agentimpl/agent_restart.go"
owns_globs:
  - "pkg/logs/sender/**"
  - "comp/logs/auditor/**"
  - "comp/logs/agent/agentimpl/**"
related_pages:
  - "components/auditor.md"
  - "components/restart-lifecycle.md"
  - "playbooks/review-checklist.md"
last_ingested_sha: "HEAD"
---

# Auditor Delivery and Persistence

The logs agent's auditor component is responsible for ensuring that only successfully delivered reliable payloads advance the auditor's state. This invariant is essential for maintaining data integrity during restart recovery, preventing excessive data replay or critical log omissions.

## Key Invariants

- **Durable Progress**: Only payloads that have been reliably delivered should update the auditor's state.
- **Unreliable Destinations**: Payloads sent to unreliable destinations do not advance offsets in the auditor.
- **Flush on Restart**: Restart paths must flush the auditor's state after transient pipeline components cease operation.
- **Offset Management**: Offset updates must not regress when late acknowledgments are received.

## Failure Modes

- **Duplicate Logs**: Changes in sender or restart logic that prioritize throughput over the timing of auditor updates can lead to duplicate logs.
- **Lost Acknowledgments**: Inadequate acknowledgment management may result in lost logs or incorrect auditor state.

## Review Traps

- Scrutinize changes that enhance throughput but alter the timing of auditor updates for potential duplicate log risks.
- Evaluate modifications to sender, destination, restart, or file-position code against these invariants to ensure compliance.

## Related Components

- [Sender](../components/sender.md): Manages the transmission of logs to various destinations, including both reliable and unreliable ones.
- [Restart Mechanism](../components/restart.md): Ensures graceful restart of the logs agent while preserving the auditor's state.
- [Auditor Component](../components/auditor.md): Oversees the registry of log offsets and maintains data integrity during log processing.

This invariant is critical to verify during PR reviews involving changes to the sender, destination, restart, or file-position logic. Always ensure that the architecture's integrity is maintained to prevent data loss or duplication.
