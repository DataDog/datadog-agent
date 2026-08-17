# How to add and select SMP regression experiments

This guide is for **teams adding their own SMP regression experiments** and choosing when they run.
It is the practical how-to.

## The three ways an experiment runs

Which experiments run on a PR is decided by one central manifest, `selection.yaml`, which maps
**trigger buckets → experiments**. The buckets are *unioned* — an experiment runs if **any** of its
triggers fire.

| Bucket | Runs when | Who usually uses it |
|---|---|---|
| `always` | Every PR + scheduled runs, unconditionally | SMP (core quality gates) |
| `codeowners` | Automatically when **your team's** code changes on the PR | Teams wanting no-touch coverage |
| `labels` | The `smp/<label>` is applied to the PR (picked up by the next SMP job) | Teams wanting opt-in/on-demand suites |

An experiment may appear in several buckets. Every experiment **must** be placed in at least one
bucket before it can merge (the validate gate enforces this).

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

Experiments are discovered under **any** `cases/` directory at any depth, so
`logs/cases/…`, `logs/general/cases/…`, and `logs/syslog/cases/…` can all coexist.

---

## Recipe A — an opt-in experiment (label)

Use this when the suite should run **only on request** (someone applies a label to the PR).

1. **Create the experiment** under a `cases/` directory, e.g.
   `logs/syslog/cases/my_syslog_experiment/` with its `experiment.yaml`, `lading/lading.yaml`, and
   `datadog-agent/` config.

2. **Declare the label** in `selection.yaml` under `labels:` (keys keep the `smp/` prefix):

   ```yaml
   labels:
     smp/logs/syslog:
       description: Optional syslog experiment suite.
       experiments:
         - logs/syslog/**        # glob, exact path, or bare experiment name
   ```

3. **Create the GitHub label** so it can be applied. The `smp-label-sync` check will remind you with
   the exact command if you forget; it looks like:

   ```bash
   gh label create "smp/logs/syslog" --repo DataDog/datadog-agent --description "Optional syslog experiment suite."
   ```

   (A manifest label with no repo label is a *warning*, not a merge blocker — the trigger is just
   dormant until the label exists. After creating it, re-run the check.)

4. **Select it** by applying the `smp/logs/syslog` label to a PR. The label controls which experiments
   the **next** SMP regression job includes — it does **not** re-trigger a job on its own. Apply the
   label before the SMP job runs, or trigger a new run afterward (e.g. push a commit or re-run the
   pipeline) for it to be picked up.

---

## Recipe B — an ownership-driven experiment (codeowners)

Use this when the suite should run **automatically whenever your team's code changes** — no label
needed.

> [!IMPORTANT]
> **The critical step is the CODEOWNERS delegation.** The `codeowners` bucket runs an experiment when
> its *owning team* is involved in the PR (i.e. the PR touches files that team owns). Ownership comes
> from `.github/CODEOWNERS`, resolved by the experiment's folder. **If you do not add a CODEOWNERS rule
> for your experiment folder, it inherits the default `/test/regression/ → @DataDog/single-machine-performance`
> rule — and will then only run when *SMP-owned* files change, never on your team's normal PRs.**
> This fails *silently* (the experiment just never fires). See Troubleshooting.

1. **Create the experiment** under a `cases/` directory, e.g. `logs/general/cases/my_experiment/`.

2. **Delegate ownership to your team** in `.github/CODEOWNERS` (this is what makes "ownership-driven"
   mean *your* team). Ownership is resolved **per experiment**, so delegate at whatever granularity
   fits — a whole folder:

   ```
   /test/regression/logs/     @DataDog/agent-log-pipelines
   ```

   …or a single experiment, and experiments in the same folder may have different owners (e.g. a
   shared folder where one experiment is co-owned):

   ```
   /test/regression/logs/general/cases/my_experiment @DataDog/agent-log-pipelines @DataDog/some-team
   ```

   A `codeowners` experiment is then run whenever **any** of its owning teams has a file changed on
   the PR.

3. **Add the experiment to the `codeowners` bucket** in `selection.yaml`:

   ```yaml
   codeowners:
     - logs/general/**
   ```

4. Done — it now runs automatically on any PR that changes a file your team owns.

---

## Note — always-run experiments (quality gates)

The `always` bucket is the core, unconditional performance suite. Experiments under
`test/regression/quality_gates/` are **co-owned with SMP** and generally managed by SMP; talk to
#single-machine-performance before adding here. Everything else is fully team-owned.

---

## Verify locally before you push

Build/download the `smp` binary, then ask it what will run and *why*:

```bash
# What buckets does each experiment fall into? (selection breakdown per experiment)
smp experiments list --target-config-dir test/regression --manifest test/regression/selection.yaml --format json

# What would actually run for a given set of labels / involved teams?
smp experiments resolve --target-config-dir test/regression --manifest test/regression/selection.yaml \
  --runner container --format path-filter \
  --label smp/logs/syslog \
  --involved-team agent-log-pipelines --ownership ownership.json
```

The agent also wraps this end-to-end as `dda inv owners.smp-resolve` (builds involved-teams and the
ownership map from CODEOWNERS for you).

Every experiment must land in some bucket: `smp experiments validate --in-ci` **blocks merge** on any
experiment that matches no bucket.

---

## Troubleshooting

**"My `codeowners` experiment never runs when I change my code."**
Almost always the missing CODEOWNERS delegation (Recipe B, step 2). Without a rule covering your
experiment, it inherits SMP ownership and only fires on SMP-file PRs. Ownership is resolved at the
experiment's own path, so check exactly that:
```bash
dda inv owners.find-codeowners --path test/regression/<area>/cases/<experiment>/
```
If that prints only `@DataDog/single-machine-performance`, add your team's rule (folder- or
experiment-level).

**"My label doesn't do anything when applied."**
First, a label only changes selection for the **next** SMP job — applying it does not re-run a job that
already ran. Trigger a fresh run (push a commit or re-run the pipeline). If it still doesn't take
effect, check both halves of the registry: the label must be a **key under `labels:`** in
`selection.yaml` *and* exist as a repo label (`gh label list | grep smp/`). The `smp-label-sync` check
reports either gap.

**"Two labels / a multi-team PR — which experiments run?"**
The union of all fired triggers. Multiple applied labels each contribute their experiments; a PR
touching several teams' code involves all of them.

**"I added an experiment and CI says it's 'in no manifest bucket'."**
That's the merge gate: assign it to a bucket in `selection.yaml` (or ensure it falls under an existing
glob such as `logs/**`).
