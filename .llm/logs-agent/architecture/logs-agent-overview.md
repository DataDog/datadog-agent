---
title: "Logs Agent Overview"
kind: "architecture"
summary: "High-level map of how sources, pipelines, destinations, and the auditor fit together."
source_paths:
  - "pkg/logs/README.md"
  - "comp/logs/agent/agentimpl/agent.go"
owns_globs:
  - "pkg/logs/README.md"
  - "comp/logs/agent/agentimpl/agent.go"
related_pages:
  - "architecture/pipeline-flow.md"
  - "architecture/source-discovery.md"
  - "invariants/pipeline-ordering.md"
last_ingested_sha: "HEAD"
---

The logs agent architecture is structured around two primary halves: the discovery of log sources and the delivery of collected logs through a series of pipelines to the intake. This architecture can be divided into two main paths:

## Discovery Path
- **Schedulers**: Populate sources and services dynamically.
- **Launchers**: Create tailers based on the populated sources and services.

## Delivery Path
- **Tailers**: Feed into decoders, processors, strategies, senders, and destinations.
- **Auditor**: Ensures durable progress of logs.

### Key Architectural Boundaries
- **Sources and Services**: 
  - Long-lived stores shared across schedulers and launchers.
  
- **Pipelines**: 
  - Fewer in number than inputs; multiple inputs can multiplex into a limited number of ordered pipelines.
  
- **Destinations**: 
  - Handle transport and retry behavior, while the auditor manages durable progress.

- **Partial Restart**: 
  - Preserves persistent state (e.g., sources, tracker, schedulers, auditor) while rebuilding transient transport components.

### Invariants
- Input-to-pipeline ordering must be maintained.
- Sender reliability vs. unreliable destination semantics must be clearly defined.
- The auditor must ensure acknowledgment flow and persistence.
- Tailer position and fingerprint recovery must be robust.
- Config-driven path changes must be handled gracefully during service interactions.

### Review Trap
- Changes that seem localized to sender, restart, or tailer setup can inadvertently disrupt the overall delivery contract, especially if they affect how offsets become durable or how inputs are bound to pipelines.

For further details, see related pages:
- [Pipeline Flow](pipeline-flow.md)
- [Source Discovery and Launchers](source-discovery.md)
- [Pipeline Ordering Invariants](../invariants/pipeline-ordering.md)

The component wiring is primarily located in `comp/logs/agent/agentimpl/agent.go` and `agent_core_init.go`. The documentation in `pkg/logs/README.md` is crucial as it outlines invariants that may be overlooked during isolated code reviews.
