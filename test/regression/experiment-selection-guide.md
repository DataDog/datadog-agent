# Add and select SMP regression experiments

This guide explains how teams add SMP regression experiments. It also explains how teams select when experiments run.

## Selection buckets

The central `container/selection.yaml` manifest controls the container lane. The separate `ebpf/` lane does not use this manifest.

The manifest has three optional sections. The CLI combines the experiments from all matching sections.

| Bucket | Condition |
|---|---|
| `always` | Every pull request |
| `codeowners` | A listed team owns a changed file |
| `labels` | The pull request has a listed label |

Each experiment must match one or more manifest entries. The resolve lint job rejects an experiment that matches no entry.

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

The directory name is the experiment name. Names must be unique without case differences across the container lane.

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

The manifest key must equal the repository label. Matching is case-sensitive, and the CLI does not change the label.

A label change does not start a new SMP job. Restart the pipeline after you apply the label to an existing pull request.

## Add a codeowners experiment

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

CI compares changed files with `.github/CODEOWNERS`. The wrapper changes owner names to lowercase team slugs.

The CLI compares the emitted slug and manifest key exactly. A case difference or spelling error prevents selection.

Experiment folder ownership controls reviews only. It does not select codeowners experiments.

## The always bucket

The `always` bucket runs on every pull request. This selection does not control the nightly SMP schedule.

SMP manages the quality gates below `container/quality_gates/`. Ask `#single-machine-performance` before you add a quality gate.

## Verify the selection

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

The command returns a nonzero status for duplicate names. It also rejects stale globs and experiments without a bucket.

## Troubleshooting

If a codeowners experiment does not run, check the team slug. Then show the owners for a changed file:

```bash
dda inv owners.find-codeowners --path <changed-file>
```

If a label experiment does not run, check the manifest key and repository label. Apply the label before the next SMP job.

The CLI combines all selected experiments for multiple teams or labels.
