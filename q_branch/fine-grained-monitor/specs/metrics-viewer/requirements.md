# Metrics Viewer

## User Story

As an engineer investigating container behavior in a Kubernetes cluster, I need
to explore metrics timeseries interactively so that I can identify patterns,
anomalies, and correlate pod names with resource usage without manually copying
files to my local machine.

## Core Viewer Requirements

### REQ-MV-001: View Metrics Timeseries

WHEN user opens the viewer with a metrics file
THE SYSTEM SHALL display an interactive timeseries chart

WHEN no containers are selected
THE SYSTEM SHALL display an empty chart with instructions

**Rationale:** Engineers need visual representation of metrics to understand
container behavior over time. Raw numbers are difficult to interpret; charts
reveal patterns instantly.

---

### REQ-MV-002: Select Metrics to Display

WHEN user opens the metric selector
THE SYSTEM SHALL show all available metric types from the data file

WHEN user selects a metric
THE SYSTEM SHALL update the chart to display that metric for selected containers

WHEN user switches metric
THE SYSTEM SHALL preserve container selection and time range

**Rationale:** Different investigations require different signals. CPU usage
helps diagnose throttling; memory helps find leaks; IO helps find bottlenecks.
Users need to switch freely without losing context.

---

### REQ-MV-003: Search and Select Containers

WHEN user types in the container search box
THE SYSTEM SHALL filter the container list to matching names

WHEN user selects containers from the list
THE SYSTEM SHALL add their timeseries to the chart

WHEN user deselects a container
THE SYSTEM SHALL remove its timeseries from the chart

WHEN user changes container selection
THE SYSTEM SHALL preserve the current time range

**Rationale:** Users often know which container they want to investigate.
Search accelerates finding specific containers. Preserving the time window lets
users compare different containers at the same moment.

**DEPRECATED:** The "Top N by average" capability was removed. Computing average
values required reading all parquet files (30+ seconds), making metric switching
unacceptably slow. See REQ-MV-019 for the replacement container ordering strategy.

---

### REQ-MV-004: Zoom and Pan Through Time

WHEN user drags on the chart to select a time region
THE SYSTEM SHALL zoom to show only that region

WHEN user scrolls on the chart
THE SYSTEM SHALL zoom in or out centered on cursor position

WHEN user drags while zoomed
THE SYSTEM SHALL pan the view in the drag direction

WHEN zoomed, WHEN user clicks reset
THE SYSTEM SHALL return to showing the full time range

**Rationale:** Metrics files can span hours or days. Users need to zoom into
specific incidents (seconds/minutes) while maintaining ability to see the full
picture.

---

### REQ-MV-005: Navigate with Range Overview

WHEN viewing a zoomed region
THE SYSTEM SHALL display a miniature overview of the full time range

WHEN user drags on the overview
THE SYSTEM SHALL pan the main chart to that time region

WHEN user resizes the selection on the overview
THE SYSTEM SHALL zoom the main chart to match

**Rationale:** When zoomed into detail, users lose context of where they are in
the overall timeline. The overview provides spatial context and fast navigation.

---

### REQ-MV-006: Detect Periodic Patterns

WHEN user initiates periodicity study on a specific container
THE SYSTEM SHALL analyze that container's timeseries for periodic patterns

WHEN user initiates periodicity study while multiple containers are selected
THE SYSTEM SHALL deselect other containers and focus on the target container

WHEN periodic patterns are detected
THE SYSTEM SHALL highlight time regions where periodicity was found

WHEN user views periodicity results
THE SYSTEM SHALL display the detected period, confidence score, and amplitude

WHEN no periodic patterns meet detection threshold
THE SYSTEM SHALL indicate that no periodic patterns were found

WHEN user initiates periodicity study
THE SYSTEM SHALL preserve the current time range

**Rationale:** Periodic patterns often indicate throttling, resource
contention, or scheduling issues. Manual detection requires tedious visual
scanning. Automated detection surfaces these patterns instantly, letting
engineers focus on root cause rather than pattern hunting. Single-container
focus eliminates visual noise and makes the study action intentional.

---

### REQ-MV-007: Visualize Periodicity Patterns

WHEN periodicity study is active
THE SYSTEM SHALL display a results panel showing the target container, window
count, dominant period, and average confidence

WHEN periodic regions are detected
THE SYSTEM SHALL overlay period markers on the chart within detected regions

WHEN user zooms or pans
THE SYSTEM SHALL update periodicity markers to remain aligned with the data

WHEN user hovers over a periodic region
THE SYSTEM SHALL display a tooltip with period duration, confidence, and amplitude

WHEN user exits periodicity study
THE SYSTEM SHALL remove all periodicity markers and the results panel

WHEN user exits periodicity study
THE SYSTEM SHALL offer to restore the previous container selection

WHEN user restores previous selection
THE SYSTEM SHALL restore container selection AND preserve the current time range

**Rationale:** Seeing periodicity markers overlaid on raw data confirms the
detection and shows exactly when periodic behavior occurs. Engineers can
correlate periodic timing with other events or container state changes.
The results panel provides a summary without requiring hover interactions.
Restoration of previous selection supports the explore-then-deep-dive workflow.

---

### REQ-MV-008: Automatic Y-Axis Scaling

