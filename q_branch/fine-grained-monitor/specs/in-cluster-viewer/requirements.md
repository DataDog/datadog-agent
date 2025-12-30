# In-Cluster Viewer

## User Story

As an engineer debugging container behavior in a Kubernetes cluster, I need to
access the metrics viewer directly from within the cluster so that I can
visualize collected data without manually copying files to my local machine.

## Requirements

### REQ-ICV-001: Access Viewer From Cluster

WHEN user port-forwards to the fine-grained-monitor DaemonSet
THE SYSTEM SHALL serve the metrics viewer UI on port 8050

WHEN user connects from outside the cluster
THE SYSTEM SHALL display the same interactive chart interface as the local viewer

**Rationale:** Engineers currently must `kubectl cp` parquet files from pods and
run the viewer locally. Direct in-cluster access eliminates this friction,
enabling faster debugging workflows during incident investigation.

---

### REQ-ICV-002: View Node-Local Metrics

WHEN user accesses the in-cluster viewer
THE SYSTEM SHALL display metrics collected on that specific node

WHEN multiple parquet files exist from rotation
THE SYSTEM SHALL load all available files as a unified dataset

WHEN new parquet files are written by the collector
THE SYSTEM SHALL include them in subsequent data loads

**Rationale:** Each node collects independent metrics. Users investigating
node-specific issues (CPU throttling, memory pressure) need to see that node's
data without cross-node confusion.

---

### REQ-ICV-003: Fast Startup via Index

WHEN the viewer starts with thousands of parquet files present
THE SYSTEM SHALL begin serving the UI within 5 seconds

WHEN the viewer starts before any data exists
THE SYSTEM SHALL poll for data with a 3-minute timeout before displaying an error

WHEN new containers appear during collection
THE SYSTEM SHALL make them available in the viewer without restart

WHEN containers disappear
THE SYSTEM SHALL retain their metadata for historical queries

**Rationale:** Engineers need immediate access to the viewer during incidents.
Scanning thousands of accumulated parquet files at startup creates unacceptable
delays (30+ minutes observed with 11k files). A lightweight index enables
instant startup while preserving full historical queryability.

---

### REQ-ICV-004: Viewer Operates Independently

WHEN the metrics collector container restarts
THE SYSTEM SHALL continue serving the viewer UI

WHEN the viewer container restarts
THE SYSTEM SHALL NOT affect metrics collection

WHEN either container experiences issues
THE SYSTEM SHALL allow the other to continue operating normally

**Rationale:** Separation of concerns ensures debugging the viewer doesn't
disrupt data collection, and collector issues don't prevent viewing historical
data.

---
