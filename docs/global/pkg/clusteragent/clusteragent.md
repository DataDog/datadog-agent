> **TL;DR:** Business logic for the Datadog Cluster Agent, centralising cluster-check dispatching, Kubernetes admission webhooks, pod/cluster autoscaling, and orchestrator metadata collection to reduce load on the Kubernetes API server.

# pkg/clusteragent

## Purpose

`pkg/clusteragent` contains the business logic for the **Datadog Cluster Agent (DCA)**, a Kubernetes-only component that sits between node agents and the Kubernetes API server. Without the DCA, every node agent would query the API server directly, generating excessive load in large clusters. The DCA centralises:

- **Cluster checks**: discovery and dispatch of integration checks that target cluster-level resources (e.g. kube-state-metrics, etcd) to available node agents or CLC runners.
- **Admission webhooks**: mutating and validating Kubernetes admission controllers that automatically enrich pods with Datadog configuration (env vars, labels, APM instrumentation, etc.).
- **Autoscaling**: horizontal and vertical pod autoscaling driven by Datadog metrics, and cluster-level autoscaling of Karpenter node pools.
- **Orchestrator metadata**: collection and forwarding of Kubernetes resources (pods, nodes, deployments, …) to Datadog.

The binary entry point is `cmd/cluster-agent`. This package provides the internal packages consumed by that binary; it is not a standalone importable library.

---

## Key elements

### Key types

### Root

| Symbol | Kind | Description |
|--------|------|-------------|
| `ServerContext` | struct | Minimal context passed to API endpoint setup. Holds a pointer to `clusterchecks.Handler`. |

**File:** `pkg/clusteragent/servercontext.go`

**Build tag:** none (root is always compiled); most sub-packages require `kubeapiserver` or `clusterchecks`.

---

### admission

**Package path:** `pkg/clusteragent/admission`
**Build tag:** `kubeapiserver`

The admission controller registers HTTP endpoints with the Kubernetes API server that intercept pod-creation (and other) requests. Each endpoint runs a webhook that can mutate or validate the incoming object.

| Symbol | Kind | Description |
|--------|------|-------------|
| `ControllerContext` | struct | Aggregates all dependencies needed to start the controllers: `kubernetes.Interface` client, shared informer factories (secrets, validating/mutating webhooks), leadership callbacks, and the demultiplexer. |
| `StartControllers(ctx, wmeta, pa, datadogConfig)` | func | Bootstraps the `secret.Controller` (manages TLS cert rotation) and the `webhook.Controller` (manages `MutatingWebhookConfiguration` / `ValidatingWebhookConfiguration` objects in the cluster). Returns the slice of active `Webhook` values. |

**Sub-packages:**

| Package | Role |
|---------|------|
| `admission/controllers/secret` | Manages the TLS certificate secret used to authenticate webhook traffic. Auto-rotates before expiry. |
| `admission/controllers/webhook` | Owns the `Controller` interface (`Run`, `EnabledWebhooks`). Keeps the `MutatingWebhookConfiguration` and `ValidatingWebhookConfiguration` objects in sync with the enabled set of webhooks. |
| `admission/mutate` | Contains all **mutating** webhook implementations. |
| `admission/validate` | Contains all **validating** webhook implementations. |

#### Webhook interface

Every webhook must implement `webhook.Webhook` (`admission/controllers/webhook/controller_base.go`):

```go
type Webhook interface {
    Name() string
    WebhookType() common.WebhookType   // "mutating" or "validating"
    IsEnabled() bool
    Endpoint() string
    Resources() map[string][]string    // Kubernetes resource groups -> resource names
    Operations() []admiv1.OperationType
    LabelSelectors(useNamespaceSelector bool) (*metav1.LabelSelector, *metav1.LabelSelector)
    MatchConditions() []admiv1.MatchCondition
    WebhookFunc() admission.WebhookFunc
    Timeout() int32
}
```

#### Built-in webhooks

Registered in `generateWebhooks` inside `admission/controllers/webhook/controller_base.go`:

| Webhook | Package | Type | Purpose |
|---------|---------|------|---------|
| `config` | `mutate/config` | Mutating | Injects agent endpoint env vars and socket mounts into pods. |
| `tags_from_labels` | `mutate/tagsfromlabels` | Mutating | Propagates Kubernetes labels as Datadog tags via env vars. |
| `agent_sidecar` | `mutate/agent_sidecar` | Mutating | Injects a Datadog Agent sidecar (Fargate use-case). |
| `autoscaling` | `mutate/autoscaling` | Mutating | Patches pods on behalf of the vertical autoscaler. |
| `appsec` | `mutate/appsec` | Mutating | Enables AppSec proxy mode. |
| `auto_instrumentation` | `mutate/autoinstrumentation` | Mutating | Injects APM auto-instrumentation libraries (SSI). |
| `cws_instrumentation` (pods + execs) | `mutate/cwsinstrumentation` | Mutating | Instruments workloads for Cloud Workload Security. |
| `kubernetes_admission_events` | `validate/kubernetesadmissionevents` | Validating | Emits Datadog events for notable Kubernetes admission activity. |