WHEN the visible time range changes (via zoom, pan, or reset)
THE SYSTEM SHALL automatically adjust Y-axis bounds to fit the visible data

WHEN displayed data changes (via metric, filter, or container selection)
THE SYSTEM SHALL automatically adjust Y-axis bounds to fit the new data

**Rationale:** Y-axis should always maximize use of chart space for whatever
data is currently visible. This reveals subtle variations that would be
compressed if the scale remained fixed to the original full-range bounds.

---

### REQ-MV-009: Graceful Empty Data Display

WHEN selected containers have no data points for the chosen metric
THE SYSTEM SHALL display a clear message indicating no data is available

WHEN selected containers have all zero or constant values
THE SYSTEM SHALL display the chart with a reasonable Y-axis range

**Rationale:** Some metrics only record non-zero values during specific
conditions (e.g., hugetlb usage). Users need clear feedback when data is absent
rather than a broken chart display, helping them understand the metric is valid
but simply has no recorded activity.

---

## Cluster Deployment Requirements

### REQ-MV-010: Access Viewer From Cluster

WHEN user port-forwards to the fine-grained-monitor DaemonSet
THE SYSTEM SHALL serve the metrics viewer UI on port 8050

WHEN user connects from outside the cluster
THE SYSTEM SHALL display the same interactive chart interface as the local viewer

**Rationale:** Engineers currently must `kubectl cp` parquet files from pods and
run the viewer locally. Direct in-cluster access eliminates this friction,
enabling faster debugging workflows during incident investigation.

---

### REQ-MV-011: View Node-Local Metrics

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

### REQ-MV-012: Fast Startup via Index

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

### REQ-MV-013: Viewer Operates Independently

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

## Metadata Display Requirements

### REQ-MV-014: Display Pod Names in Viewer

WHEN viewing the metrics viewer container list
THE SYSTEM SHALL display pod names instead of container short IDs for containers
running in Kubernetes pods

WHEN a container's pod name cannot be determined
THE SYSTEM SHALL fall back to displaying the container short ID

**Rationale:** Engineers recognize pod names from their deployments; container IDs
are opaque 12-character hex strings that require manual lookup.

---

### REQ-MV-015: Enrich Containers with Kubernetes Metadata

WHEN discovering containers via cgroup scanning
THE SYSTEM SHALL query the Kubernetes API to obtain pod metadata for each container

WHEN the Kubernetes API is unavailable
THE SYSTEM SHALL continue operation without metadata enrichment and log an info message

**Rationale:** Graceful degradation ensures the monitor works in non-Kubernetes
environments or when API access is restricted.

---

### REQ-MV-016: Persist Metadata in Index

WHEN container metadata is obtained from Kubernetes API
THE SYSTEM SHALL persist pod_name, namespace, and labels in the index.json file

WHEN the viewer loads from index.json
THE SYSTEM SHALL display the persisted metadata without requiring API access

**Rationale:** The viewer sidecar should display pod names instantly without needing
its own Kubernetes API access.

---

### REQ-MV-017: Detect Changepoints in Metrics

WHEN user initiates changepoint study on a specific container
THE SYSTEM SHALL analyze that container's timeseries for abrupt changes in behavior

WHEN user initiates changepoint study while multiple containers are selected
THE SYSTEM SHALL deselect other containers and focus on the target container

WHEN changepoints are detected
THE SYSTEM SHALL report each changepoint location with a confidence indicator

WHEN no changepoints meet detection threshold
THE SYSTEM SHALL indicate that no significant changes were found

WHEN user initiates changepoint study
THE SYSTEM SHALL preserve the current time range

**Rationale:** Sudden changes in metrics often indicate deployments, configuration
changes, or the onset of problems. Engineers investigating incidents need to quickly
identify when behavior shifted rather than manually scanning timeseries for step
changes or trend breaks.

---

### REQ-MV-018: Visualize Changepoint Locations

WHEN changepoint study is active
THE SYSTEM SHALL display a results panel showing the target container and
detected changepoint count

WHEN changepoints are detected
THE SYSTEM SHALL draw vertical markers at each changepoint location on the chart

WHEN user hovers over a changepoint marker
THE SYSTEM SHALL display a tooltip with the timestamp and metric values before/after

WHEN user clicks a changepoint marker
THE SYSTEM SHALL zoom the chart to show context around that changepoint

WHEN user exits changepoint study
THE SYSTEM SHALL remove all changepoint markers and the results panel

WHEN user exits changepoint study
THE SYSTEM SHALL offer to restore the previous container selection

**Rationale:** Visual markers on the chart confirm detection accuracy and show exactly
when changes occurred. Engineers can correlate changepoint timing with deployments,
alerts, or external events. Click-to-zoom enables rapid drill-down into specific
transitions.

---

### REQ-MV-019: Container List Sorted by Recency

WHEN user views the container list
THE SYSTEM SHALL order containers by most recently observed first

WHEN user selects a metric
THE SYSTEM SHALL display the container list within 100ms

WHEN a container has not been observed for more than 1 hour
THE SYSTEM SHALL display the container with reduced visual prominence

**Rationale:** Engineers investigating current issues care most about actively
running containers. Containers that were recently observed are more likely to be
relevant to ongoing investigations. Instant container list loading enables rapid
metric switching during incident triage.

---
