---
name: smp
description: >-
  Load the shared Single Machine Performance (SMP) skills for datadog-agent.
  Use for SMP questions, performance CI failures, CPU or memory regressions,
  throughput, profiles, experiments, and build or configuration comparisons.
metadata:
  owner: single-machine-performance
---

# Load SMP guidance

Owner: `@DataDog/single-machine-performance`.

This needs Mise, Git, and SSH access to `ddoghq/claude-marketplace` on the laptop or Workspace.
From the checkout root, refresh the shared skills once before the first SMP workflow in each session:

```sh
mise run dotagents install
```

If Mise reports an untrusted configuration, inspect `mise.toml`, then run `mise trust` for this checkout.
If setup fails, explain the error and stop the SMP workflow.

Read these installed files directly so the current session can continue without a skill index reload:

1. `.agents/skills/using-smp/SKILL.md` for capability fit and workflow selection.
2. `.agents/skills/investigating-datadog-agent-performance/SKILL.md` for Agent CI context.

Read the selected workflow under `.agents/skills/<skill-name>/SKILL.md` and resolve references relative to that file.
Follow the shared guidance for run limits. Install the SMP CLI only when the workflow needs it.
