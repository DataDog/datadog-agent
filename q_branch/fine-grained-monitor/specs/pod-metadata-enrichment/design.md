# Pod Metadata Enrichment - Design

## Architecture Overview

The monitor enriches container metadata by querying the Kubernetes API server.
This provides pod names, namespaces, and labels that are persisted in the index
file for the viewer to display.

## Data Flow

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  Cgroup Scan    │────▶│  K8s API Query   │────▶│  Index Update   │
│  (discovery.rs) │     │  (kubernetes.rs) │     │  (index.rs)     │
└─────────────────┘     └──────────────────┘     └─────────────────┘
                                                         │
                                                         ▼
                                                ┌─────────────────┐
                                                │  Viewer Display │
                                                │  (lazy_data.rs) │
                                                └─────────────────┘
```

1. Monitor discovers containers via cgroup scanning (existing)
2. Monitor queries Kubernetes API: `GET /api/v1/pods?fieldSelector=spec.nodeName={node}`
3. Monitor matches container IDs to pod metadata
4. Monitor writes enriched data to index.json
5. Viewer reads index.json and displays pod names

## REQ-PME-002 Implementation: Kubernetes API Client

New module `src/kubernetes.rs`:

- Uses `kube` crate with in-cluster config
- Queries pods filtered by node name (from `NODE_NAME` env var)
- Extracts container ID to pod metadata mapping
- Refresh interval: 30 seconds

### Container ID Matching

Kubernetes API returns container IDs with runtime prefix:
- `containerd://abc123def456...`
- `docker://abc123def456...`
- `cri-o://abc123def456...`

Strip prefix to match cgroup-discovered IDs:

```rust
fn strip_runtime_prefix(id: &str) -> &str {
    id.find("://").map(|i| &id[i+3..]).unwrap_or(id)
}
```

### Metadata Extraction

For each pod, extract:
- `pod_name`: `pod.metadata.name`
- `namespace`: `pod.metadata.namespace`
- `labels`: `pod.metadata.labels` (optional HashMap)

Map each container status to these values using the container ID.

## REQ-PME-003 Implementation: Index Schema Extension

`ContainerEntry` gains new optional fields (schema version 2):

```rust
pub struct ContainerEntry {
    pub full_id: String,
    pub pod_uid: Option<String>,
    pub qos_class: String,
    pub first_seen: DateTime<Utc>,
    pub last_seen: DateTime<Utc>,
    // NEW - REQ-PME-003
    pub pod_name: Option<String>,
    pub namespace: Option<String>,
    pub labels: Option<HashMap<String, String>>,
}
```

Schema version bumped to 2 for forward compatibility. Fields are optional
to support graceful degradation when API unavailable.

## Graceful Degradation

If Kubernetes API unavailable:
1. Log info message at startup: "Kubernetes API not available, running without pod metadata enrichment"
2. Continue with cgroup-only discovery
3. Containers display as short IDs (existing behavior)
4. No error state - this is expected for non-k8s environments

## RBAC Requirements

Minimal permissions needed (pods list only):

```yaml
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["list"]
```

This is significantly simpler than kubelet RBAC which requires `nodes/proxy`,
`nodes/stats`, etc.
