# On-Demand 1 Hz Metrics

## Summary

Agent Metric Lookback collects selected metrics at high granularity, keeps them in bounded process memory, and sends them only when a configured signal identifies a relevant window. Use it to inspect fine-grained context around short-lived changes. The normal metric stream is unchanged.

```text
normal check  -> configured interval -> normal sender -----------------> intake
shadow check  -> lookback interval   -> lookback sender -> lookback ring
                                                            -> monitor-gated intake
```

- Each selected check runs independently as a `:shadow` instance.
- The monitor selects which retained windows to send and includes configured context before and after the change.

Locally retained data is lost when the Core Agent stops or restarts.

## Configuration

Metric lookback is **disabled by default**. The settings are under `metric_lookback` in `datadog.yaml`.

| Setting | Default | Description |
|---|---:|---|
| `enabled` | `false` | Enables metric lookback globally. |
| `enabled_checks` | `[]` | Exact check-name allowlist for shadow checks. An empty list allows every supported check. |
| `collection_interval` | `1s` | Shadow check interval. Values below 1 second are raised to 1 second. |
| `capacity` | `262144` | Records retained across the shards of each allocated ring. |
| `shard_count` | `16` | Number of independent shards in each lookback ring. |
| `dogstatsd.metric_names` | `[]` | Additional exact DogStatsD metric names to retain. The active monitor metric is admitted automatically. |
| `monitor.mode` | `disabled` | Monitor behavior: `disabled`, `dry_run`, or `enabled`. |
| `monitor.metric_name` | `""` | Exact DogStatsD metric name watched by the monitor. |
| `monitor.evaluation_interval` | `30s` | Approximate width of each monitor evaluation window. |
| `monitor.range_epsilon` | `0` | Maximum healthy `max - min` range. A negative value prevents the Agent from starting. |
| `monitor.partition_tags` | `[]` | Tag keys used to evaluate independent monitor partitions. An empty list evaluates all matching points together. |
| `egress.pre_trigger_window` | `0s` | Additional retained time forwarded before a breach or unknown window. |
| `egress.post_recovery_window` | `30s` | Additional retained time forwarded after the first healthy window suppresses egress. |

The settings bind to corresponding `DD_METRIC_LOOKBACK_*` environment variables.

### Monitor modes

| Mode | Retention | Monitor evaluation | Forwarding |
|---|---:|---:|---|
| `disabled` | Yes | No | No monitor-driven egress |
| `dry_run` | Yes | Yes | Continuous from startup; decisions do not gate egress |
| `enabled` | Yes | Yes | Controlled by monitor decisions |

## Suggested configuration for GPU Monitoring

This example runs the GPU check at 1 Hz and uses the workload's MFU metric as the monitor signal. It groups points by training job and rank and includes 30 seconds before a trigger and after recovery.

```yaml
gpu:
  enabled: true

metric_lookback:
  enabled: true
  enabled_checks:
    - gpu
  collection_interval: 1s
  capacity: 262144
  shard_count: 16

  monitor:
    mode: enabled
    metric_name: <mfu_metric_name>
    evaluation_interval: 30s
    range_epsilon: 0.005
    partition_tags:
      - <training_job_tag>
      - rank

  egress:
    pre_trigger_window: 30s
    post_recovery_window: 30s
```

Replace the placeholders as follows:

- `<mfu_metric_name>`: the workload's DogStatsD MFU metric;
- `<training_job_tag>`: the tag identifying a job or experiment.

The Agent does not provide a standard MFU metric. The monitored metric is retained automatically and need not appear under `dogstatsd.metric_names`.

The trigger metric, partition tags, range epsilon, and context windows are workload-specific. In particular:

- the MFU metric should arrive throughout the training job and remain comparatively flat during healthy execution;
- `partition_tags` should define the jobs, workers, or ranks used for per-partition range calculations;
- `range_epsilon` should be above normal within-job variation because MFU baselines can differ substantially between models;
- `evaluation_interval` should match the timescale of meaningful workload changes; and
- the pre-trigger and post-recovery windows should cover the context required for analysis.

Keep `enabled_checks` set explicitly to `gpu` unless broader check coverage is intentional. An empty list is unrestricted and must not be used to mean "no checks."

### Deployment test observations

