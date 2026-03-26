# pkg/util/kubernetes/apiserver

## Purpose

`pkg/util/kubernetes/apiserver` provides the authenticated Kubernetes API server client used by the Cluster Agent (and any component built with the `kubeapiserver` tag). It wraps `client-go` with:

- A **singleton** `APIClient` that carries all typed and dynamic Kubernetes clients, informer factories, and a retry-based initialization loop.
- Helpers for pod-to-service metadata mapping, event collection, endpoint/service entity ID construction, hostname resolution, and OpenShift detection.
- A **resource-type discovery cache** that maps between kind names and plural resource names.
- Sub-packages for **leader election** (`leaderelection/`), **cluster controllers** (`controllers/`), and cross-cutting utilities (`common/`).

Everything in this package requires the `kubeapiserver` build tag; most files also carry `//go:build kubeapiserver` at the top.

## Build tag

All production code in this package and its sub-packages is gated by `//go:build kubeapiserver`. Stub files (`apiserver_nocompile.go`, `controllers/metadata_controller_nocompile.go`) satisfy the interface when the tag is absent.

## Key elements

### `APIClient` struct (`apiserver.go`)

The central type. Obtain it via the package-level singletons:

```go
// Non-blocking — returns error if not yet connected
cl, err := apiserver.GetAPIClient()

// Blocking — waits until connected or context is cancelled
cl, err := apiserver.WaitForAPIClient(ctx)
```

`APIClient` fields fall into three categories:

**Regular clients** (short timeout, for direct API calls)

| Field | Type | Purpose |
|---|---|---|
| `Cl` | `kubernetes.Interface` | Core typed client |
| `DynamicCl` | `dynamic.Interface` | Dynamic client for CRDs / arbitrary resources |
| `ScaleCl` | `scale.ScalesGetter` | Scale sub-resource client (used by HPA/autoscaler) |
| `RESTMapper` | `meta.RESTMapper` | Lazy discovery-based REST mapper |

**Informer clients** (long/no timeout, for Watch/Informer calls)

| Field | Type | Purpose |
|---|---|---|
| `InformerCl` | `kubernetes.Interface` | Main client for informers |
| `DynamicInformerCl` | `dynamic.Interface` | Dynamic informer client |
| `CRDInformerClient` | `clientset.Interface` | CRD (apiextensions) informer client |
| `APISInformerClient` | `ApiregistrationV1Interface` | APIService informer client |
| `VPAInformerClient` | `vpa.Interface` | VerticalPodAutoscaler client |

**Informer factories** (lazy, started once, cannot be stopped)

| Field | Purpose |
|---|---|
| `InformerFactory` | Default `SharedInformerFactory` for all standard resources |
| `DynamicInformerFactory` | `DynamicSharedInformerFactory` for CRDs |
| `CertificateSecretInformerFactory` | Filtered factory for the Admission Controller certificate secret |
| `WebhookConfigInformerFactory` | Filtered factory for `MutatingWebhookConfiguration` |
| `APIExentionsInformerFactory` | CRD definition informer factory |

> **Important:** Informer factories cannot be stopped safely. Only use `InformerFactory` / `DynamicInformerFactory` for informers whose lifetime equals the agent's lifetime. For shorter-lived informers (e.g. leader-based or CLC-scoped) create a new factory from `InformerCl` / `DynamicInformerCl` directly.

**Configuration keys** (from `datadog.yaml`) that control client behaviour:

| Config key | Default | Effect |
|---|---|---|
| `kubernetes_kubeconfig_path` | `""` | If set, load kubeconfig from path; otherwise use in-cluster config |
| `kubernetes_apiserver_client_timeout` | — | Timeout for regular clients |
| `kubernetes_apiserver_informer_client_timeout` | — | Timeout for informer clients |
| `kubernetes_informers_resync_period` | — | Informer resync interval |
| `kubernetes_apiserver_tls_verify` | `true` | Enable TLS verification |
| `kubernetes_apiserver_use_protobuf` | `false` | Use protobuf encoding |

### Notable `APIClient` methods

