---
title: "Restart Lifecycle"
kind: "component"
summary: "Partial restart flow, transport switching, persistent state reuse, and graceful stop expectations."
source_paths:
  - "comp/logs/agent/agentimpl/agent_restart.go"
  - "comp/logs/agent/agentimpl/agent_core_init.go"
  - "comp/logs/agent/agentimpl/agent.go"
owns_globs:
  - "comp/logs/agent/agentimpl/**"
related_pages:
  - "invariants/graceful-restart.md"
  - "components/auditor.md"
  - "components/sender.md"
last_ingested_sha: "HEAD"
---

The logs agent features a partial restart mechanism designed for transport switching and similar reconfigurations, allowing for state preservation without a full system teardown.

### Persistent Components Across Partial Restart
The following components maintain their state during a partial restart:
- **Sources**
- **Services and Schedulers**
- **Auditor**
- **Tracker**
- **Diagnostic Message Receiver**

### Rebuilt Components During Partial Restart
The following components are reconstructed:
- **Destinations Context**
- **Pipeline Provider**
- **Launchers**

### Critical Ordering for Restart
To ensure a smooth transition during a partial restart, the following order of operations is critical:
1. **Stop transient delivery components** first.
2. **Flush the auditor** after stopping the pipeline.
3. **Build and start new transient components**.
4. **Rollback** to previous endpoints if the rebuild fails.

### Review Trap
Be cautious of changes that may alter the order of stop, flush, or rebuild steps. Such changes can lead to:
- Duplicate retry loops
- Loss of in-memory acknowledgments
- Launchers remaining attached to stale pipeline state

### Related Pages
- [Graceful Restart Invariants](../invariants/graceful-restart.md)
- [Auditor Component](auditor.md)

This page serves as a guide to understanding the restart lifecycle within the logs agent architecture, emphasizing the importance of component persistence and the critical order of operations during partial restarts. For further details on the implications of these components, refer to the related pages listed above.
