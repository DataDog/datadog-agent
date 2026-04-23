---
title: "Auditor Component"
kind: "component"
summary: "Registry-backed persistence for offsets, tailing state, fingerprints, and restart safety."
source_paths:
  - "comp/logs/auditor/def/component.go"
  - "comp/logs/auditor/def/types.go"
  - "comp/logs/auditor/impl/auditor.go"
  - "comp/logs/auditor/impl/registry_writer.go"
owns_globs:
  - "comp/logs/auditor/**"
related_pages:
  - "invariants/auditor-delivery.md"
  - "invariants/tailer-position-and-fingerprint.md"
  - "components/restart-lifecycle.md"
last_ingested_sha: "HEAD"
---

The auditor component serves as the durable progress ledger for the logs-agent delivery system. It is responsible for maintaining offsets, tailing states, fingerprints, and ensuring restart safety through a registry file located at `logs_config.run_path`.

## Core Responsibilities

- **Acknowledge Payloads**: Accepts successful payload acknowledgements via `Channel()`.
- **Update Registry**: Updates in-memory registry entries without reverting to older offsets.
- **Periodic Flushing**: Flushes registry state to disk periodically and upon explicit `Flush()` calls.
- **Maintain Liveness**: Preserves tailed-source liveness using `KeepAlive()` and `SetTailed()`.
- **State Recovery**: Recovers registry state on startup, including handling versioned migrations.

## Why Reviewers Care

- **Downstream Dependency**: The auditor operates downstream of successful delivery, meaning it relies on prior acknowledgements.
- **Partial Restart Risks**: A partial restart requires `Flush()` to mitigate duplicate replay risks.
- **Registry Cleanup**: Uses TTL and tailed-state tracking; lifecycle changes can inadvertently drop offsets prematurely.
- **File Position Recovery**: Relies on fingerprints aligning with stored state for accurate recovery.

## Specific Risks

- **Channel Disconnection**: Making `Channel()` nil or disconnected while reliable destinations still report success can lead to data loss.
- **Flush Semantics Changes**: Altering `Flush()` or shutdown semantics may prevent in-memory acknowledgements from reaching disk.
- **Offset Update Ordering**: Changing the order of offset updates can allow stale values to overwrite newer ones.
- **Write Behavior Changes**: Modifying atomic versus non-atomic write behavior without considering crash recovery can introduce risks.

## Invariants

- **Durability**: Offsets must be durable and recoverable after restarts.
- **Ordering Guarantees**: Ensure that offsets are updated in a consistent order to prevent data loss.
- **Flush Consistency**: Flushing must be atomic to avoid partial writes that could lead to inconsistencies.

For further details, see related pages on [Auditor Delivery and Persistence](../invariants/auditor-delivery.md), [Tailer Position and Fingerprint Recovery](../invariants/tailer-position-and-fingerprint.md), and [Restart Lifecycle](restart-lifecycle.md).
