---
title: "Launchers and Tailers"
kind: "component"
summary: "Launcher-specific behavior for file, container, journald, listeners, and Windows event ingestion."
source_paths:
  - "pkg/logs/launchers/README.md"
  - "pkg/logs/tailers/README.md"
  - "pkg/logs/launchers/file/position.go"
  - "comp/logs/agent/agentimpl/agent_core_init.go"
owns_globs:
  - "pkg/logs/launchers/**"
  - "pkg/logs/tailers/**"
related_pages:
  - "architecture/source-discovery.md"
  - "invariants/launcher-source-service-contracts.md"
  - "invariants/tailer-position-and-fingerprint.md"
last_ingested_sha: "HEAD"
---

# Launchers and Tailers

Launchers are integral to the logs-agent architecture, responsible for translating sources into active tailers and managing their lifecycle. This page outlines launcher-specific behaviors, common pitfalls, and their implications on log ingestion.

## Launcher-Specific Behaviors

- **File Launchers**:
  - Manage wildcard expansion, log rotation, and selection of restart positions.
  - Ensure correct handling of offsets during log resumption.

- **Container Launchers**:
  - Switch between Docker API tailing and file tailing based on configuration settings.
  - Impact log ingestion paths depending on the chosen mode.

- **Kubernetes Discovery**:
  - Generate file sources instead of direct tailers, affecting ingestion strategies.

- **Service and Source Consumption**:
  - Different launchers may consume services or sources, with some reconciling both.

## Tailer-Specific Behaviors

- **Message Production**:
  - Tailers send messages into pipelines; file-like tailers rely on auditor state for accurate resumption.

- **Tracker and Source Identity**:
  - Critical during stop/restart operations to prevent duplicate attachments or leaked tailers.

## Review Traps

- **Source Identity Changes**:
  - Modifications in source identity can lead to the auditor treating the same physical log as a new stream, risking data loss or duplication.

## Invariants

- Launchers must maintain consistent source identity to ensure reliable log ingestion.
- Tailers should accurately track their position to prevent message loss during restarts.
- Configuration changes must be carefully managed to avoid unintended regressions in log handling.

## Related Pages

- [Source Discovery and Launchers](../architecture/source-discovery.md)
- [Launcher Source and Service Contracts](../invariants/launcher-source-service-contracts.md)
- [Tailer Position and Fingerprint Recovery](../invariants/tailer-position-and-fingerprint.md)

This page serves as a durable reference for understanding the intricacies of launchers and tailers within the logs-agent architecture, ensuring that changes in source or configuration do not regress critical behaviors.
