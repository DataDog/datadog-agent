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

The watcher maintains two distinct staleness concepts:

**Global watcher staleness:** Detects Kubernetes API connectivity issues.
- Track `last_k8s_sync_ms`: updated on every successful list or watch resync
- If `now - last_k8s_sync_ms > 120s`, the entire cache is considered stale
- `list_nodes` should indicate `watcher_stale: true` when this occurs

**Per-node staleness:** Detects individual pods disappearing.
- Track `last_observed_ms` per node: updated when pod is seen in list/resync
- If `now - last_observed_ms > 60s` and pod not in latest resync, node is stale
- Return error "node 'X' is stale (not observed in recent sync)"

**Why this matters:** Stable pods generate zero watch events. If we only updated
timestamps on MODIFIED events, every healthy pod would appear "stale" after 60s.
The resync-based approach ensures quiet clusters remain healthy.

**Implementation:** Use kube-rs watcher with periodic resync (default ~5 minutes
in kube-rs, configurable). On each resync, update `last_observed_ms` for all
pods seen. This handles both the "quiet cluster" case and detects pods that
disappeared without generating DELETE events (rare but possible).

**Cache invalidation:** Watch events (ADDED, MODIFIED, DELETED) update cache
immediately. Resync updates `last_observed_ms` for all current pods.

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

On failure, retry behavior depends on error type:

| Error Type | Action |
|------------|--------|
| 4xx from viewer | Don't retry (client error) |
| 5xx from viewer | Retry once after re-resolving node→pod |
| Connection timeout | Retry once after re-resolving node→pod |
| Connection refused | Retry once after re-resolving node→pod |

**Re-resolve on retry:** Before retrying, look up the node again in the pod cache.
The pod IP may have changed due to a restart detected by the watcher.

**Jitter:** Add 50-200ms random delay before retry to avoid thundering herds.

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
      "last_observed_ms": 1704067200000
    }
  ]
}
```

**Field definitions:**
- `watcher_stale`: true if `now - last_sync_ms > 120s` (Kubernetes API may be unreachable)
- `last_sync_ms`: timestamp of last successful Kubernetes list/watch resync
- `last_observed_ms`: per-node timestamp when pod was last seen in a resync

Note: Pod details (IP, name) are intentionally hidden. Agents route by node name.

### list_metrics (REQ-MCP-001)

**Input:**
- `node` (optional): Specific node to query. If omitted, picks first ready node.

**Output:**
```json
{
  "node": "worker-1",
  "node_selection": "explicit",
  "metrics": [{ "name": "cgroup.v2.cpu.stat.usage_usec" }],
  "studies": [{ "id": "periodicity", "name": "Periodicity Study", "description": "..." }]
}
```

**Field definitions:**
- `node_selection`: How the node was chosen:
  - `"explicit"`: caller specified `node` parameter
  - `"first_ready"`: node was auto-selected (caller should be aware this may differ across calls)

**Fallback behavior:**
1. If `node` specified → query that node
2. If `node` omitted → pick first ready node (alphabetically by name for determinism)
3. If no nodes ready → return error "no nodes available"
4. If selected node errors → try next ready node once
5. If all fail → return last error

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

Readiness should verify watcher is connected and cache is populated.

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
