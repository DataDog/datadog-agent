# MCP Metrics Viewer - Technical Design

## Architecture Overview

The MCP server runs as a Kubernetes Deployment that discovers fine-grained-monitor
DaemonSet pods and routes node-targeted queries to the correct pod. It exposes
MCP tools via HTTP/SSE transport, enabling access from both local development
(via port-forward) and remote AI SRE agents.

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                            Kubernetes Cluster                                 │
│                                                                               │
│  ┌──────────────────────────────────────────────────────────────────────────┐│
│  │                    Namespace: fine-grained-monitor                        ││
│  │                                                                           ││
│  │  ┌────────────────────┐       ┌────────────────────────────────┐         ││
│  │  │  MCP Deployment    │       │  fine-grained-monitor DaemonSet │         ││
│  │  │  (fgm-mcp-server)  │       │                                 │         ││
│  │  │                    │       │   Node A        Node B      ... │         ││
│  │  │  - watches pods    │ HTTP  │   ┌──────┐      ┌──────┐        │         ││
│  │  │  - routes by node  │◄─────►│   │viewer│      │viewer│        │         ││
│  │  │  - HTTP/SSE MCP    │       │   │:8050 │      │:8050 │        │         ││
│  │  │                    │       │   └──────┘      └──────┘        │         ││
│  │  └─────────┬──────────┘       │   (node-local   (node-local     │         ││
│  │            │                  │    metrics)      metrics)       │         ││
│  │   Service (ClusterIP)         └────────────────────────────────┘         ││
│  │   fgm-mcp-server:8080                                                     ││
│  │                                                                           ││
│  │   RBAC: Role (namespace-scoped, least-privilege)                          ││
│  └──────────────────────────────────────────────────────────────────────────┘│
└──────────────┼───────────────────────────────────────────────────────────────┘
               │
       ┌───────┴───────┐
       │               │
   Claude Code    AI SRE Agents
  (port-forward)  (direct/ingress)
```

All components run in the `fine-grained-monitor` namespace, enabling namespace-scoped
RBAC. The MCP server only needs to watch pods in its own namespace, so a Role
(not ClusterRole) provides sufficient access with least-privilege security.

### Key Design Principles

1. **Node locality is explicit:** Each DaemonSet pod only has metrics for its own
   node's containers. The MCP server makes this visible to agents via `list_nodes`
   and requires node targeting on container/analysis queries.

2. **MCP is the routing layer:** The MCP Deployment handles pod discovery and
   request routing. Agents don't need to understand Kubernetes internals—they
   see nodes, not pods or IPs.

3. **No aggregation:** The MCP server always queries a single node. Cluster-wide
   analysis is the agent's responsibility (iterate nodes, synthesize results).

## Design Decisions

### Discovery and Routing Strategy

**Decision:** Use Kubernetes API list/watch for discovery, route directly to pod IP.

| Alternative | Pros | Cons |
|-------------|------|------|
| **API + Pod IP (chosen)** | Simple, rich metadata, no extra resources | Pod IP changes on restart |
| Headless Service DNS | Stable per-pod hostname | Extra Service resource, less metadata |
| EndpointSlices | "Proper" k8s pattern | Same IP instability, more complex |

**Rationale:** For a research project, simplicity wins. The watch handles IP
changes quickly (<1s typically). Production deployments should consider DNS-based
routing via headless Service for stability.

### Staleness and Caching

| Condition | Detection | Effect |
|-----------|-----------|--------|
| Kubernetes API unreachable | `now - last_k8s_sync_ms > 120s` | `watcher_stale: true`, all routing operations fail fast |
| Individual pod disappeared | `now - last_observed_ms > 60s` AND not in latest resync | `stale: true` for that node, operations to that node fail |

**Timestamps:**
- `last_k8s_sync_ms`: updated on any proof-of-life (successful list, watch reconnect, any watch event)
- `last_observed_ms`: per-node, updated when pod is seen in list/resync

**Resync cadence:** Force periodic relist every 60 seconds (not the kube-rs default
of ~5 minutes). This ensures quiet clusters remain healthy—stable pods generate
zero watch events, so without periodic relist they would appear stale.

**Behavior when watcher is stale:**
- `list_nodes`: Returns cached data with `watcher_stale: true` (visibility into state)
- All other tools: Fail fast with error "watcher is stale"

### Ready vs Serving

"Ready" means Kubernetes PodReady condition is true. For production reliability,
readiness must actually indicate "viewer HTTP server is responding."

**Viewer DaemonSet pods must have:**
- `readinessProbe`: HTTP GET to `/api/health` (or similar lightweight endpoint)
- `startupProbe`: Same endpoint, with longer timeout for initialization

**MCP Deployment must have:**
- `readinessProbe`: Verify watcher is connected and cache is populated
- `livenessProbe`: Basic health check

Once readiness is meaningful, routing logic simplifies: "Ready" actually means
"will answer HTTP." This eliminates spurious timeouts and agent flakiness.

### Heterogeneous Metrics

`list_metrics` returns metrics from one node. If clusters have nodes with
different metric schemas (e.g., different kernel versions), agents should:
1. Call `list_nodes` to enumerate nodes
2. Call `list_metrics` per-node to discover each node's available metrics

## Pod Discovery (REQ-MCP-007, REQ-MCP-008)

The MCP server discovers DaemonSet pods using the Kubernetes API:

```
PodWatcher
  - namespace: String (e.g., "default")
  - label_selector: String (e.g., "app=fine-grained-monitor")
  - cache: HashMap<NodeName, PodInfo>

