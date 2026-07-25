# Cluster Agent

-----

The Datadog Cluster Agent (DCA) is a separate binary, `datadog-cluster-agent`, that runs as a Kubernetes Deployment and acts as the single point of contact between a cluster and both the Kubernetes API server and the Datadog backend for cluster-level concerns. Without it, every node agent would open its own watch streams against the apiserver — a load that scales linearly with node count. With it, node agents fetch cluster-level tags and metadata, dispatched check configurations, and the cluster ID from the DCA over an authenticated HTTPS + gRPC API on port 5005, and the DCA alone runs the controllers that patch cluster resources, serve Kubernetes APIServices, and admit pods.

The DCA is Kubernetes-only: the binary builds with `!windows && kubeapiserver` build tags and ships as a Linux container image. A Cloud Foundry variant, [`cmd/cluster-agent-cloudfoundry`](<<<SRC>>>/cmd/cluster-agent-cloudfoundry), reuses the same packages with BBS/CAPI metadata endpoints instead of Kubernetes ones.

## Key packages

| Path | Purpose |
|---|---|
| [`cmd/cluster-agent/main.go`](<<<SRC>>>/cmd/cluster-agent/main.go) | Entry point; sets flavor `ClusterAgent` |
| [`cmd/cluster-agent/subcommands/start/command.go`](<<<SRC>>>/cmd/cluster-agent/subcommands/start/command.go) | The full startup sequence: API server, leader election, controllers, remote config, every product |
| [`cmd/cluster-agent/api/server.go`](<<<SRC>>>/cmd/cluster-agent/api/server.go) | Muxed HTTPS + gRPC server on port 5005, dual-token validation |
| [`cmd/cluster-agent/api/grpc.go`](<<<SRC>>>/cmd/cluster-agent/api/grpc.go) | gRPC `AgentSecure` service: tagger stream, kube-metadata stream |
| [`cmd/cluster-agent/api/v1`](<<<SRC>>>/cmd/cluster-agent/api/v1) | Node-agent-facing REST endpoints: tags, metadata, cluster checks, language detection, cluster ID |
| [`cmd/cluster-agent/api/agent/agent.go`](<<<SRC>>>/cmd/cluster-agent/api/agent/agent.go) | Intra-pod CLI endpoints (`/status`, `/flare`, `/config`, ...) |
| [`pkg/clusteragent/api`](<<<SRC>>>/pkg/clusteragent/api) | `LeaderForwarder` reverse proxy and the `WithLeaderProxyHandler` wrapper |
| [`pkg/util/kubernetes/apiserver/leaderelection/leaderelection.go`](<<<SRC>>>/pkg/util/kubernetes/apiserver/leaderelection/leaderelection.go) | `LeaderEngine`: Lease/ConfigMap lock, `IsLeader`, `Subscribe`, `GetLeaderIP` |
| [`pkg/util/kubernetes/apiserver/controllers`](<<<SRC>>>/pkg/util/kubernetes/apiserver/controllers) | DCA core controllers: metadata (service↔pod mapping), legacy HPA/WPA autoscalers, informer registration |
| [`pkg/util/clusteragent/clusteragent.go`](<<<SRC>>>/pkg/util/clusteragent/clusteragent.go) | Node-agent-side `DCAClient`: retry-wrapped, Bearer-token HTTP client with a version handshake |
| [`comp/core/workloadmeta/collectors/catalog-clusteragent/options.go`](<<<SRC>>>/comp/core/workloadmeta/collectors/catalog-clusteragent/options.go) | DCA workloadmeta catalog: the `kubeapiserver` collector only |
| [`pkg/clusteragent/languagedetection/patcher.go`](<<<SRC>>>/pkg/clusteragent/languagedetection/patcher.go) | Annotates deployments with detected languages for SSI |
| [`pkg/clusteragent/kubeactions`](<<<SRC>>>/pkg/clusteragent/kubeactions) | Remote-config-driven Kubernetes actions (rollout restart, ...) |
| [`pkg/clusteragent/instrumentation`](<<<SRC>>>/pkg/clusteragent/instrumentation) | `DatadogInstrumentation` CRD controller |
| [`pkg/clusteragent/patcher`](<<<SRC>>>/pkg/clusteragent/patcher) | Shared leader-gated dynamic-client patcher used by language detection and workload autoscaling |
| [`pkg/clusteragent/README.md`](<<<SRC>>>/pkg/clusteragent/README.md) | In-repo overview of the `pkg/clusteragent` tree |

## What runs inside the DCA

