> **TL;DR:** `pkg/hosttags` provides a time-bounded cache of host tags that the aggregator attaches to metrics during the startup window, preventing tag loss while cloud-provider metadata is still being fetched.

# pkg/hosttags

## Purpose

`pkg/hosttags` provides a **time-bounded cache of host tags** that the aggregator attaches to metrics during the startup window. Normally, host tags are carried by the backend (attached to hosts through metadata) rather than embedded in every metric. However, during the first minutes after agent startup some tags — especially those fetched from cloud provider APIs — may not yet have been picked up by the backend. `HostTagProvider` solves this by attaching the tags directly to metrics for a configurable duration (`expected_tags_duration`), after which it clears them to avoid redundancy.

There are **two related packages** that together implement host tag collection:

| Package | Role |
|---|---|
| `pkg/hosttags` | Time-bounded provider consumed by the aggregator/demultiplexer |
| `comp/metadata/host/hostimpl/hosttags` | Full collection logic: reads config tags, queries cloud providers (EC2, GCE, Docker, Kubernetes), applies caching |

---

## Key Elements

### Key types

#### `pkg/hosttags`

**`HostTagProvider`** — thread-safe struct that holds a snapshot of host tags and automatically expires it:

```go
type HostTagProvider struct {
    hostTags []string
    sync.RWMutex
}
```

**`NewHostTagProvider() *HostTagProvider`** — creates a provider using the `expected_tags_duration` config value. If the duration is zero or negative, `GetHostTags` always returns `nil` (tags are never attached to metrics).

**`NewHostTagProviderWithDuration(duration time.Duration) *HostTagProvider`** — same but with an explicit duration; useful in tests.

**`(p *HostTagProvider) GetHostTags() []string`** — returns the current snapshot, or `nil` if the deadline has passed. Callers should treat a `nil` return as "no host tags to attach".

**Expiry mechanism** — at construction time, the provider calls `hostMetadataUtils.Get(ctx, false, cfg).System` to snapshot the current `System` tags, then schedules a `clock.AfterFunc` callback to nil-out the slice when `pkgconfigsetup.StartTime + expected_tags_duration` is reached. The `clock.Clock` abstraction is injectable for testing.

---

### Key functions

#### `comp/metadata/host/hostimpl/hosttags` (collection layer)

**`Tags`** — the result type:

```go
type Tags struct {
    System              []string `json:"system"`
    GoogleCloudPlatform []string `json:"google cloud platform,omitempty"`
}
```

GCE tags are stored separately under `GoogleCloudPlatform` because they are reported in a dedicated section of the Datadog host metadata payload; all other tags go into `System`.

**`Get(ctx, cached, conf) *Tags`** — the main collection function. Assembles tags from multiple sources in order:

1. **Config tags** — `tags`, `extra_tags`, and `DD_TAGS` env var (via `configUtils.GetConfiguredTags`).
2. **`env` config key** — appended as `env:<value>`.
3. **GPU tags** — if `collect_gpu_tags: true`.
4. **Cluster name** — resolved via `clustername.GetClusterNameTagValue`; added as both `kube_cluster_name:<name>` and `cluster_name:<name>` (the latter can be disabled with `disable_cluster_name_tag_key`).
5. **Kubernetes distribution** — `kube_distribution:<name>` from the cloud provider info.
6. **Cloud/infrastructure providers** (with retry logic):
   - `gce` — GCE instance labels via `gce.GetTags` (1 retry); stored in `GoogleCloudPlatform`.
   - `ec2` — EC2 instance tags via `ec2tags.GetTags` (10 retries, gated on `collect_ec2_tags`).
   - `ec2_instance_info` — EC2 instance attributes via `ec2tags.GetInstanceInfo` (3 retries, gated on `collect_ec2_instance_info`).
   - `kubernetes` — node labels/annotations via `k8s.NewKubeNodeTagsProvider` (10 retries).
   - `kubernetes_cluster_agent_tags` — static tags from the Cluster Agent (10 retries).
   - `docker` — Docker container labels via `docker.GetTags` (1 retry).

