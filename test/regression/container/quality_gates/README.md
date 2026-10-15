---
description: Core Agent performance regression quality gates.
---

# Quality gates

These experiments are the core Agent performance quality gates. The `always` bucket runs them on every pull request and every `main` commit.

The nightly SMP schedule discovers and runs `quality_gate_*` experiments automatically. The prefix is reserved for experiments in this directory because Agent CI also uses it when applying the quality-gate pass or fail decision.

New quality gates must have a CODEOWNERS entry for their owning team. Ask `#single-machine-performance` about the ownership, promotion, incident-response, and limit-adjustment contract before adding one.
