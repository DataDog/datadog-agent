# MCP Metrics Viewer - Technical Design

## Architecture Overview

The MCP server runs as a Kubernetes Deployment that discovers fine-grained-monitor
DaemonSet pods and routes node-targeted queries to the correct pod. It exposes
MCP tools via HTTP/SSE transport, enabling access from both local development
(via port-forward) and remote AI SRE agents.

```
┌─────────────────────────────────────────────────────────────────┐
│                     Kubernetes Cluster                           │
│                                                                  │
│  ┌────────────────────┐       ┌────────────────────────────────┐│
│  │  MCP Deployment    │       │  fine-grained-monitor DaemonSet ││
│  │  (mcp-metrics)     │       │                                 ││
│  │                    │       │   Node A        Node B      ... ││
│  │  - watches pods    │ HTTP  │   ┌──────┐      ┌──────┐        ││
│  │  - routes by node  │◄─────►│   │viewer│      │viewer│        ││
│  │  - HTTP/SSE MCP    │       │   │:8050 │      │:8050 │        ││
│  │                    │       │   └──────┘      └──────┘        ││
│  └─────────┬──────────┘       │   (node-local   (node-local     ││
│            │                  │    metrics)      metrics)       ││
│   Service (ClusterIP)         └────────────────────────────────┘│
│   mcp-metrics:8080                                               │
└────────────┼─────────────────────────────────────────────────────┘
             │
     ┌───────┴───────┐
     │               │
 Claude Code    AI SRE Agents
(port-forward)  (direct/ingress)
```

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

- **Max staleness:** 60 seconds. If a node's `last_seen` exceeds this and the pod
  is not in the latest watch state, refuse to route.
- **Pod restart race:** Watch events propagate quickly. Accept small race window,
  rely on retry-once semantics.
- **Cache invalidation:** Watch events (ADDED, MODIFIED, DELETED) update cache
  immediately.

### Ready vs Serving

"Ready" means Kubernetes PodReady condition is true. This does not guarantee the
viewer HTTP server is responding. Mitigations:

- **Aggressive HTTP timeouts:** 5 seconds per request
- **Retry-once:** On timeout or 5xx, try the same node once more
- **No health probe:** Unnecessary complexity for research phase

### Heterogeneous Metrics

`list_metrics` returns metrics from one node. If clusters have nodes with
different metric schemas (e.g., different kernel versions), agents should:
1. Call `list_nodes` to enumerate nodes
2. Call `list_metrics` per-node if needed (optional `node` param supported)

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
4. If found but stale (>60s, not in watch) → return error "node 'X' is stale"
5. Construct URL: `http://{pod_ip}:8050/api/...`
6. Forward request with 5s timeout
7. On timeout/5xx → retry once
8. Return response or error

## Tool Schemas

### list_nodes (REQ-MCP-007)

**Input:** None

**Output:**
```json
{
  "nodes": [
    {
      "name": "worker-1",
      "ready": true,
      "last_seen": 1704067200000
    }
  ]
}
```

Note: Pod details (IP, name) are intentionally hidden. Agents route by node name.

### list_metrics (REQ-MCP-001)

**Input:**
- `node` (optional): Specific node to query. If omitted, picks first ready node.

**Output:**
```json
{
  "node": "worker-1",
  "metrics": [{ "name": "cgroup.v2.cpu.stat.usage_usec" }],
  "studies": [{ "id": "periodicity", "name": "Periodicity Study", "description": "..." }]
}
```

**Fallback behavior:**
1. If `node` specified → query that node
2. If `node` omitted → pick first ready node
3. If no nodes ready → return error "no nodes available"
4. If selected node errors → try next ready node once
5. If all fail → return last error

### list_containers (REQ-MCP-002, REQ-MCP-003, REQ-MCP-008)

**Input:**
- `node` (required): Node name to query
- `metric` (optional): Metric name to filter containers (only those with data for this metric)
- `namespace` (optional): Kubernetes namespace filter
- `qos_class` (optional): QoS class filter
- `search` (optional): Text search in pod/container names
- `limit` (optional): Max results (default 20)

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
  "total_matching": 47
}
```

### analyze_container (REQ-MCP-004, REQ-MCP-005, REQ-MCP-008)

**Input:**
- `node` (required): Node name to query
- `container` (required): Container ID (short 12-char or full 64-char)
- `study_id` (required): "periodicity" or "changepoint"
- `metric` (optional): Single metric name
- `metric_prefix` (optional): Metric prefix to analyze all matching metrics

**Output:**
```json
{
  "node": "worker-1",
  "container": { "id": "abc123def456", "pod_name": "nginx-xyz", "namespace": "default" },
  "study": "periodicity",
  "results": [{
    "metric": "cgroup.v2.cpu.stat.usage_usec",
    "stats": { "avg": 1234.5, "max": 5000, "min": 100, "stddev": 500, "trend": "stable", "sample_count": 1000 },
    "findings": [{ "type": "periodicity", "timestamp_ms": 1704067200000, "label": "period=60000ms", "details": {...} }]
  }]
}
```

**Container ID collision handling:**

If a short ID matches multiple containers on the node, return an error:
```json
{
  "error": "ambiguous_container_id",
  "message": "Container ID 'abc123' matches 3 containers. Use full ID or list_containers to disambiguate.",
  "matches": ["abc123def456...", "abc123789abc...", "abc123000111..."]
}
```

## HTTP/SSE Transport (REQ-MCP-006)

The MCP server uses `rmcp` with HTTP/SSE transport:

```rust
use rmcp::transport::sse::SseServer;

let server = McpMetricsViewer::new(pod_watcher);
let sse = SseServer::new("0.0.0.0:8080");
server.serve(sse).await?;
```

Clients connect via:
- **Claude Code:** `kubectl port-forward svc/mcp-metrics 8888:8080`, then configure MCP with `http://localhost:8888`
- **AI SRE Agents:** Direct HTTP to `http://mcp-metrics.{namespace}.svc.cluster.local:8080`

## Kubernetes Manifests

### Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mcp-metrics
  labels:
    app: mcp-metrics
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mcp-metrics
  template:
    metadata:
      labels:
        app: mcp-metrics
    spec:
      serviceAccountName: mcp-metrics
      containers:
      - name: mcp
        image: fine-grained-monitor:latest
        command: ["/usr/local/bin/mcp-metrics-viewer"]
        args:
          - "--port=8080"
          - "--daemonset-label=app=fine-grained-monitor"
          - "--daemonset-namespace=default"
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
  name: mcp-metrics
spec:
  selector:
    app: mcp-metrics
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
  name: mcp-metrics
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: mcp-metrics
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: mcp-metrics
subjects:
  - kind: ServiceAccount
    name: mcp-metrics
roleRef:
  kind: Role
  name: mcp-metrics
  apiGroup: rbac.authorization.k8s.io
```

## Configuration

The MCP server binary accepts:

- `--port` (default: 8080): HTTP/SSE listen port
- `--daemonset-namespace` (default: "default"): Namespace containing DaemonSet
- `--daemonset-label` (default: "app=fine-grained-monitor"): Label selector for pods
- `--viewer-port` (default: 8050): Port on viewer containers
- `--stale-threshold` (default: 60): Seconds before node considered stale
- `--http-timeout` (default: 5): Seconds for viewer HTTP requests

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
