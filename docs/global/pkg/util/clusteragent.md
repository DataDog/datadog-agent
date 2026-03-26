> **TL;DR:** Provides the HTTP clients node agents use to query the Datadog Cluster Agent for Kubernetes metadata, cluster/endpoint check configurations, and language detection, serving as the main IPC boundary between node agents and the cluster agent.

# pkg/util/clusteragent

## Purpose

`pkg/util/clusteragent` provides HTTP clients used by **node agents** (and
cluster-check runners) to query the Datadog Cluster Agent (DCA) and the
Cluster Level Check Runner (CLC Runner) APIs.  It is the main IPC boundary
between an ordinary node agent and the cluster agent: all Kubernetes metadata,
cluster check configurations, and endpoint check configurations flow through
these clients.

The package exposes two independent clients:

- **`DCAClient`** — talks to the cluster agent REST API.
- **`CLCRunnerClient`** — talks to individual CLC Runner pods over their own
  REST API.

Both clients are singletons (one global instance per process) managed via lazy
initialization helpers.

## Key elements

### Key interfaces

#### DCAClient interface (`DCAClientInterface`)

`DCAClientInterface` is the public contract; the concrete `DCAClient` implements
it.  Always depend on the interface in production code to allow mocking in
tests.

```
DCAClientInterface
  Version(withRefresh bool) version.Version
  ClusterAgentAPIEndpoint() string

  // Kubernetes metadata (backed by the kube API server cache on the DCA)
  GetNodeLabels(nodeName string) (map[string]string, error)
  GetNodeAnnotations(nodeName string, filter ...string) (map[string]string, error)
  GetNodeInfo(nodeName string, filter ...string) (*NodeSystemInfo, error)
  GetNodeUID(nodeName string) (string, error)
  GetNamespaceLabels(nsName string) (map[string]string, error)
  GetNamespaceMetadata(nsName string) (*Metadata, error)
  GetPodsMetadataForNode(nodeName string) (apiv1.NamespacesPodsStringsSet, error)
  GetKubernetesMetadataNames(nodeName, ns, podName string) ([]string, error)
  GetCFAppsMetadataForNode(nodename string) (map[string][]string, error)

  // Cluster / distributed checks
  PostClusterCheckStatus(ctx, nodeName string, status types.NodeStatus) (types.StatusResponse, error)
  GetClusterCheckConfigs(ctx, nodeName string) (types.ConfigResponse, error)
  GetEndpointsCheckConfigs(ctx, nodeName string) (types.ConfigResponse, error)
  GetKubernetesClusterID() (string, error)

  // Language detection
  PostLanguageMetadata(ctx, *pbgo.ParentLanguageAnnotationRequest) error

  // Feature detection
  SupportsNamespaceMetadataCollection() bool
```

#### Obtaining a client

```go
dcaClient, err := clusteragent.GetClusterAgentClient()
```

`GetClusterAgentClient` returns the global singleton, initialising it on first
call.  Initialisation uses `retry.Retrier` with exponential back-off (1 s
initial, 5 min cap) so it is safe to call early at startup.

#### Connection details

- Endpoint is resolved from config via `utils.GetClusterAgentEndpoint()`.
- Authentication: `Bearer <cluster-agent-auth-token>` in every request header.
- TLS: cross-node client TLS config from `pkg/api/util.GetCrossNodeClientTLSConfig`.
- HTTP timeouts: dial 1 s, response header 3 s, total request 10 s.
- The client reconnects periodically if
  `cluster_agent.client_reconnect_period_seconds` is set.
- For DCA versions < 1.21 a **`leaderClient`** is used: it follows HTTP
  redirects from the service to the current leader pod and caches the leader
  URL to avoid unnecessary round-trips.  This compatibility shim will be removed
  in a future release.

#### Supporting types

| Type | Description |
|---|---|
| `Metadata` | Name, namespace, annotations, and labels for a Kubernetes resource. |
| `NodeSystemInfo` | Subset of `NodeSystemInfo` from the Kubernetes API (kernel, OS, runtime, kubelet version, arch). |

#### API paths (DCA)

| Method | Path |
|---|---|
| `GetNodeLabels` | `GET api/v1/tags/node/{name}` |
| `GetNamespaceLabels` | `GET api/v1/tags/namespace/{name}` |
| `GetNodeUID` | `GET api/v1/uid/node/{name}` |
| `GetNamespaceMetadata` | `GET api/v1/metadata/namespace/{name}` |
| `GetNodeAnnotations` | `GET api/v1/annotations/node/{name}[?filter=…]` |
| `GetNodeInfo` | `GET api/v1/info/node/{name}[?filter=…]` |
| `GetCFAppsMetadataForNode` | `GET api/v1/tags/cf/apps/{node}` |
| `GetPodsMetadataForNode` | `GET api/v1/tags/pod/{node}` |
| `GetKubernetesMetadataNames` | `GET api/v1/tags/pod/{node}/{ns}/{pod}` |
| `GetKubernetesClusterID` | `GET api/v1/cluster/id` |
| `PostClusterCheckStatus` | `POST api/v1/clusterchecks/status/{id}` |
| `GetClusterCheckConfigs` | `GET api/v1/clusterchecks/configs/{id}` |
| `GetEndpointsCheckConfigs` | `GET api/v1/endpointschecks/configs/{node}` |
| `PostLanguageMetadata` | `POST api/v1/languagedetection` |

