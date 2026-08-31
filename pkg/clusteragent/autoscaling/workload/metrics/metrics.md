# DatadogPodAutoscaler Metrics

All metrics are emitted by the Cluster Agent and share the prefix
`datadog.cluster_agent.autoscaling.workload`.

## Base tags

Every metric carries the following base tags.

| Tag key | Description |
|---------|-------------|
| `namespace` | Kubernetes namespace of the DPA object (kept for backward compatibility with `kube_namespace`) |
| `kube_namespace` | Kubernetes namespace of the DPA object |
| `target_name` | Name of the scaling target (from `spec.targetRef.name`) |
| `target_kind` | Kind of the scaling target, lowercased (e.g. `deployment`, `statefulset`) |
| `autoscaler_name` | Name of the DPA object (kept for backward compatibility with `name`) |
| `name` | Name of the DPA object |
| `join_leader` | Always `true`; used to join metrics with leader-election metrics |
| *(arbitrary)* | Any key/value pairs found in the `ad.datadoghq.com/tags` annotation on the DPA object (JSON map, e.g. `{"team":"payments","tier":"critical"}`) |
| `env` | Unified Service Tagging `env` label (`tags.datadoghq.com/env`) if set on the DPA object |
| `service` | Unified Service Tagging `service` label (`tags.datadoghq.com/service`) if set on the DPA object |
| `version` | Unified Service Tagging `version` label (`tags.datadoghq.com/version`) if set on the DPA object |

---

## Metrics

### Recommendations

#### `datadog.cluster_agent.autoscaling.workload.received_recommendations_version`
- **Type:** Gauge
- **Tags:** base tags
- **Description:** Version number of the most-recently received scaling recommendation from the
  remote recommender. Emitted only when a valid recommendation with a positive version has been
  received. Can be used to detect stale recommendations by comparing this value over time or
  against an expected version.

#### `datadog.cluster_agent.autoscaling.workload.local.fallback_enabled`
- **Type:** Gauge
- **Tags:** base tags
- **Description:** Indicates whether the local (in-cluster) fallback recommender is currently
  active for horizontal scaling. Value is `1` when the active horizontal source is `Local`,
  `0` otherwise. Always emitted.

---

### Apply mode

#### `datadog.cluster_agent.autoscaling.workload.apply_mode`
- **Type:** Gauge
- **Tags:** base tags + `dpa_mode` + `dpa_dimension`
- **Description:** Info-style metric that exposes the DPA apply mode for each enabled
  autoscaling dimension. Value is always `1`. The `dpa_mode` tag is `apply` when
  `spec.applyPolicy.mode` is unset, empty, or `Apply`; it is `preview` when the mode is
  `Preview`. The `dpa_dimension` tag is emitted for each enabled dimension (`horizontal`,
  `vertical`, or both); disabled dimensions are not emitted. Use this metric when you need to count
  or filter DPAs by preview/apply mode.

---

### Horizontal scaling — received recommendations

#### `datadog.cluster_agent.autoscaling.workload.horizontal_scaling_received_replicas`
- **Type:** Gauge
- **Tags:** base tags + `source`
- **Description:** Number of replicas recommended by the main (non-fallback) horizontal scaling
  source. The `source` tag identifies where the recommendation originated (e.g. `DDM`,
  `Autoscaler`). Emitted only when a horizontal recommendation is present.

---

### Vertical scaling — received recommendations

#### `datadog.cluster_agent.autoscaling.workload.vertical_scaling_received_requests`
- **Type:** Gauge
- **Tags:** base tags + `source` + `kube_container_name` + `resource_name`
- **Description:** Resource request value (in the unit native to the resource: millicores for CPU,
  bytes for memory) recommended by the main vertical scaling source for a specific container and
  resource. One metric point is emitted per container/resource pair. Emitted only when a vertical
  recommendation is present.

#### `datadog.cluster_agent.autoscaling.workload.vertical_scaling_received_limits`
- **Type:** Gauge
- **Tags:** base tags + `source` + `kube_container_name` + `resource_name`
- **Description:** Resource limit value (in the unit native to the resource: millicores for CPU,
  bytes for memory) recommended by the main vertical scaling source for a specific container and
  resource. One metric point is emitted per container/resource pair. Emitted only when a vertical
  recommendation is present.

---

### Horizontal scaling — applied actions

#### `datadog.cluster_agent.autoscaling.workload.horizontal_scaling_applied_replicas`
- **Type:** Gauge
- **Tags:** base tags + `source`
- **Description:** Number of replicas that were last applied to the target workload by the
  horizontal scaler. Reflects the most recent scaling action. Emitted only when at least one
  horizontal scaling action has been taken. The `source` tag reflects the active horizontal
  scaling source at query time.

#### `datadog.cluster_agent.autoscaling.workload.horizontal_scaling_actions`
- **Type:** MonotonicCount
- **Tags:** base tags + `source` + `status` (`ok` or `error`)
- **Description:** Cumulative count of horizontal scaling actions attempted by the Cluster Agent,
  split by outcome. Use `status:ok` for successful scale operations and `status:error` for
  failed ones. Always emitted (two points per flush, one per status value). The `source` tag
  reflects the active horizontal scaling source.

