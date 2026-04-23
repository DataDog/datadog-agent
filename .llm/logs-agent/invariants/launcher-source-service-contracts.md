---
title: "Launcher Source and Service Contracts"
kind: "invariant"
summary: "Launchers rely on source and service stores differently, especially for container collection and autodiscovery timing."
source_paths:
  - "pkg/logs/README.md"
  - "pkg/logs/schedulers/README.md"
  - "comp/logs/agent/agentimpl/agent_core_init.go"
owns_globs:
  - "pkg/logs/launchers/**"
  - "pkg/logs/schedulers/**"
  - "pkg/logs/service/**"
  - "pkg/logs/sources/**"
related_pages:
  - "architecture/source-discovery.md"
  - "components/launchers.md"
  - "configs/logs-agent-config-flags.md"
last_ingested_sha: "HEAD"
---

# Launcher Source and Service Contracts

Launchers in the logs agent interpret source and service states differently, which is crucial for ensuring reliable log collection, especially in containerized environments. The timing of service and source availability can lead to race conditions that impact log ingestion.

## Key Invariants

- **Independent Consumption**: Not all launchers consume both sources and services.
- **Stable Reconciliation**: Container launchers require stable reconciliation between services and sources to function correctly.
- **Startup Sequencing**: The `container_collect_all` function minimizes autodiscovery race conditions by controlling the startup sequence.

## Review Considerations

- **Generic Handling Risks**: Be cautious of launcher code that merges source and service handling into a single generic path, as this often overlooks container-specific edge cases.

Understanding these contracts is essential for maintaining the integrity of the logs agent's architecture and ensuring that log collection operates smoothly across various environments.

For further details, see [Source Discovery and Launchers](../architecture/source-discovery.md) and [Logs Agent Config Flags with Architectural Side Effects](../configs/logs-agent-config-flags.md).

## Related Components

- **Launchers**: Implemented in `pkg/internal/launchers`, responsible for creating tailers based on sources and services.
- **Schedulers**: Manage log sources and services, ensuring they are recognized by launchers. More information can be found in [Schedulers](../components/schedulers.md).