#### CLCRunnerClient interface (`CLCRunnerClientInterface`)

Talks directly to individual CLC Runner pod IPs over HTTPS on the port
configured by `cluster_checks.clc_runners_port`.

```
CLCRunnerClientInterface
  GetVersion(IP string) (version.Version, error)
  GetRunnerStats(IP string) (types.CLCRunnersStats, error)
  GetRunnerWorkers(IP string) (types.Workers, error)
```

```go
clcClient, err := clusteragent.GetCLCRunnerClient()
```

### Key functions

#### Diagnostics

Importing the package registers a `"Cluster Agent availability"` metadata
diagnose check (via `diagnoseComp.RegisterMetadataAvail`) that fires
`GetClusterAgentClient()` to verify the connection is healthy.  This check
surfaces in the output of `agent diagnose`.

## Usage

**Workload-metadata kubemetadata collector**
(`comp/core/workloadmeta/collectors/internal/kubemetadata/`) calls
`GetClusterAgentClient()` to resolve Kubernetes service metadata for pods on
the local node.

**Cluster check config provider**
(`comp/core/autodiscovery/providers/clusterchecks.go`) calls
`GetClusterCheckConfigs` and `PostClusterCheckStatus` to pull check
configurations assigned to the local node and report health back to the DCA.

**Endpoints check config provider**
(`comp/core/autodiscovery/providers/endpointschecks.go`) calls
`GetEndpointsCheckConfigs` to pull endpoint check configs for the local node.

**Language detection client**
(`comp/languagedetection/client/impl/client.go`) calls
`PostLanguageMetadata` to forward detected process language annotations to the
DCA for Kubernetes annotation patching.

**Cluster check dispatcher** (`pkg/clusteragent/clusterchecks/dispatcher_main.go`)
and the **cluster-agent API handler** (`cmd/cluster-agent/api/v1/clusterchecks.go`)
use `CLCRunnerClient` to fetch stats and worker counts from CLC Runner pods for
load-balancing decisions.

### Testing

The interface `DCAClientInterface` is designed for mocking.  Tests typically
create a struct that embeds `DCAClientInterface` and overrides the methods under
test.  See `clusteragent_test.go`, `clusterchecks_test.go`, and
`endpointschecks_test.go` for examples.

Call `resetGlobalClusterAgentClient()` (unexported, test-only) to clear the
singleton between test cases.

## Cross-references

| Related package / component | Relationship |
|-----------------------------|--------------|
| [`pkg/clusteragent`](../clusteragent/clusteragent.md) | The server side of this IPC boundary. The DCA's `clusterchecks.Handler` serves the endpoints that `DCAClient` calls (`GetClusterCheckConfigs`, `PostClusterCheckStatus`). The `cmd/cluster-agent/api/v1/clusterchecks.go` HTTP handlers delegate to `Handler.GetConfigs` and `Handler.PostStatus`. |
| [`comp/core/ipc`](../../comp/core/ipc.md) | For intra-process IPC (CLI → daemon, process-agent → core agent) the agent uses `comp/core/ipc`'s bearer-token + mTLS transport. `pkg/util/clusteragent` provides its own cross-node bearer-token authentication (via `cluster-agent-auth-token`) and TLS config (`pkg/api/util.GetCrossNodeClientTLSConfig`) for node-agent → cluster-agent communication, which is a different trust domain. |
| [`pkg/util/http`](http.md) | `DCAClient` builds its `http.Client` using `pkg/api/util.GetCrossNodeClientTLSConfig` for TLS. For general outbound HTTP (proxies, transport options) the rest of the agent uses `pkg/util/http.CreateHTTPTransport`; the DCA client bypasses this because it targets an internal cluster endpoint where proxy configuration does not apply. |
| [`comp/core/autodiscovery`](../../comp/core/autodiscovery.md) | The `clusterchecks` and `endpointschecks` config providers (under `comp/core/autodiscovery/providers/`) are the primary consumers of `DCAClient`. They call `GetClusterCheckConfigs` / `GetEndpointsCheckConfigs` on each poll interval and feed the returned `integration.Config` values into the autodiscovery engine via `Scheduler.Schedule`. |