---

### Vertical scaling — applied actions

#### `datadog.cluster_agent.autoscaling.workload.vertical_rollout_triggered`
- **Type:** MonotonicCount
- **Tags:** base tags + `source` + `status` (`ok` or `error`)
- **Description:** Cumulative count of pod rollouts triggered by the vertical scaler to apply new
  resource recommendations, split by outcome. Use `status:ok` for successful rollouts and
  `status:error` for failed ones. Always emitted (two points per flush, one per status value).
  The `source` tag reflects the active vertical scaling source.

---

### Local (fallback) recommender

#### `datadog.cluster_agent.autoscaling.workload.local.horizontal_scaling_recommended_replicas`
- **Type:** Gauge
- **Tags:** base tags + `source`
- **Description:** Number of replicas recommended by the local in-cluster fallback recommender.
  This metric is independent of whether the fallback is currently active; it is emitted whenever
  the fallback has produced a recommendation. Useful for comparing the fallback recommendation
  against the primary recommendation.

#### `datadog.cluster_agent.autoscaling.workload.local.horizontal_utilization_pct`
- **Type:** Gauge
- **Tags:** base tags + `source`
- **Description:** CPU utilization percentage (0–100) computed by the local fallback recommender
  when deriving its horizontal scaling recommendation. Emitted only when the fallback recommender
  has computed a utilization-based recommendation.

---

### Horizontal scaling — constraints

#### `datadog.cluster_agent.autoscaling.workload.horizontal_scaling.constraints.max_replicas`
- **Type:** Gauge
- **Tags:** base tags
- **Description:** Maximum number of replicas configured in the DPA `spec.constraints.maxReplicas`
  field. Emitted only when the constraint is set. Use this to verify that the configured upper
  bound is what you expect and to alert when the autoscaler is at its ceiling.

#### `datadog.cluster_agent.autoscaling.workload.horizontal_scaling.constraints.min_replicas`
- **Type:** Gauge
- **Tags:** base tags
- **Description:** Minimum number of replicas configured in the DPA `spec.constraints.minReplicas`
  field. Emitted only when the constraint is set. Use this to verify that the configured lower
  bound is what you expect and to alert when the autoscaler is at its floor.

---

### Vertical scaling — container constraints

One metric point is emitted per container that has the corresponding constraint configured.
The `kube_container_name` tag identifies the container. CPU values are in **millicores**;
memory values are in **bytes**.

#### `datadog.cluster_agent.autoscaling.workload.vertical_scaling.constraints.container.cpu.request_min`
- **Type:** Gauge
- **Tags:** base tags + `kube_container_name`
- **Description:** Minimum CPU request (in millicores) allowed for the container, as configured
  in `spec.constraints.containers[*].minAllowed` (or the deprecated
  `spec.constraints.containers[*].requests.minAllowed`).

#### `datadog.cluster_agent.autoscaling.workload.vertical_scaling.constraints.container.memory.request_min`
- **Type:** Gauge
- **Tags:** base tags + `kube_container_name`
- **Description:** Minimum memory request (in bytes) allowed for the container, as configured
  in `spec.constraints.containers[*].minAllowed` (or the deprecated
  `spec.constraints.containers[*].requests.minAllowed`).

#### `datadog.cluster_agent.autoscaling.workload.vertical_scaling.constraints.container.cpu.request_max`
- **Type:** Gauge
- **Tags:** base tags + `kube_container_name`
- **Description:** Maximum CPU request (in millicores) allowed for the container, as configured
  in `spec.constraints.containers[*].maxAllowed` (or the deprecated
  `spec.constraints.containers[*].requests.maxAllowed`).

#### `datadog.cluster_agent.autoscaling.workload.vertical_scaling.constraints.container.memory.request_max`
- **Type:** Gauge
- **Tags:** base tags + `kube_container_name`
- **Description:** Maximum memory request (in bytes) allowed for the container, as configured
  in `spec.constraints.containers[*].maxAllowed` (or the deprecated
  `spec.constraints.containers[*].requests.maxAllowed`).

#### `datadog.cluster_agent.autoscaling.workload.vertical_scaling.controlled_resources`
- **Type:** Gauge
- **Tags:** base tags + `kube_container_name` + `resource_name`
- **Description:** Info-style metric that exposes which container resources are controlled by
  vertical autoscaling. Value is always `1`. One point is emitted per controlled resource, so the
  same DPA/container can emit multiple `resource_name` tag values, such as `cpu` and `memory`.
  Emitted only when vertical autoscaling is enabled for the DPA.
  If `spec.constraints` is omitted, or `spec.constraints.containers` is empty, the metric emits
  `kube_container_name:all` with both `resource_name:cpu` and `resource_name:memory`, matching the
  controller default. If `spec.constraints.containers[*].controlledResources` is omitted, the metric
  emits both `resource_name:cpu` and `resource_name:memory` for that container constraint. If
  `controlledResources` is an empty list or the container constraint has `enabled: false`, no point
  is emitted for that container constraint. A wildcard container constraint named `*` is emitted as
  `kube_container_name:all`.

