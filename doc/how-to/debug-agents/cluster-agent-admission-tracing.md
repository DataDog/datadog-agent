# Tracing Cluster Agent admission webhooks

Admission tracing uses the existing Cluster Agent tracer and service
`datadog-cluster-agent`. It does not enable application tracing or change webhook
selection, mutation, failure policy, or patch generation.

Enable it on the Cluster Agent with `DD_CLUSTER_AGENT_TRACING_ENABLED=true`.
`DD_CLUSTER_AGENT_TRACING_SAMPLE_RATE` controls sampling; the default is `0.1`.
For a short, controlled QA run, use `1.0` to retain representative requests at
source, then restore the usual rate. Traces use the existing APM transport and
`cluster_agent.tracing.env` setting; no additional tracing configuration is needed.

## Coverage

All names below have the prefix `cluster_agent.admission.`. Only `request` has
`span.kind=server`; the other spans represent internal phases.

```text
request                                 resource: fixed webhook name
  mutate                                resource: fixed webhook name
    mutate_pod                          resource: fixed webhook name
      tags_from_labels                  standalone and composed SSI/AppSec stages
        resolve_target                  when SSI selection is the filter
        owner_lookup                    only if owner-based tagging is needed
      inject_config                     standalone and composed SSI/AppSec stages
        resolve_target                  when SSI selection is the filter
      autoinstrumentation
        resolve_target
        inject_tracers                   only after selection/early-return checks
    prepare_patch                       serialization, JSON diff, patch encoding
```

The diagram shows possible children, not a requirement that every request emit
all of them. Other pod mutation routes get the shared flow, with `mutate_pod`
covering their callback. In particular, autoscaling has one coarse callback span
(resource `autoscaling`), not instrumentation of its recommender or internal
workload patcher. Validation, probe bypasses, and non-pod operations get the
request span without a pod mutation breakdown.

`mutate` includes decode, the baseline snapshot, and normalization. Its
`injected` tag records the callback's result, **not** whether the patch is empty.
`patch_present` separately records whether the generated patch contains
operations. A callback returning false may still generate a patch because of
normalization or mutation-status annotations. No early-return optimization is
introduced by tracing.

SSI currently evaluates selection in the tags filter, config filter, and APM
mutation. Each evaluation gets its own `resolve_target` span, with outcome
`selected`, `no_target`, or `blocked`. This makes repeated work observable without
changing selection semantics. `autoinstrumentation` records bounded skip reasons
(`already_instrumented`, `no_target`, `blocked`). Filtered tags/config stages use
`skip_reason=filtered`.

`owner_lookup` covers cache checks and the complete owner fallback, including
ReplicaSet-to-Deployment traversal and API calls on cache misses. Existing cache
hit/miss metrics remain available. Lookup failures mark that child span as failed,
while tags injection remains nonfatal with `skip_reason=owner_lookup_failed`.
Tracing does not change the client's existing request cancellation behavior.

## Privacy and errors

New tags contain only fixed webhook/stage names, bounded outcomes/reasons,
booleans, and HTTP status codes. No pod names, namespaces, UIDs, container names,
images, labels, annotations, environment values, request/patch contents, or user
information are added. Existing tracer-level cluster/service tags are unchanged.

Original errors remain in the existing admission response/logging paths, but are
not attached to spans: they may contain pod-derived values or object identities.
Spans use fixed sanitized errors instead. Request failures also have a bounded
`failure_stage`. A mutation error can be returned in an HTTP 200 response with
`Allowed=true` under existing fail-open behavior; HTTP success alone is therefore
not evidence of mutation success. `admission.allowed` is recorded separately.

## Validation

Focused package tests can be run with the repository's build-tag-aware tooling:

```sh
dda inv test --targets=./cmd/cluster-agent/admission,./pkg/clusteragent/admission/mutate/...
dda inv cluster-agent.build
```

Alternatively, use Bazel tests for those package trees. Mocktracer tests check
parentage, injection and no-target paths, repeated selection, reinvocation,
blocked policies, normalization-only patches, owner fallback failures, request
versions, probe bypasses, and sensitive-value exclusion. The common package also
has `BenchmarkMutateTracingDisabled` for one- and twenty-container pods with and
without injection. Compare base and feature revisions using the same benchmark
and toolchain; mocktracer is not a representative performance benchmark because
it retains spans.

For live QA on an authorized test cluster:

1. Enable existing DCA tracing and temporarily set sampling to `1.0`.
2. Submit a pod matching SSI and verify the connected stage spans in APM. Submit
   a nonmatching pod and verify selection spans but no `inject_tracers` span.
3. Re-admit an already-instrumented pod and verify `already_instrumented` on the
   APM stage. Tags/config may still have work: don't infer their outcome from the
   APM stage alone.
4. Exercise standalone config/tags webhooks and an autoscaling request. Use a pod
   lacking standard tags but with an owner to exercise `owner_lookup`.
5. Check returned patches/admitted pods against the uninstrumented build, inspect
   span tags for forbidden values, and compare request latency/allocations and
   binary sizes using equivalent builds.
6. Restore sampling. Use aggregate webhook duration/count metrics to measure
   overall frequency; sampled traces explain individual slow executions, not the
   exact number of slow requests.

Intentional gaps: no per-container/library/env spans, automatic client-go
integration, or deeper autoscaling spans. Nonfatal library injection failures
handled internally by `injectTracers` are not reclassified as webhook errors.
A successful timing span does not replace existing injection-outcome telemetry.
Unit coverage does not establish live intake delivery or negligible production
overhead; those require the live and benchmark checks above.