```go
// Node metadata
func (c *APIClient) NodeLabels(nodeName string) (map[string]string, error)
func (c *APIClient) NodeAnnotations(nodeName string) (map[string]string, error)
func (c *APIClient) GetNodeForPod(ctx, namespace, podName string) (string, error)
func (c *APIClient) GetARandomNodeName(ctx context.Context) (string, error)

// Pod-to-service metadata (used by CLI commands)
func GetMetadataMapBundleOnAllNodes(cl *APIClient) (*apiv1.MetadataResponse, error)
func GetMetadataMapBundleOnNode(nodeName string) (*apiv1.MetadataResponse, error)
func GetPodMetadataNames(nodeName, ns, podName string) ([]string, error)

// Event collection (used by kubernetes_apiserver check)
func (c *APIClient) RunEventCollection(resVer string, lastListTime time.Time, ...) ([]*v1.Event, string, time.Time, error)

// ConfigMap token store (used for event de-duplication)
func (c *APIClient) GetTokenFromConfigmap(token string) (string, time.Time, error)
func (c *APIClient) UpdateTokenInConfigmap(token, tokenValue string, timestamp time.Time) error

// API server health / custom access
func (c *APIClient) IsAPIServerReady(ctx context.Context) (bool, error)
func (c *APIClient) GetRESTObject(path string, output runtime.Object) error
func (c *APIClient) ComponentStatuses() (*v1.ComponentStatusList, error)
func (c *APIClient) DetectOpenShiftAPILevel() OpenShiftAPILevel

// Remote command
func (c *APIClient) NewSPDYExecutor(...) (remotecommand.Executor, error)

// Informer factory helpers
func (c *APIClient) GetInformerWithOptions(resyncPeriod *time.Duration, options ...informers.SharedInformerOption) informers.SharedInformerFactory
```

### `MetadataMapperBundle` (`apiserver.go`, `services.go`)

Holds the per-node mapping of pods to Kubernetes Services. Stored in the shared in-memory cache under key `KubernetesMetadataMapping/<nodeName>`.

```go
type MetadataMapperBundle struct {
    Services apiv1.NamespacesPodsStringsSet
}

func NewMetadataMapperBundle() *MetadataMapperBundle
func (b *MetadataMapperBundle) ServicesForPod(ns, podName string) ([]string, bool)
func (b *MetadataMapperBundle) DeepCopy(old *MetadataMapperBundle) *MetadataMapperBundle
```

### Errors

```go
var ErrNotFound = errors.New("entity not found")
var ErrNotLeader = errors.New("not Leader")
```

### Informer sync helpers (`util.go`)

```go
func SyncInformers(informers map[InformerName]cache.SharedInformer, extraWait time.Duration) error
func SyncInformersReturnErrors(informers map[InformerName]cache.SharedInformer, extraWait time.Duration) map[InformerName]error
```

Block until all provided informers have synced their caches or the `kube_cache_sync_timeout_seconds` deadline is reached. Always call these after starting informers.

### Resource type cache (`resourcetypes.go`)

```go
func InitializeGlobalResourceTypeCache(discoveryClient discovery.DiscoveryInterface) error
func GetResourceType(kind, group string) (string, error)     // "Deployment","apps" -> "deployments"
func GetResourceKind(resource, apiGroup string) (string, error) // "deployments","apps" -> "Deployment"
func GetClusterResources() (map[string]ClusterResource, error)
func GetAPIGroup(apiVersion string) string   // "apps/v1" -> "apps"
func GetAPIVersion(apiVersion string) string // "apps/v1" -> "v1"
```

Prepopulated from the discovery API on first access and refreshed on cache misses. Backed by a single retrying goroutine to avoid request storms.

### Entity ID builders (`endpoints.go`, `services.go`)

```go
func EntityForEndpoints(namespace, name, ip string) string    // "kube_endpoint_uid://ns/name/ip"
func EntityForService(svc *v1.Service) string                 // "kube_service://ns/name"
func EntityForServiceWithNames(namespace, name string) string
func SearchTargetPerName(endpoints *v1.Endpoints, targetName string) (v1.EndpointAddress, error)
func SearchTargetPerNameInEndpointSlices(slices []*discv1.EndpointSlice, targetName string) (string, error)
```

### Client constructors for one-off use

```go
func GetClientConfig(timeout time.Duration, qps float32, burst int) (*rest.Config, error)
func GetKubeClient(timeout time.Duration, qps float32, burst int) (kubernetes.Interface, error)
func GetKubeSecret(namespace, name string) (map[string][]byte, error)
```

---

## Sub-packages

### `leaderelection/`

Implements Kubernetes leader election for the Cluster Agent using the `k8s.io/client-go/tools/leaderelection` library. The lock resource is either a `Lease` (Kubernetes >= 1.14) or a `ConfigMap` (older clusters).

**Key type: `LeaderEngine`**