PodInfo (internal, not exposed to agents)
  - node_name: String
  - pod_name: String
  - pod_ip: String
  - ready: bool
  - last_seen: Timestamp

Methods:
  - start() -> watch pods, update cache on changes
  - list_nodes() -> Vec<NodeInfo>  (sanitized, no pod details)
  - get_pod_for_node(node: &str) -> Option<PodInfo>
```

The watcher uses `kube-rs` to list/watch pods with the label selector, updating
an in-memory cache. The cache maps node names to pod info, enabling O(1) routing.
Pod details (IP, name) are internal implementation—agents only see node names.

## Node Routing

When a tool call specifies a node:

1. Look up node in pod cache
2. If not found → return error "node 'X' not found"
3. If found but not ready → return error "node 'X' is not ready"
4. If found but stale (not observed in recent resync) → return error "node 'X' is stale"
5. Construct URL: `http://{pod_ip}:8050/api/...`
6. Forward request with operation-specific timeout (see below)
7. On failure, apply retry policy (see below)
8. Return response or error

### Operation Timeouts

Different operations have different latency characteristics:

| Operation | Timeout | Rationale |
|-----------|---------|-----------|
| list_metrics | 5s | Small response, fast |
| list_containers | 5s | Bounded by limit parameter |
| analyze_container | 30s | Analysis can scan large windows |

Timeouts are hardcoded; no configuration surface exposed (KISS).

### Retry Policy

On any error except 4xx, retry once after re-resolving node→pod. Add 50-200ms
jitter before retry.

- **4xx from viewer:** Don't retry (client error)
- **All other errors:** Retry once, re-resolve node→pod first (IP may have changed)

All operations are read-only and safe to retry.

### Multiple Pods Per Node

During DaemonSet rollouts, multiple pods may exist for the same node briefly.

**Policy:** Select the pod that is:
1. Ready (PodReady condition true)
2. If multiple ready, pick the one with newest `creationTimestamp`

This ensures we route to the new pod during rolling updates.

## Tool Schemas

### list_nodes (REQ-MCP-007)

**Input:** None

**Output:**
```json
{
  "watcher_stale": false,
  "last_sync_ms": 1704067200000,
  "nodes": [
    {
      "name": "worker-1",
      "ready": true,
      "stale": false,
      "last_observed_ms": 1704067200000
    }
  ]
}
```

