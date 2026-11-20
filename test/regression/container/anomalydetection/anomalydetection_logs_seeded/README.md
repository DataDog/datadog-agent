# Seeded anomaly-detection container logs

This logs-only experiment runs when `agent-anomaly-detection` owns a changed
pull request file, through `container/selection.yaml`. It is outside the
`always` bucket and is not selected on `main`. The ordinary SMP experiments
still run alongside it.

The source is the [seeded logs case linked by AAD-92][source]. Both Agent
variants receive the same workload and Observer configuration. There is no
metric load generator, and external Observer metric ingestion is disabled.
The normal logs pipeline is disabled; Observer reads container stdout through
its independent container source.

## Workload

One Python container builds 3,000 unique base patterns with seed `20260928`.
Each pattern contains 10–40 words of 3–10 lowercase letters. Each log selects
one base pattern and changes exactly three letters. The first 300 patterns
each have ten times the selection weight of the remaining patterns, accounting
for about 52.6% of emitted logs.

The generator emits at least 100 KiB every 100 ms, targeting about 1,000 KiB/s
before container framing. It uses a versioned Python image. Observer accepts
`info` logs with a medium-priority cap of 10,000 logs/s. Kubelet and internal
log sources are disabled to isolate the container workload.

The detector settings preserve the playground case, with baseline analysis
disabled and 30 seconds of point retention. This measures that fixed workload
and configuration rather than the production defaults.

The seed repeats the payload sequence, not the exact number of logs accepted
over a timed run. Scheduling, Docker metadata, rate limiting, and ingestion
can still differ between replicates. Generated base-pattern cardinality also
does not guarantee the same number of learned Observer clusters.

## Validation

Preview selection from the repository root:

```sh
smp experiments resolve \
  --target-config-dir test/regression/container \
  --manifest test/regression/container/selection.yaml \
  --involved-team agent-anomaly-detection
```

Repeat without `--involved-team` and with an unrelated team: this experiment
must be absent. With AAD and another team, each experiment must appear once.

Before interpreting a remote performance comparison, check that Observer
starts, accepts container logs at comparable rates, and has no channel-full
or sampler drops. Compare learned pattern counts as well as CPU and memory.
An identical-image comparison establishes the run-to-run noise floor.

Memory is the comparative optimization goal; CPU and memory bounds are also
reported. The bounds are inherited from the playground experiment and need
calibration on current Agent builds. This is not a `quality_gate_*`
experiment, so its bounds do not block Agent CI under the current policy.

[source]: https://github.com/ddoghq/smp-playground/tree/55bc8fadabba668143efbc2391b57544c2a1275e/experiments/regression/agent/cases/observer_logs_seeded