The DCA embeds a full miniature agent — its own [collector](../checks/collector.md), [Autodiscovery](../checks/autodiscovery.md), [workloadmeta](workloadmeta.md) (with only the `kubeapiserver` collector), a local [tagger](tagger.md), an aggregator/[forwarder](../pipelines/forwarder.md) pipeline, and a [remote config](../configuration/remote-config.md) service and client — plus a set of cluster-only subsystems documented on sibling pages:

| Subsystem | Gate | Page |
|---|---|---|
| Cluster-check and endpoints-check dispatching | `cluster_checks.enabled` | [Cluster checks and endpoints checks](cluster-checks.md) |
| Admission controller (~12 mutating/validating webhooks) | `admission_controller.enabled` | [Admission controller](admission-controller.md) |
| External metrics provider for HPA, DatadogPodAutoscaler workload autoscaling | `external_metrics_provider.enabled`, `autoscaling.workload.enabled` | [Autoscaling](autoscaling.md) |
| Orchestrator explorer (cluster-scoped resource collection) | `orchestrator_explorer.enabled` | [Orchestrator explorer](orchestrator.md) |
| Cluster-level tag and metadata serving | `kubernetes_collect_metadata_tags`, `cluster_agent.kube_metadata_collection.*` | this page, below |

Smaller leader-only products started from `start()`: the language-detection patcher ([`pkg/clusteragent/languagedetection`](<<<SRC>>>/pkg/clusteragent/languagedetection)), remote-config Kubernetes actions ([`pkg/clusteragent/kubeactions`](<<<SRC>>>/pkg/clusteragent/kubeactions)), the compliance agent for control-plane CIS benchmarks (`compliance_config.enabled`, see [Compliance and SBOM](../ebpf/compliance.md)), the AppSec injector (`cluster_agent.appsec.injector.enabled`), the private action runner (`private_action_runner.enabled`), and the `DatadogInstrumentation` CRD controller (`instrumentation_crd_controller.enabled`).

The DCA also registers four Go core checks of its own — `kubernetes_apiserver` ([`pkg/collector/corechecks/cluster/kubernetesapiserver`](<<<SRC>>>/pkg/collector/corechecks/cluster/kubernetesapiserver), control-plane health and Kubernetes events), `kubernetes_state_core` ([`pkg/collector/corechecks/cluster/ksm`](<<<SRC>>>/pkg/collector/corechecks/cluster/ksm)), `helm`, and `orchestrator` — which it can run itself or hand off to cluster-check runners.

## Startup sequence

`start()` in [`cmd/cluster-agent/subcommands/start/command.go`](<<<SRC>>>/cmd/cluster-agent/subcommands/start/command.go) is a single Fx `OneShot` that wires the core bundle, forwarders (API-key checking disabled), the demultiplexer, the dedicated orchestrator forwarder, the event-platform forwarder, workloadmeta (`AgentType: ClusterAgent`), the local tagger, Autodiscovery, the collector, the remote-config service, and the IPC component, then proceeds roughly in this order:

1. Refuse to start without an `api_key`; expose Prometheus telemetry on `0.0.0.0:<metrics_port>` (default 5000).
1. Create the global `LeaderEngine` and — if cluster checks, language-detection reporting, or autoscaling failover are enabled — the global `LeaderForwarder`.
1. Start the API server on port 5005 (`api.StartServer`) **before** connecting to the apiserver, so investigation endpoints are reachable during a slow startup.
1. Block in `apiserver.WaitForAPIClient` until the Kubernetes API is reachable, then resolve the hostname.
1. Start the "core" controllers via `StartControllers` in [`pkg/util/kubernetes/apiserver/controllers`](<<<SRC>>>/pkg/util/kubernetes/apiserver/controllers): the metadata controller (gated by `kubernetes_collect_metadata_tags`, default true), the legacy HPA autoscalers controller, services/endpoints informers for cluster checks, and the instrumentation controller. Informer factories start after registration; `apiserver.SyncInformers` waits for cache sync.
1. Compute the RFC1123-compliant cluster name and generate or load the **cluster ID** — a UUID persisted in the ConfigMap `datadog-cluster-id` via `GetOrCreateClusterID` in [`pkg/util/kubernetes/apiserver/common/common.go`](<<<SRC>>>/pkg/util/kubernetes/apiserver/common/common.go). If autoscaling is enabled and no cluster name can be detected, startup is fatal.
1. Initialize the remote-config client, subscribing to `AGENT_CONFIG` plus per-feature products: `APM_TRACING` (deployment patcher), `CONTAINER_AUTOSCALING_SETTINGS`/`VALUES` (workload autoscaling), `CLUSTER_AUTOSCALING_VALUES`, `K8S_ACTIONS`, and `GRADUAL_ROLLOUT`. The client identifies itself with the cluster name and cluster ID.
1. Load components and register the DCA core checks, then start Autodiscovery (`ac.LoadAndRun`).
1. Conditionally start the products listed above: the cluster-check `Handler`, the external-metrics APIService server, workload/cluster autoscaling controllers, the admission controller, and the smaller leader-only products.
1. Wait for SIGINT/SIGTERM; on shutdown, report health and wait for the external-metrics and admission servers to drain.