**Retry and per-provider caching** — if a provider fails, the function retries up to its configured limit (sleeping 1 second between attempts). On exhaustion it falls back to a previously cached value for that provider (`cache.Cache`, keyed per provider name). This two-level caching ensures that a transient Cluster Agent outage does not drop tags for up to 30 minutes.

**`tag_value_split_separator`** — a config map that allows splitting multi-value tags. For example `environment: ","` would split `environment:prod,staging` into two tags.

---

### Configuration and build flags

No build tags. Key configuration options are `expected_tags_duration`, `collect_ec2_tags`, `collect_ec2_instance_info`, `collect_gce_tags`, `collect_gpu_tags`, `disable_cluster_name_tag_key`, and `tag_value_split_separator`. See `## Usage / Configuration keys` for the full table.

## Usage

### In the aggregator

`pkg/aggregator/demultiplexer_agent.go` creates a `HostTagProvider` during `AgentDemultiplexer` initialisation and stores it as `hostTagProvider`. The time samplers call `GetHostTags()` and append the returned tags to every metric sample during the startup window.

```go
demux.hostTagProvider = hosttags.NewHostTagProvider()
```

The same pattern is used in `demultiplexer_serverless.go` and `no_aggregation_stream_worker.go`.

### In metadata

`comp/metadata/host/hostimpl/hosttags.Get` (the collection layer) is called:
- By `pkg/hosttags.NewHostTagProvider` (at construction, `cached=false`) to snapshot the initial tags.
- By the host metadata component to include tags in the periodic host metadata payload.

### Configuration keys

| Key | Default | Effect |
|---|---|---|
| `expected_tags_duration` | `0s` | How long to attach host tags to metrics after startup. Set to e.g. `10m` on cloud instances. |
| `collect_ec2_tags` | `false` | Enable EC2 instance tag collection. |
| `collect_ec2_instance_info` | `false` | Enable EC2 instance attribute collection. |
| `collect_gce_tags` | `true` | Enable GCE label collection. |
| `collect_gpu_tags` | `false` | Enable GPU tag collection. |
| `disable_cluster_name_tag_key` | `false` | Suppress `cluster_name` tag, keeping only `kube_cluster_name`. |
| `tag_value_split_separator` | `{}` | Map of tag key to separator string for splitting multi-value tags. |

---

---

## Related packages

| Package | Relationship |
|---|---|
| [`pkg/util/cloudproviders`](util/cloudproviders.md) | The collection layer (`comp/metadata/host/hostimpl/hosttags.Get`) calls `gce.GetTags` (GCE labels), `azure`, `kubernetes`, and other sub-packages through `pkg/util/cloudproviders`. The root `DetectCloudProvider` result is also used to gate which tag providers are retried. |
| [`pkg/util/ec2`](util/ec2.md) | EC2 instance tags are fetched via `ec2tags.GetTags` (gated on `collect_ec2_tags`) and instance attributes via `ec2tags.GetInstanceInfo` (gated on `collect_ec2_instance_info`). Both calls are wrapped in retry loops (10 attempts with 1-second sleep) before falling back to the per-provider cache. |
| [`comp/metadata/host`](../../comp/metadata/host.md) | The host metadata component includes a `HostTags` field (of type `hosttags.Tags`) in the `utils.Payload` struct sent to the Datadog backend. `comp/metadata/host/hostimpl/hosttags.Get` is the shared collection function used both here and by `pkg/hosttags.NewHostTagProvider`. |

---

## Notes for contributors

- `pkg/hosttags` is intentionally thin. Collection logic belongs in `comp/metadata/host/hostimpl/hosttags`, not here.
- Adding a new cloud-provider tag source means adding a new entry to `getProvidersDefinitions` in the collection layer, choosing an appropriate retry count, and deciding whether the tags go into `System` or a separate field.
- The `retrySleepTime` and `getProvidersDefinitionsFunc` variables in the collection layer are package-level vars to allow substitution in tests.
