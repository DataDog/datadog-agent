---
title: "Logs Agent Review Checklist"
kind: "playbook"
summary: "Reviewer checklist for architectural regressions in delivery guarantees, offsets, restarts, and launcher behavior."
source_paths:
  - "pkg/logs/README.md"
  - "comp/logs/agent/agentimpl/agent_restart.go"
  - "comp/logs/auditor/impl/auditor.go"
owns_globs:
  - "pkg/logs/**"
  - "comp/logs/**"
related_pages:
  - "invariants/pipeline-ordering.md"
  - "invariants/sender-destination-semantics.md"
  - "invariants/auditor-delivery.md"
  - "invariants/tailer-position-and-fingerprint.md"
  - "invariants/graceful-restart.md"
last_ingested_sha: "HEAD"
---

# Logs Agent Review Checklist

## Overview

This checklist serves as a guide for reviewers to identify potential architectural regressions in the logs-agent, particularly concerning delivery guarantees, offsets, restarts, and launcher behavior. It emphasizes an architecture-first approach to ensure the integrity of the logs-agent's functionality.

## Review Checklist

When reviewing a logs-agent PR, consider the following questions:

1. **Durability of Offsets**: 
   - Does this change affect when offsets become durable or how the auditor is flushed?
   
2. **Destination Reliability**: 
   - Does this change blur the line between reliable and unreliable destinations?

3. **Pipeline Movement**: 
   - Can one input now move between pipelines or have multiple concurrent delivery paths?

4. **Source Identity and Routing**: 
   - Does this change alter source identity, launcher routing, or file-vs-container selection?

5. **Restart Behavior**: 
   - Does restart still preserve persistent components and cancel old retry loops cleanly?

6. **File Recovery Consistency**: 
   - If file recovery is involved, do identifiers, fingerprints, and tailing modes still agree?

## Escalation Protocol

- **Escalate**: If the answer to any item is "maybe," escalate the review. These scenarios are most likely to lead to:
  - Duplicate logs
  - Skipped logs
  - Hard-to-debug restart regressions

## Related Pages

- [Logs Agent Architecture](../architecture/)
- [Logs Agent Components](../components/)
- [Logs Agent Invariants](../invariants/)
- [Logs Agent Configurations](../configs/)

This checklist is designed to ensure that architectural integrity is maintained throughout the review process, aligning with the overarching goals of the logs-agent architecture.