## The DCA API on port 5005

One TCP listener on `cluster_agent.cmd_port` (5005) serves both HTTP REST and gRPC, dispatched by content type through `helpers.NewMuxedGRPCServer` in [`cmd/cluster-agent/api/server.go`](<<<SRC>>>/cmd/cluster-agent/api/server.go). TLS comes from the IPC component's server config with `MinVersion` forced to TLS 1.3, unless `cluster_agent.allow_legacy_tls` relaxes it to TLS 1.0 for very old node agents.

### Two auth tokens

The `validateToken` middleware distinguishes two credentials on the same port:

1. The **DCA token** (`cluster_agent.auth_token`, env `DD_CLUSTER_AGENT_AUTH_TOKEN`) is the cluster-wide shared secret between node agents and the DCA, typically distributed as a Kubernetes Secret by Helm or the Operator. It is sent as `Authorization: Bearer <token>` and created or loaded through `CreateOrGetClusterAgentAuthToken` in [`pkg/api/security`](<<<SRC>>>/pkg/api/security), which falls back to a `cluster_agent.auth_token` file next to `datadog.yaml`.
1. The **local IPC token** is the per-pod `auth_token` from the IPC component (see [Inter-process communication](../processes/ipc.md)), used by intra-pod CLI commands such as `datadog-cluster-agent status` and `flare`.

`isExternalPath` hardcodes the list of node-agent-facing paths (`/api/v1/clusterchecks/*`, `/api/v1/endpointschecks/*`, `/api/v1/tags/*`, `/api/v1/annotations/node/*`, `/api/v1/cluster/id`, `/api/v1/languagedetection`, `/api/v2/series`, `/version`, the Cloud Foundry paths, ...). External paths accept **only** the DCA token; internal paths try the IPC token first and fall back to the DCA token.

/// warning
When adding a new node-agent-facing endpoint, you must extend `isExternalPath` in [`server.go`](<<<SRC>>>/cmd/cluster-agent/api/server.go). If you forget, node agents get 401s while your local CLI testing (which uses the IPC token on internal paths) passes.
///

### gRPC services

The gRPC side registers `pb.AgentSecureServer` with a per-RPC interceptor doing a constant-time compare against the DCA token:

- `TaggerStreamEntities` / `TaggerFetchEntity` — the DCA's [tagger](tagger.md) streamed to cluster-check runners ("cluster tagger"; runners enable it via `clc_runner_remote_tagger_enabled`, default true). Message size is capped by `cluster_agent.cluster_tagger.grpc_max_message_size` (4 MiB default).
- `StreamKubeMetadata` ([`kubernetes_metadata_stream.go`](<<<SRC>>>/cmd/cluster-agent/api/v1/kubernetes_metadata_stream.go)) — a server-push stream of pod→service mappings, namespace metadata, and Kueue entities to node agents, replacing the older polling endpoints for agents that support it.

Routers stay mutable after startup: `api.ModifyAPIRouter` lets the cluster-check and instrumentation setup install their endpoints later in the boot sequence.

## Leader election

Multiple DCA replicas coordinate through Kubernetes leader election so that exactly one replica runs controllers, dispatches checks, patches resources, and queries Datadog. The `LeaderEngine` in [`leaderelection.go`](<<<SRC>>>/pkg/util/kubernetes/apiserver/leaderelection/leaderelection.go) wraps client-go's `LeaderElector`; the identity is the pod name, and the lock is named `leader_lease_name` (default `datadog-leader-election`) in the resources namespace.

The lock type comes from `leader_election_default_resource`. The raw agent default is `configmap` (using a vendored ConfigMap lock); when the value is empty or unknown, `CanUseLeases` auto-detects Lease support via API discovery. Helm and Operator deployments set it to `lease` explicitly. Lease duration is `leader_lease_duration` (60 s).

Consumers gate on leadership in two ways: polling `le.IsLeader()` or subscribing to `le.Subscribe()` for change notifications. `GetLeaderIP()` resolves the leader pod's IP through the DCA Service's Endpoints or EndpointSlices, cached for five minutes, and returns the empty string when the caller *is* the leader — several subsystems, including the cluster-check handler, derive their own leadership state from that empty-string convention.