#### Adding a new webhook

1. Create a new package under `admission/mutate/` (or `admission/validate/`).
2. Implement the `Webhook` interface.
3. Register it in `generateWebhooks` inside `admission/controllers/webhook/controller_base.go`.
4. Follow the config key convention `admission_controller.<webhook_name>.*`.

---

### clusterchecks

**Package path:** `pkg/clusteragent/clusterchecks`
**Build tag:** `clusterchecks`

Implements detection and dispatch of cluster-level integration checks to node agents or Cluster Level Check (CLC) runners.

| Symbol | Kind | Description |
|--------|------|-------------|
| `Handler` | struct | The top-level coordinator. Tracks leadership state and delegates to a `dispatcher`. Implements leader-election-aware HTTP API methods used by `cmd/cluster-agent/api/v1`. |
| `NewHandler(ac, tagger)` | func | Constructs a `Handler`. Requires a `pluggableAutoConfig` (the Autodiscovery instance) and a `tagger.Component`. Caches itself in the global agent cache for the status command. |
| `Handler.Run(ctx)` | method | Main goroutine. Waits for leadership, then starts the warmup period before enabling dispatching. Transitions back to follower on leadership loss. |
| `Handler.RejectOrForwardLeaderQuery(rw, req)` | method | Used by HTTP handlers; forwards requests to the leader if the current instance is a follower, or returns 503 during startup. |
| `Handler.GetConfigs(identifier)` | method | Returns the set of check configs dispatched to a given node agent. Called by node agents polling for their work. |
| `Handler.PostStatus(identifier, clientIP, status)` | method | Receives heartbeat/status from a node agent; updates node liveness in the store. |
| `dispatcher` | struct (unexported) | Holds the `clusterStore` of pending/dispatched configs. Implements `scheduler.Scheduler` so Autodiscovery can feed it configs directly. |
| `dispatcher.Schedule(configs)` | method | Implements `scheduler.Scheduler`. Filters non-cluster-check configs and assigns each remaining config to the node with the lowest load (or to the dangling queue if no node is available). |
| `dispatcher.Unschedule(configs)` | method | Removes configs from the store and from their assigned nodes. |

**Key config keys** (under `cluster_checks.*`):

| Key | Default | Purpose |
|-----|---------|---------|
| `cluster_checks.enabled` | `false` | Enable cluster checks dispatching. |
| `cluster_checks.warmup_duration` | `30s` | How long to wait after becoming leader before serving configs. |
| `cluster_checks.node_expiration_timeout` | `30s` | A node silent for this long is evicted; its configs become dangling. |
| `cluster_checks.advanced_dispatching_enabled` | `false` | Use runner utilization data for load-aware scheduling. |
| `cluster_checks.ksm_sharding_enabled` | `false` | Shard `kubernetes_state` checks across multiple CLC runners. |
| `cluster_checks.rebalance_period` | `10s` | How often to rebalance configs across nodes. |

---

### autoscaling

**Package path:** `pkg/clusteragent/autoscaling`

Contains three autoscaling sub-systems:

#### autoscaling/workload

**Build tag:** `kubeapiserver`

Implements the `DatadogPodAutoscaler` controller for horizontal and vertical pod autoscaling. Reads scaling recommendations from Remote Config or from external metrics, then applies them by patching `Deployment` specs (vertical) or adjusting replica counts (horizontal).

| Symbol | Kind | Description |
|--------|------|-------------|
| `Controller` | struct | Main reconciliation loop for `DatadogPodAutoscaler` objects. |
| `PodPatcher` | interface | Applies vertical scaling patches to pod templates. Used by the admission webhook. |
| `PodWatcher` | struct | Watches pod events to track rollout progress during vertical scaling. |

#### autoscaling/cluster

**Build tag:** `kubeapiserver`

Manages Karpenter `NodePool` objects based on cluster-level recommendations received via Remote Config. Creates, updates, or deletes Datadog-managed node pools on EC2.

| Symbol | Kind | Description |
|--------|------|-------------|
| `Controller` | struct | Embeds `autoscaling.Controller`. Reconciles NodePool objects against the internal store of recommendations. |
| `NewController(...)` | func | Constructor; wires up the Karpenter dynamic informer for `nodepools.karpenter.sh`. |

#### autoscaling/custommetrics and autoscaling/externalmetrics

