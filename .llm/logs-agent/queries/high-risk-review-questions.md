---
title: "High-Risk Review Questions"
kind: "query"
summary: "Saved set of questions the bot should ask itself when reviewing logs-agent pull requests."
source_paths:
  - "pkg/logs/README.md"
  - "comp/logs/auditor/impl/auditor.go"
  - "pkg/logs/sender/worker.go"
owns_globs:
  - "pkg/logs/**"
  - "comp/logs/**"
related_pages:
  - "playbooks/review-checklist.md"
  - "invariants/auditor-delivery.md"
  - "invariants/graceful-restart.md"
last_ingested_sha: "5615108532614341fbd4a15ad37e7020b7250149"
---

Reusable self-checks for the review bot:

- Could this diff cause the auditor to advance before a reliable destination truly succeeded?
- Could this diff cause a restart to lose in-memory acknowledgements or rebuild around stale state?
- Could this diff cause the same physical log stream to be identified differently after restart or rotation?
- Could this diff turn a source/service timing issue into duplicate tailers or missed tailers?
- Could this diff weaken per-input ordering by changing how pipelines are assigned or reused?

If none of these questions produce a concrete risk, the bot should prefer silence over generic feedback.
