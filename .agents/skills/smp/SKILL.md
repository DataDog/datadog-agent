---
name: smp
description: >-
  Loads the shared Single Machine Performance (SMP) skills for datadog-agent.
  Use for SMP questions, performance CI failures, CPU or memory regressions,
  throughput changes, profiles, experiment design, and build or configuration comparisons.
metadata:
  owner: single-machine-performance
---

# Load SMP guidance

Owner: `@DataDog/single-machine-performance`.

SMP owns the workflow instructions in `ddoghq/claude-marketplace`.
This entry skill retrieves them for the current session, across agent harnesses.
It needs Git, Python 3, and SSH access to that repository in the environment that runs the agent.

## Load the shared skills

From the `datadog-agent` checkout root, run:

```sh
python3 .agents/skills/smp/scripts/fetch_skills.py
```

Run this once at the start of an SMP workflow, then reuse the returned paths for the session.
The helper fetches `main` into the ignored `.cache/smp-skills/` directory.
It retrieves skill definitions, not the SMP CLI, and does not change harness configuration.

Read these files from the returned plugin paths:

1. Under `common`, read `skills/using-smp/SKILL.md` to check capability fit and select a workflow.
2. Under `datadog_agent`, read `skills/investigating-datadog-agent-performance/SKILL.md` for Agent CI context.
3. Load the shared skill that matches the task.

When a shared skill names another skill, find its `SKILL.md` under either returned plugin's `skills/` directory.
Resolve each skill's relative reference links from that skill's directory.
Read only the instructions and references needed for the request.
Use the printed revision when identifying which guidance the session loaded.

If retrieval fails, explain the access or source error and stop the SMP workflow.
Do not claim that the shared skills loaded, install unrelated tools, or invent missing SMP capabilities.
Follow the shared skills for CLI setup and run limits.
Loading this entry skill does not authorize a job submission.

## Preview a marketplace change

If the user asks to test a specific marketplace branch or tag, pass it with `--ref`:

```sh
python3 .agents/skills/smp/scripts/fetch_skills.py --ref <marketplace-ref>
```

Use the paths from that invocation for the session.
Do not select an unmerged branch without the user's request.