Two properties are easy to miss:

- `leader_election` defaults to **false**. With a single replica and no leader election, the cluster-check handler assumes permanent leadership. Multi-replica deployments must enable it.
- **Losing leadership does not restart the process.** `runLeaderElection` loops forever and re-runs the elector; every subsystem must react to leadership transitions through callbacks or subscriptions, not by assuming a fresh process.

### Follower-to-leader forwarding

Followers are not passive: they serve metadata and tagger reads from their own workloadmeta, and transparently reverse-proxy leader-only calls. `LeaderForwarder` in [`pkg/clusteragent/api/leader_forwarder.go`](<<<SRC>>>/pkg/clusteragent/api/leader_forwarder.go) is an `httputil.ReverseProxy` to `https://<leaderIP>:5005` with `InsecureSkipVerify: true` — trust is the bearer token, not TLS certificates. It tags forwarded requests with `X-DCA-Follower-Forwarded: true` and responses with `X-DCA-Forwarded`. Two usage patterns exist: the cluster-check handler's `RejectOrForwardLeaderQuery` (leader answers, follower forwards, unknown state returns 503 "Startup in progress"), and the generic `WithLeaderProxyHandler` wrapper in [`leader_handler.go`](<<<SRC>>>/pkg/clusteragent/api/leader_handler.go) used by `/api/v1/languagedetection` and `/api/v2/series`.

## Cluster-level metadata and tag propagation

The DCA serves Kubernetes metadata to node agents through three generations of plumbing, all consumed on the node side through `DCAClient` in [`pkg/util/clusteragent/clusteragent.go`](<<<SRC>>>/pkg/util/clusteragent/clusteragent.go):

1. The **metadata controller** ([`metadata_controller.go`](<<<SRC>>>/pkg/util/kubernetes/apiserver/controllers/metadata_controller.go), gated by `kubernetes_collect_metadata_tags`) watches Endpoints (or EndpointSlices when `kubernetes_use_endpoint_slices` is set) and builds per-node bundles mapping pods to the Kubernetes services that select them. Node agents poll `GET /api/v1/tags/pod/{nodeName}` every `kubernetes_metadata_tag_update_freq` (60 s), and their tagger turns the result into `kube_service` tags.
1. The **workloadmeta `kubeapiserver` collector** ([`comp/core/workloadmeta/collectors/internal/kubeapiserver`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal/kubeapiserver), DCA-only) fills DCA workloadmeta with `KubernetesMetadata` entities: node labels and annotations, namespace labels and annotations, deployments, and — with `cluster_agent.kube_metadata_collection.enabled` and `.resources` — arbitrary resource types for generic metadata-as-tags. HTTP endpoints in [`kubernetes_metadata.go`](<<<SRC>>>/cmd/cluster-agent/api/v1/kubernetes_metadata.go) serve from it: `/api/v1/tags/node/{node}`, `/api/v1/annotations/node/{node}`, `/api/v1/tags/namespace/{ns}`, `/api/v1/metadata/namespace/{ns}`, `/api/v1/uid/node/{node}`.
1. The **gRPC `StreamKubeMetadata` stream** pushes the same pod-services and namespace metadata (plus Kueue entities when `cluster_agent.kueue.enabled`) to node agents, replacing polling for agents that support it, with a keep-alive event roughly every nine minutes.

`GET /api/v1/cluster/id` returns the cluster UUID, which node agents need for the [orchestrator explorer](orchestrator.md) and remote config.

Language detection follows the reverse path: node agents POST detected process languages to `/api/v1/languagedetection` (leader-proxied), the handler merges them per owner (deployment, statefulset) into DCA workloadmeta, and the leader-only patcher ([`patcher.go`](<<<SRC>>>/pkg/clusteragent/languagedetection/patcher.go)) annotates the owner resources with `internal.dd.datadoghq.com/<container>.detected_langs` — annotations the [APM single-step instrumentation (SSI) admission webhook](admission-controller.md) later reads to pick tracer libraries.

## Configuration

All keys live in [`pkg/config/setup/common_settings.go`](<<<SRC>>>/pkg/config/setup/common_settings.go) unless noted; env vars use the `DD_` prefix with dots replaced by underscores.

