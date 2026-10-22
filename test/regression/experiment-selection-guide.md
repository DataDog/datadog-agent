# Add and select SMP regression experiments

This guide explains how to add an SMP regression experiment and choose when it runs.

## Selection buckets

The `container/selection.yaml` manifest controls most Agent experiments. The separate `ebpf/` lane does not use this manifest.

The manifest has three optional sections. The CLI combines the experiments from all matching sections.

| Bucket | Pull request behavior | Main branch behavior |
|---|---|---|
| `always` | Selected for every pull request | Selected for every `main` commit |
| `codeowners` | Selected when a configured team owns a changed file | Not selected |
| `labels` | Selected when the pull request has a configured label | Not selected |

Each experiment under `container/` must match one or more manifest entries. The resolve lint job rejects an experiment that matches no entry.

The resolver combines all matching buckets. On `main`, no pull request context is available, so the selected set contains only `always` experiments. The nightly quality-gate schedule is separate from this manifest.

## Manifest format

```yaml
always:
  - quality_gates/**
codeowners:
  agent-log-pipelines:
    - logs/general/**
labels:
  smp/logs/syslog:
    - logs/syslog/**
```

The `always` section contains a list. The `codeowners` section maps team slugs to lists. The `labels` section maps labels to lists.

Entry values are path globs relative to the `container/` directory. The `*` and `?` patterns stay within one segment.

The `**` pattern spans segments. `foo/**` matches experiments inside `foo/`, but it does not match `foo` itself.

Use `**/name` to match a name at any depth. A plain name does not match by name.

## Experiment locations

```text
test/regression/
  container/
    selection.yaml
    config.yaml
    <group>/
      <experiment-name>/
        experiment.yaml
        lading/lading.yaml
        datadog-agent/
      README.md
  ebpf/
```

The CLI finds each directory below `container/` that contains `experiment.yaml`. Intermediate directories only group experiments.

The directory name is the experiment name. Names must be unique without case differences within `container/`.

## Set the optimization goal

The `optimization_goal` field selects the metric used to compare the baseline and comparison builds. Use one of these values:

* `cpu`
* `memory`
* `ingress_throughput`
* `egress_throughput`
* `none`, which disables comparative analysis

## Add a label experiment

Use a label when experiments must run only after a person selects them.

1. Create the experiment below `container/`.
2. Add its path glob below a label in the `labels` section.
3. Create the repository label if it does not exist.
4. Apply the label before the SMP job starts.

For example:

```yaml
labels:
  smp/logs/syslog:
    - logs/syslog/**
```

Create the label once:

```bash
gh label create "smp/logs/syslog" --repo DataDog/datadog-agent
```

The manifest key must equal the repository label. Matching is case-sensitive.

A label change does not start a new SMP job. Restart the pipeline after you apply the label to an existing pull request. Pushing a new commit will also create a new pipeline.

## Add a CODEOWNERS experiment

Use code ownership when experiments must run after changes to team-owned code.

1. Create the experiment below `container/`.
2. Add its path glob below a team in the `codeowners` section.
3. Use the lowercase team slug without the `@DataDog/` prefix.

For example:

```yaml
codeowners:
  agent-log-pipelines:
    - logs/general/**
```

CI compares changed files with `.github/CODEOWNERS`.

The CLI compares the emitted slug and manifest key exactly. A case difference or spelling error prevents selection.

Experiment folder ownership controls reviews only. It does not select `codeowners` experiments. Add the team slug and experiment glob to the manifest to run the experiment for changes owned by that team.

## The always bucket

The `always` bucket runs on every pull request and every `main` commit. This selection does not control the nightly SMP schedule.

SMP reserves the `quality_gate_` prefix for designated quality gates. A quality gate must live below `container/quality_gates/`, match the `always` bucket, and have a CODEOWNERS entry for its owning team.

Agent CI uses the prefix to apply the quality gate result. Do not use it for a label-triggered, CODEOWNERS-triggered, or other non-gate experiment.

Nightly confidence runs automatically include quality gates. Ask `#single-machine-performance` about the ownership, promotion, incident-response, and limit-adjustment contract before adding a new gate.

## Verify the selection

Install the SMP CLI if needed:

```bash
dogbrew install smp
```

Run resolve before you submit a change. Resolve validates the manifest and experiment tree.

```bash
smp experiments resolve \
  --target-config-dir test/regression/container \
  --manifest test/regression/container/selection.yaml
```

Use selection inputs to preview conditional experiments:

```bash
smp experiments resolve \
  --target-config-dir test/regression/container \
  --manifest test/regression/container/selection.yaml \
  --involved-team agent-log-pipelines \
  --label smp/logs/syslog
```

The command returns a nonzero status for duplicate names. It also rejects stale globs and experiments under `container/` without a bucket.

An experiment may match more than one manifest entry. The resolver selects it once and does not submit duplicate copies. Prefer the simplest configuration that expresses when the experiment should run.

## Troubleshooting

If a codeowners experiment does not run, check the team slug. Then show the owners for a changed file:

```bash
dda inv owners.find-codeowners --path <changed-file>
```

If a label experiment does not run, check the manifest key and repository label. Apply the label before the next SMP job.

The CLI combines all selected experiments for multiple teams or labels.