**Field definitions:**
- `watcher_stale`: true if `now - last_sync_ms > 120s` (Kubernetes API may be unreachable)
- `last_sync_ms`: timestamp of last successful Kubernetes list/watch resync
- `ready`: Kubernetes PodReady condition is true
- `stale`: true if `now - last_observed_ms > 60s` AND pod not in latest resync
  (pre-computed so agents don't need to implement staleness logic)
- `last_observed_ms`: per-node timestamp when pod was last seen in a resync

Note: Pod details (IP, name) are intentionally hidden. Agents route by node name.
Agents should check `watcher_stale` first, then use nodes where `ready && !stale`.

### list_metrics (REQ-MCP-001)

**Input:**
- `node` (required): Node name to query

**Output:**
```json
{
  "node": "worker-1",
  "metrics": [{ "name": "cgroup.v2.cpu.stat.usage_usec" }],
  "studies": [{ "id": "periodicity", "name": "Periodicity Study", "description": "..." }]
}
```

**Design note:** `node` is required, consistent with the "node locality is explicit"
principle. Agents should call `list_nodes` first to discover available nodes.

### list_containers (REQ-MCP-002, REQ-MCP-003, REQ-MCP-008)

**Input:**
- `node` (required): Node name to query
- `metric` (optional): Metric name to filter containers (only those with data for this metric)
- `namespace` (optional): Kubernetes namespace filter (exact match)
- `qos_class` (optional): QoS class filter (exact match)
- `pod_name` (optional): Pod name prefix filter
- `container_name` (optional): Container name prefix filter
- `limit` (optional): Max results per page (default 20, max 100)
- `cursor` (optional): Pagination cursor from previous response

**Output:**
```json
{
  "node": "worker-1",
  "containers": [{
    "id": "abc123def456",
    "short_id": "abc123def456",
    "pod_name": "nginx-xyz",
    "container_name": "nginx",
    "namespace": "default",
    "qos_class": "Burstable",
    "last_seen": 1704067200000
  }],
  "total_matching": 47,
  "next_cursor": "eyJvZmZzZXQiOjIwfQ=="
}
```

**Pagination semantics:**
- `cursor` is opaque. Clients must not decode, inspect, or construct cursors.
- **Best-effort pagination:** Results are sorted by recency (`last_seen` descending).
  Between page fetches, containers may appear/disappear and shift ordering. Clients
  may see duplicates or miss items across pages. This is acceptable for research;
  production use cases requiring stable pagination should snapshot results.
- `next_cursor` is null/absent when no more results exist.

### analyze_container (REQ-MCP-004, REQ-MCP-005, REQ-MCP-008)

**Input:**
- `node` (required): Node name to query
- `container` (required): Container ID (short 12-char or full 64-char)
- `study_id` (required): "periodicity" or "changepoint"
- `metric` (required): Metric name to analyze

**Output:**
```json
{
  "node": "worker-1",
  "container": { "id": "abc123def456", "pod_name": "nginx-xyz", "namespace": "default" },
  "study": "periodicity",
  "metric": "cgroup.v2.cpu.stat.usage_usec",
  "stats": { "avg": 1234.5, "max": 5000, "min": 100, "stddev": 500, "trend": "stable", "sample_count": 1000 },
  "findings": [{ "type": "periodicity", "timestamp_ms": 1704067200000, "label": "period=60000ms", "details": {...} }]
}
```

**Design note:** `metric` is required (not optional, no prefix matching). This avoids
unbounded analysis cost. Agents should call `list_metrics` first to discover available
metrics, then call `analyze_container` once per metric of interest.

**Container ID collision handling:**

If a short ID matches multiple containers on the node, return an error:
`"Container ID 'abc123' is ambiguous. Use full ID from list_containers."`

## HTTP/SSE Transport (REQ-MCP-006)

The MCP server uses `rmcp` with HTTP/SSE transport:

```rust
use rmcp::transport::sse::SseServer;

let server = McpMetricsViewer::new(pod_watcher);
let sse = SseServer::new("0.0.0.0:8080");
server.serve(sse).await?;
```

Clients connect via:
- **Claude Code:** `kubectl port-forward svc/fgm-mcp-server -n fine-grained-monitor 8888:8080`, then configure MCP with `http://localhost:8888`
- **AI SRE Agents:** Direct HTTP to `http://fgm-mcp-server.fine-grained-monitor.svc.cluster.local:8080`

## Kubernetes Manifests

All manifests use namespace `fine-grained-monitor`. The DaemonSet runs in the same
namespace, so namespace-scoped RBAC (Role, not ClusterRole) is sufficient.

### Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: fgm-mcp-server
  namespace: fine-grained-monitor
  labels:
    app: fgm-mcp-server
spec:
  replicas: 1
  selector:
    matchLabels:
      app: fgm-mcp-server
  template:
    metadata:
      labels:
        app: fgm-mcp-server
    spec:
      serviceAccountName: fgm-mcp-server
      containers:
      - name: mcp
        image: fine-grained-monitor:latest
        command: ["/usr/local/bin/mcp-metrics-viewer"]
        args:
          - "--port=8080"
          - "--daemonset-label=app=fine-grained-monitor"
          # No --daemonset-namespace needed; defaults to same namespace
        ports:
          - containerPort: 8080
            name: mcp
        resources:
          requests:
            memory: "32Mi"
            cpu: "10m"
          limits:
            memory: "128Mi"
            cpu: "100m"
```

### Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: fgm-mcp-server
  namespace: fine-grained-monitor
spec:
  selector:
    app: fgm-mcp-server
  ports:
    - name: mcp
      port: 8080
      targetPort: 8080
```

### RBAC

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: fgm-mcp-server
  namespace: fine-grained-monitor
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: fgm-mcp-server
  namespace: fine-grained-monitor
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: fgm-mcp-server
  namespace: fine-grained-monitor
subjects:
  - kind: ServiceAccount
    name: fgm-mcp-server
    namespace: fine-grained-monitor
roleRef:
  kind: Role
  name: fgm-mcp-server
  apiGroup: rbac.authorization.k8s.io
```

## Configuration

The MCP server binary accepts:

- `--port` (default: 8080): HTTP/SSE listen port
- `--daemonset-namespace` (default: "fine-grained-monitor"): Namespace containing DaemonSet
- `--daemonset-label` (default: "app=fine-grained-monitor"): Label selector for pods
- `--viewer-port` (default: 8050): Port on viewer containers

Note: Timeouts and staleness thresholds are hardcoded (KISS). See "Operation Timeouts"
and "Staleness and Caching" sections for values.

## File Locations

| File | Purpose |
|------|---------|
| `Cargo.toml` | rmcp, kube-rs, reqwest dependencies |
| `src/bin/mcp_metrics_viewer.rs` | Binary entry point, HTTP/SSE server setup |
| `src/metrics_viewer/mcp/mod.rs` | MCP server, tool implementations |
| `src/metrics_viewer/mcp/pod_watcher.rs` | Kubernetes pod discovery and caching |
| `src/metrics_viewer/mcp/router.rs` | Node→pod routing logic |
| `deploy/mcp-deployment.yaml` | Kubernetes Deployment manifest |
| `deploy/mcp-service.yaml` | Kubernetes Service manifest |
| `deploy/mcp-rbac.yaml` | ServiceAccount, Role, RoleBinding |

## Error Handling

| Condition | Error Message |
|-----------|---------------|
| Node not found | "node 'X' not found" |
| Node not specified | "node parameter is required" |
| Node not ready | "node 'X' is not ready" |
| Node stale | "node 'X' is stale (last seen Xs ago)" |
| No nodes available | "no nodes available" |
| Ambiguous container ID | "Container ID 'X' matches N containers..." |
| HTTP timeout | "request to node 'X' timed out" |
| Viewer error | Forward HTTP status and message |

## Dependencies

| Crate | Version | Purpose |
|-------|---------|---------|
| rmcp | 0.12 | MCP SDK with SSE transport |
| kube | 0.98 | Kubernetes API client |
| k8s-openapi | 0.24 | Kubernetes types |
| reqwest | 0.12 | HTTP client for viewer calls |
| tokio | 1.x | Async runtime |

## Production Hardening

This section documents the production-readiness path. Not all items are required
for initial deployment, but they should be implemented before production use.

### High Availability

For production:
- `replicas: 2` (minimum)
- `podAntiAffinity` to spread across nodes
- `PodDisruptionBudget` with `minAvailable: 1`

SSE clients will reconnect on pod restarts; this is acceptable.

### Network Security

The MCP server exposes powerful introspection tools. Even read-only, they reveal:
- Namespace and pod names
- Container names and IDs
- Node topology

**Current posture:** ClusterIP-only. Only in-cluster callers can reach the service.
No authentication/authorization beyond Kubernetes network isolation.

**Recommended hardening:**
- NetworkPolicy: default-deny ingress, allow only from known agent namespaces
- If exposing via Ingress, add authentication (mTLS, OIDC, etc.)

### Probes

**MCP Deployment:**
```yaml
readinessProbe:
  httpGet:
    path: /health/ready
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10
livenessProbe:
  httpGet:
    path: /health/live
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 30
```

**Probe behavior:**
- **Readiness** (`/health/ready`): Returns OK only if watcher is connected and cache
  is populated. "Populated" means at least one successful list completed—even if
  zero pods were found (empty cluster or wrong label selector is a valid state).
- **Liveness** (`/health/live`): Returns OK if the process is alive and HTTP server
  is responsive. **Must NOT depend on watcher health.** If the Kubernetes apiserver
  has a brownout, liveness must still pass—otherwise Kubernetes will restart MCP
  pods in a loop, making the situation worse.

**Viewer DaemonSet (separate concern, but noted here):**
- `readinessProbe`: HTTP GET `/api/health`
- `startupProbe`: Same endpoint, longer timeout

### Watch Failure Handling

The Kubernetes watcher can fail due to:
- Watch stream ends (normal, reconnect)
- Transient apiserver errors (retry with backoff)
- 410 Gone / resourceVersion too old (full relist)
- Apiserver throttling (backoff)

**Implementation:** Use kube-rs `watcher()` which handles most of these. Add:
- Exponential backoff on repeated failures
- Health signal: expose `watcher_healthy` and `last_sync_age_seconds`

### Graceful Shutdown

For SSE connections:
- `terminationGracePeriodSeconds: 30`
- Signal handler to stop accepting new connections
- Allow in-flight requests to complete

## Observability

### Structured Logging

Every tool call should log:
- `tool`: tool name
- `node`: target node (if applicable)
- `container_id`: container ID (shortened, if applicable)
- `latency_ms`: request duration
- `outcome`: `success`, `user_error`, `transient_error`, `internal_error`
- `viewer_status`: HTTP status from viewer (if applicable)

Use JSON format for machine parsing.

### Metrics

Export via Prometheus/OpenMetrics or Datadog dogstatsd:

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `mcp_tool_requests_total` | counter | tool, outcome | Request count by tool and outcome |
| `mcp_tool_latency_seconds` | histogram | tool | Request latency distribution |
| `mcp_viewer_proxy_errors_total` | counter | node, error_type | Proxy errors by node |
| `mcp_watcher_sync_age_seconds` | gauge | | Time since last successful K8s sync |
| `mcp_nodes_total` | gauge | status | Node count by status (ready/not_ready/stale) |

### Alerting (Suggested)

- `mcp_watcher_sync_age_seconds > 300`: Watcher may be disconnected
- `rate(mcp_viewer_proxy_errors_total[5m]) > 0.1`: High error rate to viewers
- `mcp_nodes_total{status="ready"} == 0`: No ready nodes
