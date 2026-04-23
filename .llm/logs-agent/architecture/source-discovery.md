---
title: "Source Discovery and Launchers"
kind: "architecture"
summary: "How schedulers, sources, services, and launchers cooperate to create tailers and attach them to pipelines."
source_paths:
  - "pkg/logs/README.md"
  - "pkg/logs/launchers/README.md"
  - "pkg/logs/schedulers/README.md"
  - "comp/logs/agent/agentimpl/agent_core_init.go"
owns_globs:
  - "pkg/logs/launchers/**"
  - "pkg/logs/schedulers/**"
  - "pkg/logs/sources/**"
related_pages:
  - "components/launchers.md"
  - "invariants/launcher-source-service-contracts.md"
  - "configs/logs-agent-config-flags.md"
last_ingested_sha: "HEAD"
---

## Source Discovery and Launchers

This document details how schedulers, sources, services, and launchers collaborate to create tailers and attach them to the logs-agent pipeline. It emphasizes the discovery fan-in, launcher responsibilities, and the influence of configuration on tailer selection.

## Overview

- **Schedulers** dynamically manage log sources and services during runtime, determining what should be logged.
- **Launchers** translate scheduling decisions into tailers, which are responsible for collecting logs.

## Discovery Contracts

The interaction between schedulers and launchers is governed by several key contracts:

- **Dynamic Management**: Schedulers can add or remove sources and services at runtime.
- **Asymmetric Consumption**: Not all launchers consume both sources and services:
  - Container-related launchers reconcile services with sources.
  - Simpler launchers may only monitor sources.
- **File Source Creation**: The Kubernetes launcher can generate file sources instead of tailing directly.
- **Child Source Creation**: File and container launchers can create additional child sources based on runtime configurations.

## High-Sensitivity Areas

- **`container_collect_all` Timing**: Startup sequencing is critical to minimize race conditions with autodiscovery-provided sources.
- **Container Log Handling**:
  - `docker_container_use_file` and `docker_container_force_use_file` dictate whether logs are read via API or converted into file sources.
- **File Handling**: File launchers can create multiple tailers per source, particularly during file rotation.

## Review Traps

Changes to launcher filtering or source generation can disrupt downstream expectations regarding:

- Source identity
- Ordering
- Offset reuse
- Duplicate collection

## Related Pages

- [Launchers and Tailers](../components/launchers.md)
- [Launcher Source and Service Contracts](../invariants/launcher-source-service-contracts.md)
- [Logs Agent Config Flags with Architectural Side Effects](../configs/logs-agent-config-flags.md)

This page serves as a foundational reference for understanding the architecture of source discovery and the roles of launchers within the logs-agent ecosystem. For further details on the components involved, refer to the related pages listed above.
