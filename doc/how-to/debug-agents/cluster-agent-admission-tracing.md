# Tracing Cluster Agent admission webhooks

Admission tracing uses the existing Cluster Agent tracer and service
`datadog-cluster-agent`. Enable it with `DD_CLUSTER_AGENT_TRACING_ENABLED=true`;
`DD_CLUSTER_AGENT_TRACING_SAMPLE_RATE` controls sampling (default `0.1`). It does
not change webhook selection, mutation, failure policy, or patch generation.

## Shared spans

All names below have the prefix `cluster_agent.admission.`:

```text
request                       inbound admission handling (span.kind=server)
  mutate                      pod decode, baseline snapshot, normalization
    mutate_pod                mutation callback, including selection/early exits
    prepare_patch             serialization, JSON diff, patch encoding
```

Request, mutate, and callback resource names identify the fixed webhook name.
This gives SSI, autoscaling, standalone tags/config, and other shared pod mutation
routes coarse timing without instrumenting their internals. Only `request` is a
server span; mutation spans are internal. Validation, non-pod operations, and
probe bypasses have request timing without this pod mutation breakdown.

`injected` records the callback result, while `patch_present` records whether the
patch contains operations. They are distinct: normalization or status annotations
can produce a patch even when the callback returns false. No skip-path
optimization is introduced. Request failures have a bounded `failure_stage`;
`admission.allowed` and HTTP status are separate because mutation failures can
remain fail-open with HTTP 200.

New tags contain fixed operational names, booleans, bounded failure stages, and
HTTP status codes, not pod contents, object identities, label/annotation/env
values, or user information. Span errors are sanitized; existing admission
responses and logging retain their original errors. Tracer-level configuration
and cluster/service tags are unchanged.

## Validation and follow-up

Run focused tests with `dda inv test --targets=./cmd/cluster-agent/admission,./pkg/clusteragent/admission/mutate/...`
and build with `dda inv cluster-agent.build`, or use the corresponding Bazel
tests. Tests cover hierarchy, request versions, privacy, errors, injection,
no-injection, and normalization-only patches. The common package includes
`BenchmarkMutateTracingDisabled`; compare base and feature revisions using the
same benchmark and toolchain rather than benchmarking mocktracer.

For live QA on an authorized test cluster, temporarily use sampling `1.0`, submit
matching and nonmatching SSI pods plus standalone tags/config and autoscaling
requests, and verify the hierarchy and outcomes in APM. Compare admitted results,
latency, and binary size against an equivalent base build; restore sampling
afterward. Sampled traces explain individual requests; aggregate metrics measure
overall frequency. Unit tests do not establish intake delivery or overhead.

SSI selection/injection, composed tags/config stages, and owner lookup spans are
intentionally deferred to [CONTP-2245](https://datadoghq.atlassian.net/browse/CONTP-2245)
for team evaluation. No context-aware mutator/filter interface or automatic
client-go instrumentation is added in this shared-tracing change.
