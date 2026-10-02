# Regression Detection

The Regression Detector finds Agent performance changes with controlled experiments. The Single Machine Performance team owns this system.

Ask `#single-machine-performance` for help with these experiments.

## Experiment selection

Agent experiments use two lanes. Container experiments use the manifest in `container/selection.yaml`. Metal eBPF experiments use the separate `ebpf/` lane.

The CLI finds each directory under `container/` that contains an `experiment.yaml` file. Intermediate directories only group experiments.

The manifest maps trigger buckets to experiment path globs. It combines all matching buckets.

* The `always` bucket runs on every pull request. This selection does not control the nightly schedule.
* The `codeowners` bucket maps a team slug to experiments. The experiments run when the team owns a changed file.
* The `labels` bucket maps an exact label to experiments. The experiments run when the pull request has that label.

Each experiment must match one or more buckets. The resolve lint job rejects an experiment that matches no bucket.

Read [`experiment-selection-guide.md`](experiment-selection-guide.md) for addition and selection procedures.

## Add an experiment

Put each container experiment in a directory below `container/`. You can use intermediate directories to group related experiments.

Each experiment directory must contain `experiment.yaml` and `lading/lading.yaml`. It must also contain the `datadog-agent/` configuration directory.

The CLI mounts `datadog-agent/` at `/etc/datadog-agent` in the target container. An optional `README.md` can explain the experiment.

Experiment directory names must be unique across the container lane. The CLI compares names without case differences.

Add the experiment to `container/selection.yaml`. You can also put it below a path that an existing glob matches.

Manifest entries are path globs relative to `container/`. The `*` and `?` patterns stay within one path segment.

The `**` pattern spans path segments. `foo/**` matches experiments inside `foo/`, but it does not match `foo` itself.

Use `**/name` to match an experiment name at any depth. A plain name does not match by name.

The `optimization_goal` field selects the metric for analysis. Supported values include `ingress_throughput` and `egress_throughput`.

## Local run

Use the `smp` and `lading` tools for a local experiment. The Lading binary must support the image architecture.

For full instructions, read the [SMP local replicate documentation][smp-local].

For example:

```bash
smp local-run --experiment-dir ~/dev/datadog-agent/test/regression/container/ --case quality_gate_logs --target-image datadog/agent-dev:nightly-main-fe13dead-py3
```

[smp-local]: https://github.com/DataDog/single-machine-performance/blob/main/smp/README.md#running-replicates-locally
