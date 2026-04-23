---
title: "Sender and Destination Semantics"
kind: "invariant"
summary: "Reliable destinations gate progress; unreliable ones do not block or update the auditor."
source_paths:
  - "pkg/logs/sender/worker.go"
  - "pkg/logs/client/destinations.go"
  - "pkg/logs/client/http/destination.go"
owns_globs:
  - "pkg/logs/sender/**"
  - "pkg/logs/client/**"
related_pages:
  - "components/sender.md"
  - "invariants/auditor-delivery.md"
  - "playbooks/review-checklist.md"
last_ingested_sha: "HEAD"
---

# Sender and Destination Semantics

Reliable and unreliable destinations are fundamental to the logs agent's architecture, each embodying distinct delivery guarantees that impact overall pipeline behavior.

## Overview

This page outlines the semantics of sender and destination interactions, emphasizing the critical differences between reliable and unreliable destinations. Understanding these distinctions is essential for reviewers to ensure the integrity of the logs agent's architecture.

## Hard Rules

- **Reliable Destinations**:
  - At least one reliable destination must successfully process a payload before the worker considers it sent.
  - Reliable destinations can apply backpressure to the pipeline if they encounter errors.

- **Unreliable Destinations**:
  - Unreliable destinations do not update the auditor upon failure.
  - Failure of an unreliable destination does not impede overall pipeline progress.
  - Unreliable destinations only send logs when at least one reliable destination is active.

## Review Traps

- **Code Intermingling**: 
  - Be cautious of code that merges the handling of reliable and unreliable paths for convenience. This can obscure the critical durability distinctions that the logs agent relies on.

## Related Pages

- [Sender and Destinations](../components/sender.md)
- [Auditor Delivery and Persistence](../components/auditor-delivery.md)

By adhering to these semantics, we can maintain the robustness of the logs agent's architecture and ensure reliable log delivery across various scenarios.
