---
name: gensim-episode-management
description: >
  Manage gensim episodes on the gensim-eks EKS runner for observer anomaly detection
  evaluation. Use this skill when the user wants to: add/run a gensim episode on EKS,
  validate episode compatibility, debug a failed gensim-eks submit or run, check run
  results, or troubleshoot Pulumi/helm/kubectl issues with the gensim cluster. Trigger
  on mentions of gensim-eks, gensim submit, episode validation, observer evaluation,
  or gensim infrastructure problems -- even if the user doesn't use these exact terms.
---

# Gensim Episode Management

## When to use which sub-skill

**Read `add-episode.md`** when the user wants to:
- Run a new episode on gensim-eks for the first time
- Validate an episode before submitting
- Check results after a run completes (monitors, events, TP/FP metrics)
- Batch-run multiple episodes

**Read `debug-infra.md`** when the user is hitting:
- Pulumi submit failures (state conflicts, stack locks, resource errors)
- Helm install timeouts or conflicts
- kubectl/auth connectivity issues
- ImagePullBackOff, orchestrator Error state, or pod scheduling problems

If unclear, start with the user's immediate problem. Infrastructure issues block
everything else, so debug-infra takes priority when things are broken.

## Shared Context

### Repository Layout

- **datadog-agent repo** (`alt-datadog-agent/`): contains the gensim-eks runner code
  - `test/e2e-framework/scenarios/aws/gensim-eks/run.go` -- Pulumi program
  - `test/e2e-framework/scenarios/aws/gensim-eks/orchestrator.sh.tmpl` -- orchestrator script
  - `test/e2e-framework/scenarios/aws/gensim-eks/agent-values.yaml.tmpl` -- observer config
  - `tasks/e2e_framework/aws/gensim_eks.py` -- `dda inv aws.eks.gensim.submit` task

- **gensim-episodes repo** (sibling dir or `GENSIM_REPO_PATH`): episode definitions
  - `synthetics/` -- hand-crafted scenarios (food-delivery, casino, ehr, timescaledb)
  - `postmortems/` -- scenarios derived from real incident postmortems

### Episode Anatomy

```
episode-name/
├── play-episode.sh          # Phase functions sourced by orchestrator
├── chart/                   # Helm chart for episode services
├── docker-compose.yaml      # Service image definitions (triggers build VM)
├── services/                # Source code for microservices
├── episodes/*.yaml          # Scenario definitions (phases, durations)
└── scenario.yaml            # Ground truth: true_positives, false_positives
```

### Observer Config (agent-values.yaml.tmpl)

The observer's detector/correlator config lives in `agent-values.yaml.tmpl`. Each
detector and correlator can be toggled via:

```
observer.components.<name>.enabled: true/false
```

Check the template for current detector names and defaults. For eval runs, ensure
`time_cluster.min_cluster_size` is low enough (1-2) -- higher values filter out
small anomaly clusters that are common in single-disruption scenarios.

### Known Issues (affect all episodes)

**Trace metric naming mismatch** (structural, unlikely to change): The observer's
`processStatsView` stores `trace.hits{operation:redis.command}` while Datadog's
backend shows `trace.redis.command.hits`. Same data, different naming convention.
When validating TP/FP against scenario.yaml, translate: `trace.{operation}.{suffix}`
in the backend maps to `trace.{suffix}{operation:{operation}}` in the observer.

**Detector-specific issues**: See `references/current-detector-issues.md` for
current detector warmup requirements, correlator eviction behavior, and known
live-vs-testbench divergences. These change as the observer algorithms evolve.

### Useful pup Commands

```bash
pup events search --query 'source:agent-q-branch-observer' --from 1h
pup metrics query --query 'avg:<metric>{env:<env-tag>}' --from '<start>' --to '<end>'
pup monitors list --name 'gensim'
pup logs search --query '"[observer]" cluster_name:gensim' --from 1h --limit 50
```

Note: `pup` auth expires frequently. Re-auth with `pup auth login`.