The [test deployment notebook](https://ddstaging.datadoghq.com/notebook/15061931) compared stable one-hour control and feature-enabled periods, excluding deployment spikes. The feature-enabled deployment ran GPU and several core shadow checks at 1 Hz:

| Anecdotal observation | Result |
|---|---:|
| Agent container CPU | 0.24 to 0.59 cores; +0.34 cores / +142% |
| Agent RSS | 337 to 347 MiB; +10 MiB / +3% |
| Agent Python heap | 59.76 to 59.78 MiB; effectively unchanged |
| Local retention | The default 262,144-record buffer filled in about 3–4 minutes; five sampled Agents retained 1.5–3.2 minutes of recent data once full |

These anecdotes are deployment-specific: Agent population, images, and workload differed. They do not isolate Metric Lookback overhead or provide GPU sizing guidance. GPU shadow runs also add NVML and system-probe requests.

A later GPU validation flush covered approximately 31 seconds and contained up to 116,643 series. Payload size varies with enabled checks, instances, metric contexts, and window length.

### GPU metric type limitations

- `gpu.nvlink.errors.fec` uses histogram buckets and is not retained.
- NCCL collective metrics use the shadow sender's unsupported `Distribution` method and are not retained.

## Verify metric lookback operation

Use backend telemetry and Agent status to confirm that shadow checks are collecting points, the monitor is evaluating its signal, and forwarding follows the monitor state.

### Forwarding behavior

For each evaluation window, the monitor selects the largest `max - min` range among partitions with at least two valid points.

| Result | Condition | Forwarding behavior |
|---|---|---|
| `healthy` | Selected range is at or below epsilon | Suppressed after `egress.post_recovery_window` |
| `breach` | Selected range is above epsilon | Opened |
| `unknown` | No partition has two valid points | Opened as a fail-safe |

- Insufficient partitions are ignored when another partition has enough points.
- Missing partition tags are grouped under `<missing>`.
- A healthy result does not confirm that every expected partition reported data.
- Points must be at least 30 seconds old before forwarding; the egress loop runs approximately every 10 seconds.

### Backend telemetry

When the deployment's Agent Telemetry profile exports metric lookback telemetry, its backend metrics use the `datadog.agent.metric_lookback.*` namespace. Scope these metrics to the Agent host or pod running the selected check.

| Backend metric | Expected signal |
|---|---|
| `datadog.agent.metric_lookback.writer_append_samples` | `check_name:<check>,state:ok` increases as a shadow check writes samples. `state:error` should remain zero. |
| `datadog.agent.metric_lookback.unsupported_payloads_dropped` | Increases only for known unsupported sender methods. |
| `datadog.agent.metric_lookback.ring_records` | Greater than zero after collection starts and no greater than `ring_capacity`. |
| `datadog.agent.metric_lookback.ring_capacity` | Matches the configured scalar ring capacity. |
| `datadog.agent.metric_lookback.ring_overwritten_samples` | Shows how quickly older scalar records are being replaced. |
| `datadog.agent.metric_lookback.monitor_evaluations` | Reports `result:healthy`, `result:breach`, or `result:unknown` for completed windows. |
| `datadog.agent.metric_lookback.monitor_window_range` | Shows the selected partition's most recent `max - min` range. |
| `datadog.agent.metric_lookback.monitor_range_epsilon` | Matches the configured breach threshold. |
| `datadog.agent.metric_lookback.egress_mode` | Indicates whether egress is `suppressed` or `forwarding`. |
| `datadog.agent.metric_lookback.egress_transitions` | Increases when monitor decisions change the egress mode. |
| `datadog.agent.metric_lookback.egress_runs` | Reports forwarding attempts and their state. |
| `datadog.agent.metric_lookback.egress_ranges` | Reports the ranges selected for forwarding and their state. |

Standard `datadog.agent.checks.*` metrics do not distinguish a normal check from its shadow instance; use the metric lookback namespace for feature health.

### Agent status

Run `sudo datadog-agent status` and check the **Running Checks** section. A shadow instance appears under its source check with an instance ID ending in `:shadow`:

```text
Running Checks
==============

  <check_name>
  ------------
    Instance ID: <instance_id> [OK]
    ...

    Instance ID: <instance_id>:shadow [OK]
    Total Runs: <count>
    ...
```

The shadow row should be `[OK]`, and `Total Runs` should increase at the configured `collection_interval`.

## Runtime behavior and limits

### Shadow execution

- Shadow checks rerun the full check at `collection_interval`; they do not reuse normal results.
- At a 1-second shadow interval and 15-second normal interval, the check runs 16 times per 15 seconds: one normal run and 15 shadow runs.
- This is 16 times the check execution count, not 16 times total Agent CPU.
- CPU and worker occupancy depend on check duration, cardinality, and external calls. Execution time can include waiting and is not equivalent to CPU utilization.
- Shadow failures should not stop the normal source check.

### Ring capacity and retention

Approximate retention is:

```text
retention seconds ~= capacity / retained points per second
```

- Retained point rate scales with selected checks, instances, and metric contexts.
- Each shard overwrites independently, so a hot shard can shorten effective history.
- `ring_records` and `ring_capacity` show occupancy; `ring_overwritten_samples` shows active replacement. They do not report each shard's exact age span.

Storage is allocated lazily, only when that data type is retained:

| Retained data | Default base storage on 64-bit builds |
|---|---:|
| Scalar metrics | Approximately 12 MiB |
| Selected normal DogStatsD distributions | Additional approximately 8 MiB |

Metadata, names, hosts, tags, retained distributions, and Go runtime overhead are additional. Fixed storage scales approximately linearly with `capacity`.

### Supported metric types

| Source and output | Retained? |
|---|---:|
| Scalar series: gauge, count, rate, monotonic count, counter, or scalar histogram | Yes |
| Timestamped gauge or count | Yes |
| Selected normal DogStatsD distribution | Yes, as a finalized sketch series |
| Shadow-check distribution | No |
| OpenMetrics or histogram bucket | No |
| Service check or event | No |

### Other constraints

- The minimum shadow interval is 1 second.
- If a shadow run exceeds its interval, the scheduler does not run that check concurrently or accumulate an unbounded backlog. At most one subsequent run waits and starts when the current run finishes.
- Retained data exists only in process memory and is lost when the Core Agent process stops or restarts.

To disable the feature:

```yaml
metric_lookback:
  enabled: false
```

Do not use `enabled_checks: []` as an off switch: with global enablement, it selects every eligible check. Apply `enabled: false` through the deployment's supported Core Agent restart or replacement mechanism. Restarting clears retained points and unschedules shadow checks.