```go
// Lifecycle
func CreateGlobalLeaderEngine(ctx context.Context) *LeaderEngine
func GetLeaderEngine() (*LeaderEngine, error)
func (le *LeaderEngine) EnsureLeaderElectionRuns() error
func (le *LeaderEngine) StartLeaderElectionRun()

// State
func (le *LeaderEngine) IsLeader() bool
func (le *LeaderEngine) GetLeader() string
func (le *LeaderEngine) GetLeaderIP() (string, error)

// Event-based subscription
func (le *LeaderEngine) Subscribe() (leadershipChangeNotif <-chan struct{}, isLeader func() bool)

// Diagnostics
func GetLeaderElectionRecord() (rl.LeaderElectionRecord, error)
func CanUseLeases(client discovery.DiscoveryInterface) (bool, error)
```

`Subscribe` returns a buffered notification channel and an `isLeader` closure. Callers that need event-driven leadership changes use `Subscribe`; callers that only need a point-in-time check use `IsLeader`.

**Configuration keys**

| Config key | Effect |
|---|---|
| `leader_lease_name` | Name of the Lease or ConfigMap used as the lock |
| `leader_lease_duration` | Duration of the lease (default: 60 s) |
| `leader_election_default_resource` | `"lease"` or `"configmap"` (auto-detected by default) |
| `cluster_agent.kubernetes_service_name` | Service name for resolving the leader's IP |

---

### `controllers/`

Runs the Kubernetes controllers needed by the Cluster Agent. All controllers are registered in a catalog and started in parallel by `StartControllers`.

```go
func StartControllers(ctx *ControllerContext) k8serrors.Aggregate
```

**`ControllerContext`** carries all shared dependencies:

```go
type ControllerContext struct {
    InformerFactory             informers.SharedInformerFactory
    APIExentionsInformerFactory apiextentionsinformer.SharedInformerFactory
    DynamicClient               dynamic.Interface
    DynamicInformerFactory      dynamicinformer.DynamicSharedInformerFactory
    Client                      kubernetes.Interface
    IsLeaderFunc                func() bool
    EventRecorder               record.EventRecorder
    WorkloadMeta                workloadmeta.Component
    DatadogClient               option.Option[datadogclient.Component]
    StopCh                      chan struct{}
}
```

**Registered controllers** (enabled by config key):

| Controller | Config key | Purpose |
|---|---|---|
| `metadataController` | `kubernetes_collect_metadata_tags` | Maps pods to services via Endpoints or EndpointSlices; populates `MetaBundleStore` |
| `autoscalersController` | `external_metrics_provider.enabled` | Processes HPA and WPA objects; pushes external metrics to Datadog |
| Services informer | `cluster_checks.enabled` | Registers services informer for cluster checks |
| Endpoints informer | `cluster_checks.enabled` | Registers endpoints informer for cluster checks |
| EndpointSlices informer | `cluster_checks.enabled` | Registers EndpointSlices informer when available |
| CRD informer | `cluster_checks.enabled && cluster_checks.crd_collection` | Registers CRD informer |

**`MetaBundleStore`** (`store.go`): thread-safe store for `MetadataMapperBundle` objects, keyed by node name, with per-node change notifications via `Subscribe`/`Unsubscribe`.

```go
func GetGlobalMetaBundleStore() *MetaBundleStore

func (m *MetaBundleStore) Get(nodeName string) (*apiserver.MetadataMapperBundle, bool)
func (m *MetaBundleStore) Subscribe(nodeName string) <-chan struct{}
func (m *MetaBundleStore) Unsubscribe(nodeName string, ch <-chan struct{})
```

---

### `common/`

Cross-cutting utilities that both `apiserver` and `leaderelection` packages consume.

```go
// Cluster identity
func GetOrCreateClusterID(coreClient corev1.CoreV1Interface) (string, error)
func GetKubeSystemUID(coreClient corev1.CoreV1Interface) (string, error)

// Self-identification
func GetSelfPodName() (string, error)  // reads DD_POD_NAME

// Server version
func KubeServerVersion(client discovery.ServerVersionInterface, timeout time.Duration) (*version.Info, error)

// Feature gates (reads /metrics from apiserver)
func ClusterFeatureGates(ctx context.Context, discoveryClient discovery.DiscoveryInterface, timeout time.Duration) (map[string]FeatureGate, error)

type FeatureGate struct {
    Name    string
    Stage   string
    Enabled bool
}
```

`GetOrCreateClusterID` first checks a cache, then looks for a `datadog-cluster-id` ConfigMap, and finally falls back to the `kube-system` namespace UID.

---

## Usage

### Obtaining the client