---

### Autoscaling objectives (target values)

One metric point is emitted per objective configured in `spec.objectives[*]` that has a value
set. The objective's kind and value semantics are differentiated entirely by tags — see the
value-unit table below.

#### `datadog.cluster_agent.autoscaling.workload.objective.target`
- **Type:** Gauge
- **Tags:** base tags + `objective_type` + `value_type` + `objective_index` + `resource_name`
  *(only for resource objectives)* + `kube_container_name` *(only for container-resource
  objectives)*
- **Description:** Target value the autoscaler aims to reach and maintain for the workload, as
  configured in `spec.objectives`. One point is emitted per objective. Objectives whose value
  pointer is unset are skipped.

| Tag | Values | Meaning |
|-----|--------|---------|
| `objective_type` | `pod_resource`, `container_resource`, `custom_query` | The objective kind (from `spec.objectives[*].type`). |
| `value_type` | `utilization`, `absolute_value` | How the target is expressed (from `spec.objectives[*].*.value.type`). |
| `objective_index` | `0`, `1`, … | 0-based position of the objective in `spec.objectives[]`. Guarantees a unique tag-set per objective so multiple objectives never collapse into one timeseries — this is the only distinguishing tag for multiple `custom_query` objectives, or for two objectives that share the same resource/container. Note it is **positional**: reordering or inserting an objective shifts the indices of those after it. |
| `resource_name` | `cpu`, `memory` | The resource being targeted. Present for `pod_resource` and `container_resource` objectives; **omitted** for `custom_query`. |
| `kube_container_name` | container name | The targeted container. Present **only** for `container_resource` objectives. |

**Value units** (the metric value is unitless in the timeseries, so the meaning depends on the
tags — filter by `value_type` before graphing so utilization and absolute values are not mixed):

| `value_type` | `resource_name` | Value unit |
|--------------|-----------------|------------|
| `utilization` | `cpu` / `memory` | Percentage, `0`–`100` (e.g. `70` for a 70% target). |
| `absolute_value` | `cpu` | Millicores (e.g. `500m` → `500`). |
| `absolute_value` | `memory` | Bytes (e.g. `256Mi` → `268435456`). |
| `absolute_value` | *(none — `custom_query`)* | The query's native unit as a floating-point number (e.g. `500M` → `5e8`); no CPU/memory conversion is applied. |

---

### Status — desired resources

These metrics reflect the **desired state** stored in the DPA `.status` subresource — i.e.
what the autoscaler intends to apply, which may differ from the currently running configuration
until the next reconciliation or rollout completes.

#### `datadog.cluster_agent.autoscaling.workload.status.desired.replicas`
- **Type:** Gauge
- **Tags:** base tags
- **Description:** Desired replica count as stored in the DPA `status.horizontal.target.replicas`
  field. Emitted only when the horizontal target status is present. Useful for tracking the
  autoscaler's intended replica count independently of what is currently running.

#### `datadog.cluster_agent.autoscaling.workload.status.vertical.desired.container.cpu.request`
- **Type:** Gauge
- **Tags:** base tags + `kube_container_name`
- **Description:** Desired CPU request (in millicores) for the container, as stored in the DPA
  `status.vertical.target.desiredResources` field. Emitted only when the vertical target status
  is present and the container has a CPU request entry.

#### `datadog.cluster_agent.autoscaling.workload.status.vertical.desired.container.memory.request`
- **Type:** Gauge
- **Tags:** base tags + `kube_container_name`
- **Description:** Desired memory request (in bytes) for the container, as stored in the DPA
  `status.vertical.target.desiredResources` field. Emitted only when the vertical target status
  is present and the container has a memory request entry.

#### `datadog.cluster_agent.autoscaling.workload.status.vertical.desired.container.cpu.limit`
- **Type:** Gauge
- **Tags:** base tags + `kube_container_name`
- **Description:** Desired CPU limit (in millicores) for the container, as stored in the DPA
  `status.vertical.target.desiredResources` field. Emitted only when the vertical target status
  is present and the container has a CPU limit entry.

#### `datadog.cluster_agent.autoscaling.workload.status.vertical.desired.container.memory.limit`
- **Type:** Gauge
- **Tags:** base tags + `kube_container_name`
- **Description:** Desired memory limit (in bytes) for the container, as stored in the DPA
  `status.vertical.target.desiredResources` field. Emitted only when the vertical target status
  is present and the container has a memory limit entry.

---

### Autoscaler conditions

#### `datadog.cluster_agent.autoscaling.workload.autoscaler_conditions`
- **Type:** Gauge
- **Tags:** base tags + `type`
- **Description:** Current state of each condition on the DPA object (from
  `status.conditions`). Value is `1` when the condition status is `True`, `0` otherwise.
  The `type` tag contains the condition type string (e.g. `Active`, `ScalingLimited`,
  `HorizontalAbleToScale`, `VerticalAbleToScale`). One metric point is emitted per condition
  present on the object.
