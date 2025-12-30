# Pod Metadata Enrichment

## User Story

As an engineer debugging container behavior, I need to see pod names instead of
container IDs so that I can quickly identify which workloads are consuming
resources without manually looking up container IDs.

## Requirements

### REQ-PME-001: Display Pod Names in Viewer

WHEN viewing the metrics viewer container list
THE SYSTEM SHALL display pod names instead of container short IDs for containers
running in Kubernetes pods

WHEN a container's pod name cannot be determined
THE SYSTEM SHALL fall back to displaying the container short ID

**Rationale:** Engineers recognize pod names from their deployments; container IDs
are opaque 12-character hex strings that require manual lookup.

---

### REQ-PME-002: Enrich Containers with Kubernetes Metadata

WHEN discovering containers via cgroup scanning
THE SYSTEM SHALL query the Kubernetes API to obtain pod metadata for each container

WHEN the Kubernetes API is unavailable
THE SYSTEM SHALL continue operation without metadata enrichment and log an info message

**Rationale:** Graceful degradation ensures the monitor works in non-Kubernetes
environments or when API access is restricted.

---

### REQ-PME-003: Persist Metadata in Index

WHEN container metadata is obtained from Kubernetes API
THE SYSTEM SHALL persist pod_name, namespace, and labels in the index.json file

WHEN the viewer loads from index.json
THE SYSTEM SHALL display the persisted metadata without requiring API access

**Rationale:** The viewer sidecar should display pod names instantly without needing
its own Kubernetes API access.
