# How to add and select SMP regression experiments

This guide is for **teams adding their own SMP regression experiments** and choosing when they run.
It is the practical how-to.

## The three ways an experiment runs

Which experiments run on a PR is decided by one central manifest, `selection.yaml`, which maps
**trigger buckets → experiments**. The buckets are *unioned* — an experiment runs if **any** of its
triggers fire.

| Bucket | Runs when |
|---|---|
| `always` | Every PR + scheduled runs, unconditionally (the core quality gates) |
| `codeowners` | A team you name has code changed on the PR (its experiments run automatically) |
| `labels` | The `smp/<label>` you name is applied to the PR |

Every experiment **must** be placed in at least one bucket — an unbucketed experiment fails the
`resolve` lint gate, so it can't merge. There is no implicit default.

## Manifest shape

```yaml
always:                       # flat list — unconditional
  - quality_gates/**
codeowners:                   # team slug -> experiments that run when that team is involved
  agent-log-pipelines:
    - logs/general/**
labels:                       # smp/<label> -> experiments that run when the label is applied
  smp/logs/syslog:
    - logs/syslog/**
```

Entry values are **globs** (`*` = one path segment, `**` = any depth), an **exact path**, or a **bare
experiment name** (globally unique). Because experiments live under a `cases/` dir, "all logs
experiments" is `logs/**`.

## Where things live

```
test/regression/
  selection.yaml                         # the selection manifest (SMP-owned)
  <your-area>/                           # organize however you like; any depth is fine
    cases/
      <experiment-name>/                 # one directory per experiment; name is globally unique
        experiment.yaml                  # optimization goal, target config, runner, ...
        lading/lading.yaml               # load profile
        datadog-agent/...                # agent config mounted into the target
    README.md                            # optional prose docs (no selection metadata)
```

Experiments are discovered under **any** `cases/` directory at any depth.

---

## Recipe A — an opt-in experiment (label)

Use this when the suite should run **only on request**.

1. **Create the experiment** under a `cases/` directory, e.g. `logs/syslog/cases/my_syslog_experiment/`.

2. **Add it to the `labels` bucket** in `selection.yaml` (keys keep the `smp/` prefix):

   ```yaml
   labels:
     smp/logs/syslog:
       - logs/syslog/**
   ```

3. **Create the GitHub label** so it can be applied (this is a manual, one-time step):

   ```bash
   gh label create "smp/logs/syslog" --repo DataDog/datadog-agent
   ```

4. **Select it** by applying the `smp/logs/syslog` label to a PR. The label controls which experiments
   the **next** SMP job includes — it does **not** re-trigger a job on its own, so apply it before the
   run, or push a commit / re-run the pipeline afterward.

---

## Recipe B — an ownership-driven experiment (codeowners)

Use this when the suite should run **automatically whenever your team's code changes** — no label.

1. **Create the experiment** under a `cases/` directory, e.g. `logs/general/cases/my_experiment/`.

2. **Add your team to the `codeowners` bucket** in `selection.yaml`, mapping it to your experiments:

   ```yaml
   codeowners:
     agent-log-pipelines:
       - logs/general/**
   ```

   The team you name is the one whose code, when changed on a PR, should trigger these experiments —
   use your CODEOWNERS team slug (bare, e.g. `agent-log-pipelines`). "Involved" is computed from the
   PR's changed files ∩ `.github/CODEOWNERS`, so this fires whenever your team owns a changed file
   (typically your product code, e.g. `pkg/logs/**`). Co-ownership = list the experiment under each
   team that should trigger it.

3. Done — it runs automatically on any PR that changes your team's code.

> Note: the trigger team is what you *declare here*, not what owns the experiment folder — so there's
> no folder-delegation footgun. You may still want a `.github/CODEOWNERS` rule over your experiment
> folder so your team **reviews** changes to it, but that's independent of selection.

---

## Note — always-run experiments (quality gates)

The `always` bucket is the core, unconditional performance suite. Experiments under
`test/regression/quality_gates/` are co-owned with SMP and generally managed by SMP; talk to
#single-machine-performance before adding here. Everything else is fully team-owned.

---

## Verify locally before you push

Build/download the `smp` binary, then run `resolve` — it validates your manifest + experiment
structure and prints what would run:

```bash
# Validate + show the baseline (always) set:
smp experiments resolve --target-config-dir test/regression --manifest test/regression/selection.yaml

# Preview what a given team-involvement / label would run:
smp experiments resolve --target-config-dir test/regression --manifest test/regression/selection.yaml \
  --involved-team agent-log-pipelines --label smp/logs/syslog
```

`resolve` exits non-zero on any validation problem (duplicate names, entries matching nothing, an
experiment in no bucket), so it's both your local check and the CI lint gate.

---

## Troubleshooting

**"My `codeowners` experiment never runs when I change my code."**
Check the team slug you put in the `codeowners` bucket matches your CODEOWNERS team exactly — a typo'd
team never matches involvement and the experiment stays dormant. Confirm which teams a change maps to:
```bash
dda inv owners.find-codeowners --path <a file your PR changes>
```

**"My label doesn't do anything when applied."**
Two things must be true: the label is a **key under `labels:`** in `selection.yaml`, **and** it exists
as a repo label (`gh label list | grep smp/`). Label creation is manual — create it with `gh label
create` if you haven't. Also remember a label only affects the **next** SMP job.

**"Two labels / a multi-team PR — which experiments run?"**
The union of all fired triggers.

**"CI says my experiment is 'in no bucket'."**
Every experiment must be in a bucket to merge — add it to `always`, `codeowners`, or `labels` (or
ensure it falls under an existing glob such as `logs/**`).
