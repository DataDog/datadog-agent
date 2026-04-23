---
title: "Streamlogs and Analyzelogs Adjacent Interfaces"
kind: "adjacent"
summary: "CLI entrypoints and adjacent logs components that expose or consume logs-agent behavior."
source_paths:
  - "cmd/agent/subcommands/streamlogs/command.go"
  - "cmd/agent/subcommands/analyzelogs/command.go"
  - "comp/logs/streamlogs/impl/streamlogs.go"
owns_globs:
  - "cmd/agent/subcommands/streamlogs/**"
  - "cmd/agent/subcommands/analyzelogs/**"
  - "comp/logs/streamlogs/**"
related_pages:
  - "architecture/logs-agent-overview.md"
  - "components/restart-lifecycle.md"
last_ingested_sha: "HEAD"
---

# Streamlogs and Analyzelogs Adjacent Interfaces

## Overview

This page details the CLI entrypoints and adjacent components that interact with the logs-agent, specifically focusing on `streamlogs` and `analyzelogs`. These interfaces are critical for operator visibility and diagnostics, and changes in internal contracts may affect their behavior.

## Importance

- **CLI Entry Points**: These commands encapsulate assumptions about the logs-agent's startup, diagnostics, and state visibility.
- **Lifecycle Dependencies**: Adjacent components may become outdated if internal lifecycle or data contracts change, potentially leading to unexpected behavior.
- **Review Scope**: Changes in these paths should be included in review processes, as they can indicate whether refactors impact user-facing functionality.

## Invariants

- **Command Reliability**: Both `streamlogs` and `analyzelogs` must maintain consistent output and error handling.
- **Configuration Integrity**: Changes in configuration files should not disrupt the expected behavior of these commands.
- **Data Flow**: The commands must correctly handle data flow from the logs-agent to the output, ensuring no data loss occurs during streaming or analysis.

## Components

### Streamlogs

- **Command**: `agent stream-logs`
- **Functionality**: Streams logs being processed by a running agent.
- **Key Parameters**:
  - `FilePath`: Output file path for the log stream.
  - `Duration`: Time limit for streaming logs.
  - `Filters`: Options to filter logs by name, type, source, or service.

### Analyzelogs

- **Command**: `agent analyze-logs`
- **Functionality**: Analyzes logs configuration in isolation and outputs results.
- **Key Parameters**:
  - `LogConfigPath`: Path to the logs configuration file.
  - `CoreConfigPath`: Path to the core configuration file.
  - `InactivityTimeout`: Duration to wait for new logs before exiting.

## Related Components

- **Logs Agent**: The core component responsible for log processing and management.
- **IPC Module**: Facilitates communication between components, crucial for both commands.
- **File System Utilities**: Used for managing output file paths and ensuring directories exist.

## Failure Modes

- **Streamlogs**:
  - Failure to open output file due to permission issues or invalid paths.
  - Dropped logs if the message receiver is not enabled correctly.
  
- **Analyzelogs**:
  - Missing log configuration file leading to command failure.
  - Incorrect core configuration path causing unexpected behavior.

## Review Considerations

When reviewing changes related to these commands, ensure to check:

- Input-to-pipeline ordering guarantees.
- Reliability of the message receiver in `streamlogs`.
- Correct handling of configuration paths in `analyzelogs`.
- Potential impacts on user-facing behavior from internal refactors.

## References

- [Logs Agent Architecture](../architecture/)
- [Logs Agent Components](../components/)
- [Logs Agent Configurations](../configs/)
- [Logs Agent Playbooks](../playbooks/)

This page serves as a durable reference for understanding the adjacent interfaces of the logs-agent and their implications on overall system behavior.