Both serve custom metric values to the Kubernetes HPA controller via the [custom metrics API](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/#scaling-on-custom-metrics). `custommetrics` stores metric values in a ConfigMap; `externalmetrics` uses `DatadogMetric` CRDs.

---

### Other sub-packages

| Package | Role |
|---------|------|
| `api` | HTTP server helpers shared by cluster-check handlers (e.g. `LeaderForwarder`). |
| `languagedetection` | Handles language detection results sent by node agents and writes them to Kubernetes resource annotations. |
| `orchestrator` | Feeds Kubernetes resource snapshots (pods, nodes, deployments, …) into the Datadog orchestrator pipeline. |
| `metricsstore` / `metricsstatus` | In-memory stores and status reporters for external/custom metric values. |
| `patcher` | Applies Remote Config patches to Kubernetes resources. |
| `evictor` | Evicts pods as directed by the autoscaler. |
| `telemetry` | Shared telemetry metrics for the cluster agent. |

---

## Cross-references

| Related package / component | Relationship |
|-----------------------------|--------------|
| [`pkg/util/kubernetes/apiserver`](../../pkg/util/kubernetes-apiserver.md) | Provides the `APIClient` singleton (typed + dynamic k8s clients, informer factories, leader election) that every sub-package of `pkg/clusteragent` builds on. The `ControllerContext` in `admission` and `controllers` packages are wired up from `APIClient` fields at startup. |
| [`pkg/clusteragent/admission`](admission.md) | Admission webhook sub-system; started by `admission.StartControllers`. Receives `workloadmeta.Component` for language detection and autoscaling. See the dedicated doc for webhook authoring and config keys. |
| [`pkg/clusteragent/autoscaling`](autoscaling.md) | Horizontal/vertical pod and cluster node-pool autoscaling. The `mutate/autoscaling` webhook bridges to `autoscaling/workload` via the `PodPatcher` interface at pod-creation time. |
| [`comp/core/workloadmeta`](../../comp/core/workloadmeta.md) | The Cluster Agent runs a workloadmeta store with `AgentType: ClusterAgent`. It is fed by the `kubeapiserver` collector (Kubernetes resource events) and consumed by `clusterchecks`, `admission`, and `languagedetection`. |
| [`comp/core/tagger`](../../comp/core/tagger.md) | The tagger is passed to `clusterchecks.NewHandler`. The Cluster Agent typically runs with the dual-tagger module so it can fall back to a remote tagger when acting as a CLC runner. |
| [`pkg/util/kubernetes`](../../pkg/util/kubernetes.md) | Provides build-tag-free constants (`EnvTagLabelKey`, workload kind strings) and name-parsing helpers (`ParseDeploymentForReplicaSet`) used broadly across sub-packages. |
| [`pkg/languagedetection`](../../pkg/languagedetection.md) | Results sent by node agents are written to Kubernetes annotations by `pkg/clusteragent/languagedetection`. The `mutate/autoinstrumentation` webhook reads those annotations to select the right APM library. |

---

## Usage

`pkg/clusteragent` is consumed exclusively by the cluster-agent binary (`cmd/cluster-agent`). The typical call chain is:

```
cmd/cluster-agent/subcommands/start/command.go
  → apiserver.WaitForAPIClient(ctx)           // blocks until k8s API server reachable
  → leaderelection.CreateGlobalLeaderEngine   // sets up leader election (Lease / ConfigMap)
  → controllers.StartControllers(ctx)         // metadata, HPA/WPA, cluster-checks informers
  → admission.StartControllers(...)           // starts admission webhook controllers
  → clusterchecks.NewHandler(ac, tagger)      // creates cluster-check handler
  → handler.Run(ctx)                          // runs in a goroutine
  → cmd/cluster-agent/api/v1/clusterchecks.go // registers HTTP API endpoints
      → handler.GetConfigs(...)
      → handler.PostStatus(...)
```

Node agents and CLC runners communicate with the cluster agent over the internal HTTP API exposed by `cmd/cluster-agent/api/`. They call `GET /api/v1/clusterchecks/{nodeName}` to pull their assigned configs, and `POST /api/v1/clusterchecks/{nodeName}` to report status.

### Dependency injection via fx

The Cluster Agent binary uses the fx component framework. Key wiring points:

- `workloadmetafx.ModuleWithProvider(...)` with `AgentType: workloadmeta.ClusterAgent` activates the `kubeapiserver` collector.
- The tagger is wired with `taggerfxdual.Module(...)` so the cluster agent can act as both a local tagger (for checks running on the DCA pod) and fall back to a remote tagger when configured as a CLC runner.
- `APIClient` fields from `apiserver.WaitForAPIClient` are destructured into the `ControllerContext` structs consumed by `admission.StartControllers` and `controllers.StartControllers`.

### Leader-aware operation

All sub-systems in `pkg/clusteragent` respect the leader election state managed by `leaderelection.LeaderEngine` (from `pkg/util/kubernetes/apiserver/leaderelection`):

- `clusterchecks.Handler.Run` waits on `Subscribe` notifications before enabling dispatching.
- `autoscaling` controllers only fetch recommendations from the Datadog backend when they hold the lease.
- `admission.StartControllers` starts the secret and webhook controllers on every replica (they are idempotent), but only the leader actively rotates the TLS certificate.
