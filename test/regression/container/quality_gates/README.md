---
description: Core Agent performance regression quality gates.
---

# Quality gates

These experiments are the core Agent performance quality gates. The `always` bucket runs them on every pull request and every `main` commit.

The nightly SMP schedule finds and runs `quality_gate_*` experiments automatically. SMP reserves this prefix for experiments in this directory. Agent CI also uses it to apply the quality-gate pass or fail decision.

New quality gates must have a CODEOWNERS entry for their owning team. Datadog employees can read the internal [Performance Quality Gates guide][quality-gates-guide] for the ownership, promotion, incident-response, and limit-adjustment contract. Ask `#single-machine-performance` for help.

[quality-gates-guide]: https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4294836779/Performance+Quality+Gates