```go
// In startup code (Cluster Agent main):
apiClient, err := apiserver.WaitForAPIClient(ctx)

// In checks or components that should not block:
apiClient, err := apiserver.GetAPIClient()
```

### Leader election (Cluster Agent)

```go
le := leaderelection.CreateGlobalLeaderEngine(ctx)
if err := le.EnsureLeaderElectionRuns(); err != nil {
    return err
}
// Poll:
if le.IsLeader() { ... }

// Event-driven:
notif, isLeader := le.Subscribe()
for range notif {
    if isLeader() { ... }
}
```

### Starting controllers (Cluster Agent)

```go
ctx := &controllers.ControllerContext{
    Client:          apiClient.Cl,
    InformerFactory: apiClient.InformerFactory,
    IsLeaderFunc:    le.IsLeader,
    WorkloadMeta:    wmeta,
    StopCh:          stopCh,
    // ...
}
if errs := controllers.StartControllers(ctx); errs != nil {
    log.Errorf("controllers failed: %v", errs)
}
```

### Representative callers

- `pkg/collector/corechecks/cluster/kubernetesapiserver/` — uses `GetAPIClient`, `RunEventCollection`, `GetTokenFromConfigmap`.
- `pkg/collector/corechecks/cluster/orchestrator/` — uses `GetAPIClient` and informer factories to list all workload resources.
- `pkg/collector/corechecks/cluster/helm/` — uses `GetAPIClient` for secret listing.
- `pkg/collector/corechecks/cluster/ksm/` — uses `GetAPIClient` for node and component status queries.
- `comp/cluster-agent/` — wires together `CreateGlobalLeaderEngine`, `StartControllers`, and `WaitForAPIClient` at startup.

---

## Cross-references

| Related package / component | Relationship |
|-----------------------------|--------------|
| [`pkg/util/kubernetes`](kubernetes.md) | Lightweight sibling providing build-tag-free constants, name-parsing helpers, and `GetStandardTags`. The `apiserver` package and its sub-packages import these constants rather than re-defining them. |
| [`pkg/clusteragent`](../../pkg/clusteragent/clusteragent.md) | Primary consumer. Every sub-system in the Cluster Agent (`admission`, `clusterchecks`, `autoscaling`, `orchestrator`) obtains its Kubernetes clients and informer factories from `WaitForAPIClient` / `APIClient` fields. The `controllers.ControllerContext` struct directly embeds `APIClient` fields. |
| [`pkg/kubestatemetrics`](../../pkg/kubestatemetrics.md) | The KSM builder calls `GetAPIClient()` to obtain `APIClient.Cl` (for node/component-status queries) and `APIClient.InformerFactory` to create per-resource reflectors. The `cacheEnabledListerWatcher` wrapper in `kubestatemetrics/builder` uses `ResourceVersionMatch=NotOlderThan` to read from the API-server cache instead of etcd, reducing load — this pattern depends on having a reliable informer client (provided by `APIClient.InformerCl`). |
| [`comp/core/workloadmeta`](../../comp/core/workloadmeta.md) | The `kubeapiserver` workloadmeta collector (under `comp/core/workloadmeta/collectors/internal/kubeapiserver/`) uses `WaitForAPIClient` to get the client, then drives `KubernetesPod`, `KubernetesDeployment`, and `KubernetesMetadata` entity events into the store. `ControllerContext.WorkloadMeta` carries the workloadmeta component for use by the metadata controller. |

---

## Common pitfalls

### Informer factory lifetime

`APIClient.InformerFactory` and `APIClient.DynamicInformerFactory` **cannot be stopped**. Once `Start()` is called on a factory, the watch goroutines run for the process lifetime. For informers that should only run while the component is a leader (e.g. cluster-check-scoped informers), create a separate factory from `APIClient.InformerCl` or `APIClient.DynamicInformerCl` and stop it on leadership loss.

### Blocking vs. non-blocking client access

`WaitForAPIClient` blocks until the API server is reachable (or the context is cancelled). Use it in startup code. `GetAPIClient` returns immediately with an error if the client is not yet initialized — use it in checks or components that should degrade gracefully rather than delay startup.

### SyncInformers is mandatory

After starting informers, always call `SyncInformers` (or `SyncInformersReturnErrors`) before reading from caches. Skipping this causes race conditions where the cache appears empty and leads to incorrect or missing data on first check runs.

### OpenShift detection

`APIClient.DetectOpenShiftAPILevel()` probes for OpenShift-specific API groups. Call this once at startup and cache the result. Re-calling it on every check run adds unnecessary API-server load.
