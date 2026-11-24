# Seeded anomaly-detection container logs

Runs for PRs involving `agent-anomaly-detection`, through `selection.yaml`.
Ported from the [seeded playground case][source] linked in AAD-92.

One Python container emits approximately 1,000 KiB/s to stdout. Seed
`20260928` generates 3,000 base patterns; the first 300 have ten times the
selection weight, and each line changes three letters of its chosen pattern.
The payload sequence repeats across runs; timing and accepted counts can vary.

Observer reads container logs at `info` severity with a 10,000 logs/s cap.
External metrics, kubelet logs, internal logs, and baseline muting are disabled.
BOCPD and ScanMW analyze signals from the log metrics and pattern extractors.
Storage uses the Agent defaults. The normal log forwarding pipeline is disabled.

## Configuration

CI uses `experiment.yaml` from the Agent PR for both baseline and comparison
images. Change detector settings in that PR to change the CI workload. For a
one-off run, edit a local copy and submit it with `--target-config-dir`, using
`--team-name agent`. No SMP service change is needed.

## Validation

Memory is the comparison goal; resource bounds are inherited from the
playground and remain provisional. Check accepted log bytes, queue drops,
rate-limiter drops, and learned pattern counts in the captured Observer
telemetry when assessing a run.

[source]: https://github.com/ddoghq/smp-playground/tree/55bc8fadabba668143efbc2391b57544c2a1275e/experiments/regression/agent/cases/observer_logs_seeded
