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

SMP maintains the shared workflows in `ddoghq/claude-marketplace`.
Use the repository's existing `dotagents` setup to install and refresh them.
This needs [Mise](https://mise.jdx.dev/getting-started.html), Git, and SSH access to the marketplace on the laptop or Workspace.

From the `datadog-agent` checkout root, run this once before the first SMP workflow in each session:

```sh
mise run dotagents install
```

This installs the skills declared in `agents.toml` and refreshes them from the configured marketplace reference.
It also reconciles the repository's declared MCP servers. It does not install the SMP CLI.
If Mise reports an untrusted configuration, inspect `mise.toml`, then run `mise trust` for this checkout.
If setup fails, explain the error and stop the SMP workflow.

After setup succeeds, read these installed files directly so the current session can continue without a skill index reload:

1. `.agents/skills/using-smp/SKILL.md` for capability fit and workflow selection.
2. `.agents/skills/investigating-datadog-agent-performance/SKILL.md` for Agent CI context.

Read the selected workflow under `.agents/skills/<skill-name>/SKILL.md` and resolve its references relative to that file.
Follow the shared guidance for CLI setup and run limits.
Install the SMP CLI only when the selected workflow needs it.
