---
title: "Logs Agent Config Flags with Architectural Side Effects"
kind: "config"
summary: "A curated list of config flags that change delivery semantics, tailer selection, restart behavior, or registry persistence."
source_paths:
  - "pkg/logs/README.md"
  - "comp/logs/auditor/impl/auditor.go"
  - "comp/logs/agent/agentimpl/agent_core_init.go"
  - "pkg/logs/launchers/container/tailerfactory/whichtailer.go"
owns_globs:
  - "comp/logs/agent/config/**"
  - "comp/logs/auditor/**"
  - "pkg/logs/launchers/**"
related_pages:
  - "architecture/source-discovery.md"
  - "invariants/launcher-source-service-contracts.md"
last_ingested_sha: "HEAD"
---

# Logs Agent Config Flags with Architectural Side Effects

This page provides a curated list of configuration flags for the logs agent that have significant implications on delivery semantics, tailer selection, restart behavior, or registry persistence. Understanding these flags is crucial as they can alter the fundamental contracts of the logs agent.

## Key Config Flags

- **`logs_config.docker_container_use_file`**: 
  - Prefers file-backed container collection when possible.
  
- **`logs_config.docker_container_force_use_file`**: 
  - Requires file-backed container collection, overriding other settings.
  
- **`logs_config.auditor_ttl`**: 
  - Controls the time-to-live for stale registry entries, impacting registry persistence.
  
- **`logs_config.message_channel_size`**: 
  - Adjusts the buffering size in auditor and diagnostics paths, affecting throughput.
  
- **`logs_config.atomic_registry_write`**: 
  - Changes how registry persistence handles crash safety, influencing data integrity.
  
- **`logs_config.pipelines`**: 
  - Modifies the level of pipeline parallelism without affecting per-input ordering guarantees.
  
- **`logs_config.stop_grace_period`**: 
  - Affects the timing of restarts and shutdowns, impacting service availability.

## Review Considerations

- **Config Changes as Refactors**: 
  - Changes in configuration plumbing may appear as harmless refactors but can introduce risks related to file selection, durability, or restart sequencing.
  
- **Architectural Impact**: 
  - Review how these flags interact with the overall architecture, especially in relation to [Schedulers and Launchers](../architecture/schedulers-and-launchers.md) and [Auditor Behavior](../components/auditor.md).

## Related Pages

- [Logs Agent Architecture Overview](../architecture/overview.md)
- [Auditor Component Details](../components/auditor.md)
- [Schedulers and Launchers](../architecture/schedulers-and-launchers.md)
- [Logs Agent Invariants](../invariants/logs-agent-invariants.md)

Understanding these configuration flags is essential for maintaining the integrity and reliability of the logs agent's operations. Always consider the architectural implications when modifying these settings.