| Key | Default | Meaning |
|---|---|---|
| `cluster_agent.enabled` | false | Node-agent-side switch to use the DCA |
| `cluster_agent.cmd_port` | 5005 | DCA API port (HTTPS + gRPC) |
| `cluster_agent.kubernetes_service_name` | `datadog-cluster-agent` | Service through which node agents resolve the DCA (via `<SVC>_SERVICE_HOST/PORT` env vars); `cluster_agent.url` overrides |
| `cluster_agent.auth_token` | — | Cluster-wide shared DCA token |
| `cluster_agent.allow_legacy_tls` | false | Relax the API server to TLS 1.0 |
| `cluster_agent.max_leader_connections` | 100 | Connection pool for the follower→leader forwarder |
| `leader_election` | false | Enable leader election (required for multiple replicas) |
| `leader_lease_name` | `datadog-leader-election` | Lock object name |
| `leader_election_default_resource` | `configmap` | Lock type; Helm/Operator set `lease` |
| `leader_lease_duration` | 60 s | Lease TTL |
| `metrics_port` | 5000 | Prometheus self-telemetry |
| `kubernetes_collect_metadata_tags` | true | Run the metadata (service↔pod) controller |
| `kubernetes_metadata_tag_update_freq` | 60 s | Node-agent poll frequency for pod-service bundles |
| `cluster_agent.kube_metadata_collection.enabled` | false | Collect arbitrary resources into workloadmeta for metadata-as-tags |
| `cluster_agent.cluster_tagger.grpc_max_message_size` | 4 MiB | Cap on cluster-tagger gRPC messages |
| `cluster_name` | — | Cluster name; validated and combined into tags by the dispatcher and checks |
| `cluster_agent.tracing.enabled` | false | Self-instrumentation with dd-trace-go |

## Deployment modes

- **Standard Helm/Operator Kubernetes:** node agents run as a DaemonSet with `DD_CLUSTER_AGENT_ENABLED=true`, the service name, and the shared token. With the DCA present, node agents stop watching the apiserver for tags and events: kube metadata comes from the DCA, and Kubernetes event collection moves to the DCA's `kubernetes_apiserver` check.
- **Host installs and plain Docker:** no DCA. Node agents either hit the apiserver directly (legacy mode) or skip cluster-level tags entirely.
- **Single vs. multiple replicas:** one replica with `leader_election: false` assumes permanent leadership; multiple replicas require `leader_election: true`, with followers serving reads and proxying leader-only writes.
- **Cluster-check runners:** a dedicated Deployment of node agents in CLC-runner mode offloads cluster checks; see [Cluster checks and endpoints checks](cluster-checks.md).
- **Fargate:** no DaemonSet is possible, so the `agent_sidecar` admission webhook injects an agent container into application pods; see [Admission controller](admission-controller.md).

## Ports

| Port | Protocol | What |
|---|---|---|
| 5005 (`cluster_agent.cmd_port`) | HTTPS + gRPC, TLS ≥ 1.3 | DCA API: node-agent endpoints, intra-pod CLI, gRPC tagger and metadata streams |
| 5005 (`clc_runner_port`) | HTTPS | CLC-runner API queried *by* the DCA leader for runner stats (client: [`clcrunner.go`](<<<SRC>>>/pkg/util/clusteragent/clcrunner.go)) |
| 8000 (`admission_controller.port`) | HTTPS | Admission webhook server, called by the Kubernetes apiserver |
| 8443 (`external_metrics_provider.port`) | HTTPS | Kubernetes external-metrics APIService |
| 5000 (`metrics_port`) | HTTP | Prometheus `/metrics`, expvar, pprof |
| `health_port` (0 = off) | HTTP | Liveness/readiness probes |

Outbound, the DCA talks to the Datadog intake through its own forwarder (check metrics), the dedicated orchestrator forwarder (see [Orchestrator explorer](orchestrator.md)), remote-config polling, and — for autoscaling — the Datadog metrics query API.

## Gotchas

- **Leadership is inferred from `GetLeaderIP() == ""`.** During startup or rolling updates the state is `unknown`, and node agents receive 503 "Startup in progress" — expected and retried.
- **Two tokens, one hardcoded path list.** External paths accept only the DCA token, internal paths accept both; the trap is testing a new endpoint only with the local CLI token and never exercising the node-agent path (see the warning above).
- **`leader_election_default_resource` defaults to `configmap`.** Lease locking is only auto-detected when the value is empty or unknown; if RBAC only grants `leases`, an unset Helm value can strand the election.
- **The follower→leader proxy skips TLS verification** by design; do not "fix" `InsecureSkipVerify` without replacing the trust model (bearer token) with certificate identities.
- **The cluster ID lives in the `datadog-cluster-id` ConfigMap.** Deleting it mints a new identity, which resets orchestrator-explorer resource lineage and remote-config client identity.
- **The DCA is a check runner too.** Its core checks (`kubernetes_apiserver`, KSM, orchestrator, helm) can also be configured as cluster checks dispatched to CLC runners — configuring both paths double-collects.
