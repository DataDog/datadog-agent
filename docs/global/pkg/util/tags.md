> **TL;DR:** Provides helpers to collect and expose static tags that are attached to every piece of telemetry in environments (such as EKS Fargate sidecars) where traditional host metadata is unavailable.

# pkg/util/tags

## Purpose

`pkg/util/tags` provides helpers to collect and expose **static tags** — tags
that are attached to every piece of telemetry the agent produces, but that are
not included in host-level metadata.  These tags must be attached at the
collector or pipeline level because the environments that need them (EKS
Fargate, GKE Autopilot sidecars) have no traditional host and therefore never
emit host metadata.

The package also exposes a cluster-agent variant that bundles the full set of
global environment tags (including cluster-check and orchestrator-explorer
extras) for intra-cluster propagation.

## Key elements

### Key functions

| Function | Description |
|---|---|
| `GetStaticTagsSlice(ctx, config.Reader) []string` | Returns static tags as a flat `key:value` string slice. Tags without a colon are included verbatim. This is the low-level building block used when a string slice is needed directly. |
| `GetStaticTags(ctx, config.Component) map[string][]string` | Wraps `GetStaticTagsSlice` and returns a `map[key][]value` representation. Tags without a colon are silently dropped. This is the form used by most collectors. |
| `GetClusterAgentStaticTags(ctx, config.Reader) map[string][]string` | Only meaningful when `flavor.GetFlavor() == flavor.ClusterAgent`. Returns all global environment tags configured on the cluster agent: `DD_TAGS`, `DD_EXTRA_TAGS`, `DD_CLUSTER_CHECKS_EXTRA_TAGS`, `DD_ORCHESTRATOR_EXPLORER_EXTRA_TAGS`, plus `kube_distribution` and `orchestrator_cluster_id`. Returns `nil` for any other agent flavor. |

### Internal helper

`sliceToMap([]string) map[string][]string` — converts a `key:value` slice to
the map form; not exported.

### Tag sources (Fargate sidecar path)

When the agent detects it is running as a Fargate sidecar (`fargate.IsSidecar()
== true`), `GetStaticTagsSlice` additionally appends:

- `provider_kind:<value>` — set when `provider_kind` config key is non-empty
  (e.g. `gke-autopilot`).
- `DD_TAGS` / `DD_EXTRA_TAGS` — configured user tags from `configUtils.GetConfiguredTags`.
- `eks_fargate_node:<nodeName>` — EKS Fargate virtual node name.
- `kube_cluster_name:<cluster>` — autodiscovered from `clustername` package
  unless already present in `DD_TAGS`.
- Tags fetched from the cluster agent via `clusterinfo.GetClusterAgentStaticTagsWithRetry`
  (only when `cluster_agent.enabled` is true).
- `kube_distribution:eks` — hardcoded for EKS Fargate.

## Usage

Import path: `github.com/DataDog/datadog-agent/pkg/util/tags`

**DogStatsD server** (`comp/dogstatsd/server/server.go`) calls
`GetStaticTagsSlice` to prepend static tags to all metrics received on the
socket:

```go
if staticTags := tagutil.GetStaticTagsSlice(context.TODO(), cfg); staticTags != nil {
    // attach to every metric
}
```

**Workload-metadata tagger** (`comp/core/tagger/collectors/workloadmeta_main.go`)
calls both `GetStaticTags` (for the node-agent path) and
`GetClusterAgentStaticTags` (for the cluster-agent path) to populate the global
environment tag set exposed by the tagger.

**OTLP/OpenTelemetry pipeline** (`comp/otelcol/otlp/config.go`) calls
`GetStaticTagsSlice` to build the `DD_TAGS` string forwarded to the OTLP
exporter.

### Testing notes

- Use `configmock.New(t)` and `env.SetFeatures(t, env.EKSFargate)` to exercise
  the Fargate tag path without a real Fargate environment.
- `clustername.ResetClusterName()` must be called before tests that set
  `cluster_name` to avoid state leakage between test cases.
