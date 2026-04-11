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
