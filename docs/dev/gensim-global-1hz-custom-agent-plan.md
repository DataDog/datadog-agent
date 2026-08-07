# Throwaway global 1 Hz Agent for GenSim

## Purpose

Build one experimental Datadog Agent image for a bounded GenSim/Bits evaluation:

- **control:** normal Agent behavior;
- **treatment:** Agent-owned recurring checks, ordinary DogStatsD aggregation, and metric flushing use a hardcoded one-second cadence.

This code is test scaffolding and is not intended to merge. The same immutable image digest must run in both arms.

## Runtime switch

The image has one private environment toggle:

```text
DD_METRIC_RESOLUTION_EXPERIMENT_ENABLED=true
```

Unset, invalid, or false values retain normal behavior. There are no configurable treatment intervals, YAML settings, generated schema, or release notes. When enabled, every treatment interval is hardcoded to one second.

## Behavior

### Recurring checks

Treatment forces every positive normal check interval to one second at the scheduler boundary. It preserves:

- interval zero as one-shot;
- negative-interval rejection;
- shadow-check intervals;
- cancellation through the effective queue; and
- existing same-check overlap prevention.

A one-second schedule does not create source information that an integration, API, cache, or producer does not expose.

### Ordinary DogStatsD

Treatment constructs ordinary TimeSamplers with a one-second bucket and flushes the metric aggregator every second. Gauge, count/rate, histogram, distribution, and sketch behavior continues through the existing TimeSampler path.

Timestamped DogStatsD remains unchanged: producer timestamps survive and count/rate normalization plus `Serie.Interval` retain the existing fixed ten-second semantics.

An explicit zero flush interval used by one-shot commands remains zero even in treatment mode.

## GenSim deployment

`RunEpisodeJobRequest.image` selects sim-generator, not the Datadog Agent. The Agent is owned by the maintained app bundle.

For each selected app:

1. set `SHARED_DATADOG_MODE=false`;
2. deploy the app-local Datadog Helm dependency;
3. pin `datadog.agents.image.repository` and `datadog.agents.image.digest`; and
4. inject the toggle through `datadog.agents.containers.agent.envDict`.

Example app-local hook:

```bash
SHARED_DATADOG_MODE=false

RUNTIME_AGENT_IMAGE_REPOSITORY="REGISTRY/PROJECT/datadog-agent"
RUNTIME_AGENT_IMAGE_DIGEST="sha256:FULL_RESOLVED_DIGEST"
RUNTIME_METRIC_RESOLUTION_ENABLED="CONTROL_FALSE_OR_TREATMENT_TRUE"

runtime_append_helm_set_args() {
    HELM_SET_ARGS+=(
        --set-string "datadog.agents.image.repository=${RUNTIME_AGENT_IMAGE_REPOSITORY}"
        --set-string "datadog.agents.image.digest=${RUNTIME_AGENT_IMAGE_DIGEST}"
        --set-string "datadog.agents.containers.agent.envDict.DD_METRIC_RESOLUTION_EXPERIMENT_ENABLED=${RUNTIME_METRIC_RESOLUTION_ENABLED}"
    )
}
```

The current worker request has no per-execution Agent override. Unless GenSim owners add one, use normalized immutable control/treatment app-bundle variants. Both variants must pin the same Agent digest and differ only in the toggle value.

## Required runtime proof

Before using a capture, retain:

```text
DaemonSet PodSpec image == expected repository@digest
Pod imageID == expected resolved digest
filtered Agent environment/config == expected toggle
Agent status is healthy
```

Stop if shared-Agent mode remains enabled, the image cannot be pulled, the digest differs, or the effective toggle is wrong.

## Validation status

Completed locally:

- focused configuration-toggle, scheduler, collector, demultiplexer, aggregator, and ring-buffer tests;
- Go lint, Agent build, schema generation, and static diff checks;
- treatment runtime against a local decoding HTTP sink;
- consecutive one-second core-check points;
- one-second ordinary DogStatsD gauge, count, and timing buckets;
- exact timestamped DogStatsD timestamp/value preservation with interval 10;
- approximately one-second serializer request cadence; and
- clean Agent shutdown.

The local sink is author-local preflight evidence, not a repository attestation or substitute for packaged-image, fakeintake, backend, Bits, or managed GenSim proof.

## Remaining gates

Before collecting canonical pairs:

1. build, scan/sign, publish, and record the amd64 image digest;
2. qualify the packaged image and smoke-only Python check;
3. obtain skipped-check and scheduler queue-lag observability;
4. pass CPU, RSS, restart/OOM, drop/error, and point-completeness soak limits;
5. run the disposable managed Pharmacy smoke;
6. prove distinct one-second timestamps and tags in the normal metrics backend;
7. prove Bits uses the correct org, tags, historical window, requested interval, and returned resolution; and
8. stop on coarse rollup, wrong scope, interpolation, timestamp rewriting, tag loss, or unexplained point loss.

The disposable smoke and historical discovery records are not members of the canonical paired dataset.
