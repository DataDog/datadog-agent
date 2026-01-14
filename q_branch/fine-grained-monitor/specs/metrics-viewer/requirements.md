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
THE SYSTEM SHALL load files within the active time range as a unified dataset

WHEN new parquet files are written by the collector
THE SYSTEM SHALL include them in subsequent queries that overlap their time range

**Rationale:** Each node collects independent metrics. Users investigating
node-specific issues (CPU throttling, memory pressure) need to see that node's
data without cross-node confusion. Time range scoping ensures queries remain
performant even with weeks of accumulated data.

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

## Multi-Panel Comparison Requirements

### REQ-MV-020: View Multiple Metrics Simultaneously

WHEN user adds multiple panels
THE SYSTEM SHALL display up to 5 chart panels stacked vertically

WHEN user has one panel
THE SYSTEM SHALL prevent removal of that panel

WHEN user has 5 panels
THE SYSTEM SHALL prevent adding additional panels

**Rationale:** Engineers investigating incidents need to correlate multiple signals.
Viewing CPU throttling alongside memory pressure and network IO reveals relationships
invisible when switching between single metrics. The 5-panel limit balances comparison
power with screen real estate.

---

### REQ-MV-021: Panel Cards in Sidebar

WHEN user views the panels section in sidebar
THE SYSTEM SHALL display one card per panel

WHEN displaying a panel card
THE SYSTEM SHALL show: Panel number, current metric name, and current study type

WHEN no study is active on a panel
THE SYSTEM SHALL show "Study: none" in that panel's card

WHEN a study is active on a panel
THE SYSTEM SHALL show the study type name (periodicity or changepoint) in that panel's card

**Rationale:** Engineers need to see which metrics and studies are plotted on each panel.
Panel cards provide a consolidated view of panel configuration, making it easy to understand
what analysis is active at a glance. Grouping metric and study together per panel clarifies
which analytical overlays belong to which chart.

---

### REQ-MV-022: Add Panels via Inline Autocomplete

WHEN user clicks "+ Add Panel" button in the sidebar
THE SYSTEM SHALL create a new panel card with an inline metric selector focused

WHEN user types in the inline metric selector
THE SYSTEM SHALL show autocomplete suggestions filtered by fuzzy matching

WHEN user selects a metric from autocomplete
THE SYSTEM SHALL create a new panel displaying that metric for all selected containers

WHEN 5 panels already exist
THE SYSTEM SHALL hide the "+ Add Panel" button

**Rationale:** Fuzzy search enables rapid panel creation without navigating dropdown
menus. Engineers can type "cpu" and quickly find cpu_user, cpu_system, cpu_throttled.
Inline autocomplete provides immediate feedback and reduces UI clutter compared to
a persistent search box. Adding panels with all selected containers maintains
consistency with the shared container selection model.

---

### REQ-MV-023: Edit Panel Metric Inline

WHEN user clicks on a panel's metric name in the sidebar panel card
THE SYSTEM SHALL show an inline autocomplete input for metric selection

WHEN user types in the inline metric input
THE SYSTEM SHALL filter available metrics using fuzzy matching

WHEN user selects a different metric from autocomplete
THE SYSTEM SHALL update that panel to display the new metric

WHEN user changes a panel's metric
THE SYSTEM SHALL preserve the time range and container selection

WHEN user changes a panel's metric
THE SYSTEM SHALL remove any active study on that panel

**Rationale:** Engineers often realize mid-investigation they want a different
metric. Inline editing is faster than removing and re-adding panels. Autocomplete
with fuzzy matching accelerates finding the right metric. Removing studies on
metric change prevents confusion from stale analysis results that no longer match
the displayed data.

---

### REQ-MV-024: Remove Panels via Sidebar

WHEN user removes a panel entry from the sidebar
THE SYSTEM SHALL remove that panel and its series from the display

WHEN user removes a panel
THE SYSTEM SHALL preserve the time range and container selection for remaining panels

WHEN only one panel remains
THE SYSTEM SHALL disable the remove action for that panel

**Rationale:** Engineers need to simplify views as investigations narrow focus.
Easy panel removal supports iterative refinement. The minimum of one panel ensures
the viewer always displays something useful.

---

### REQ-MV-025: Synchronized Time Axis Across Panels

WHEN user zooms on any panel
THE SYSTEM SHALL apply the same zoom level and time range to all panels

WHEN user pans on any panel
THE SYSTEM SHALL pan all panels by the same time offset

WHEN user clicks reset on any panel
THE SYSTEM SHALL reset all panels to show the full time range

**Rationale:** Correlating events across metrics requires seeing the exact same time
window. If CPU spikes at 14:32:15, engineers need to see what memory and IO were
doing at that precise moment. Synchronized navigation eliminates tedious manual
alignment between panels.

---

### REQ-MV-026: Shared Container Selection Across Panels

WHEN user selects containers from the container list
THE SYSTEM SHALL display those containers on all panels

WHEN user deselects a container
THE SYSTEM SHALL remove that container's series from all panels

WHEN user changes container selection
THE SYSTEM SHALL preserve each panel's metric and time range

**Rationale:** When comparing metrics, engineers typically investigate the same
containers across all views. Shared selection reduces clicks and ensures panels
stay synchronized. If investigating pod "frontend-abc123", all panels should show
that pod's CPU, memory, and IO together.

---

### REQ-MV-027: Panel-Specific Y-Axis Scaling

WHEN panels display different metrics
EACH PANEL SHALL scale its Y-axis independently to fit its visible data

WHEN the visible time range changes
EACH PANEL SHALL recalculate its own Y-axis bounds

WHEN a panel's displayed containers or metric changes
THAT PANEL SHALL rescale its Y-axis to fit the new data

**Rationale:** Different metrics have vastly different scales. CPU percentage
ranges 0-100% while memory might be 0-16GB. Each panel must scale independently
to make patterns visible; a shared Y-axis would compress one metric to invisibility.

---

### REQ-MV-028: Shared Range Overview in Multi-Panel Mode

WHEN multiple panels are displayed
THE SYSTEM SHALL show a single range overview below all panels

WHEN user interacts with the range overview
THE SYSTEM SHALL update all panels to the selected time region

**Rationale:** One range overview reinforces the mental model that all panels share
the same time axis. Multiple overviews would waste space and create confusion. The
overview shows "where you are" in the full timeline regardless of which panel the
user is examining.

---

### REQ-MV-029: Add Study to Panel

WHEN user clicks on "Study: none" in a panel card
THE SYSTEM SHALL show an inline autocomplete with available study types (periodicity, changepoint)

WHEN user types in the study autocomplete
THE SYSTEM SHALL filter study types by name

WHEN user selects a study type
THE SYSTEM SHALL apply that study to all selected containers on that panel

WHEN study analysis completes
THE SYSTEM SHALL display visual markers on the chart (vertical lines for changepoints, shaded regions for periodicity)

WHEN no containers are selected
THE SYSTEM SHALL disable study selection for that panel

**Rationale:** Studies reveal patterns across all monitored containers. Panel-level
studies apply analysis to the complete dataset, helping engineers identify system-wide
trends and correlations. Visual markers overlay directly on timeseries data, making
detected patterns immediately visible in context.

---

### REQ-MV-030: Study Visualization on Chart

WHEN a study is active on a panel
THE SYSTEM SHALL display the study type in that panel's sidebar card

WHEN a study is active on a panel
THE SYSTEM SHALL overlay visual markers on the chart for detected patterns

WHEN displaying a changepoint study
THE SYSTEM SHALL show vertical lines at each detected change location

WHEN displaying a periodicity study
THE SYSTEM SHALL show shaded regions for each detected periodic window

WHEN user hovers over a study marker on the chart
THE SYSTEM SHALL show a tooltip with time, confidence score, and magnitude

WHEN user clicks the X button next to a study name in the panel card
THE SYSTEM SHALL remove the study and its visual markers from the chart

WHEN user changes the panel's base metric
THE SYSTEM SHALL remove the active study from that panel

**Rationale:** Chart annotations keep the focus on visual data patterns. Engineers
can see detected periods and changepoints directly overlaid on timeseries data,
making correlations immediately visible. Inline tooltips provide analytical details
on demand without cluttering the sidebar. Removing studies when the metric changes
prevents confusion from stale analysis that no longer matches the displayed data.

---

### REQ-MV-031: Studies Do Not Consume Panel Slots

WHEN user adds a study to a panel
THE SYSTEM SHALL NOT count the study toward the 5-panel maximum

WHEN user has 5 panels with studies on each
THE SYSTEM SHALL allow all studies to remain active

**Rationale:** Studies are overlays that enhance existing panels, not separate views
competing for screen space. Engineers should be able to run periodicity analysis on
all 5 panels without hitting artificial limits.

---

## Dashboard Requirements

### REQ-MV-032: Filter Containers by Labels

WHEN user requests containers with label filters via API
THE SYSTEM SHALL return only containers whose labels match all specified key-value pairs

WHEN label filter specifies a key that doesn't exist on a container
THE SYSTEM SHALL exclude that container from results

WHEN label filter is combined with other filters (namespace, search)
THE SYSTEM SHALL apply all filters as intersection

**Rationale:** Engineers investigating specific incidents need to focus on related
containers. Filtering by Kubernetes labels (e.g., `fgm-scenario: abc123`) reduces
noise and enables targeted analysis of correlated containers.

---

### REQ-MV-033: Load Dashboard Configuration

WHEN user navigates to viewer with `?dashboard=<url>` parameter
THE SYSTEM SHALL fetch and parse the dashboard JSON from the URL

WHEN user navigates with `?dashboard_inline=<base64>` parameter
THE SYSTEM SHALL decode and parse the inline dashboard JSON

WHEN dashboard JSON is invalid or unreachable
THE SYSTEM SHALL display an error message and fall back to default empty view

WHEN dashboard contains template variables (e.g., `{{RUN_ID}}`)
THE SYSTEM SHALL substitute values from URL parameters (e.g., `?run_id=abc123`)

**Rationale:** Engineers need reproducible views of specific incidents. Dashboard
URLs enable sharing investigation context with teammates and preserving analysis
configurations for post-mortems.

---

### REQ-MV-034: Filter Containers via Dashboard

WHEN dashboard specifies `containers.label_selector`
THE SYSTEM SHALL filter the container list to those matching all specified labels

WHEN dashboard specifies `containers.namespace`
THE SYSTEM SHALL filter containers to those in the specified namespace

WHEN dashboard specifies `containers.name_pattern`
THE SYSTEM SHALL filter containers to those whose names match the glob pattern

WHEN multiple container filters are specified
THE SYSTEM SHALL apply all filters as intersection

**Rationale:** Dashboard authors need declarative control over which containers
appear. Label selectors integrate naturally with Kubernetes labeling conventions,
enabling scenarios like "show all containers from this scenario run."

---

### REQ-MV-035: Automatic Time Range from Containers

WHEN dashboard specifies `time_range.mode: "from_containers"`
THE SYSTEM SHALL compute the time range from earliest `first_seen` to latest `last_seen`
of filtered containers

WHEN dashboard specifies `time_range.padding_seconds`
THE SYSTEM SHALL expand the computed time range by the specified padding on both ends

WHEN dashboard computes a time range
THE SYSTEM SHALL request data from the backend for that time range

WHEN filtered containers have no valid time bounds
THE SYSTEM SHALL fall back to showing the last hour

**Rationale:** Incident investigations should automatically focus on when the
relevant containers were active. The computed time range must scope both the chart
view and the backend data loading to ensure data from the incident period is
actually retrieved.

---

### REQ-MV-036: Configure Panels from Dashboard

WHEN dashboard specifies a `panels` array
THE SYSTEM SHALL create chart panels with the specified metrics

WHEN a panel specifies a `title`
THE SYSTEM SHALL display that title instead of the metric name

WHEN a panel specifies a `study` configuration
THE SYSTEM SHALL automatically run the study on matching containers after data loads

WHEN dashboard specifies more than 5 panels
THE SYSTEM SHALL create only the first 5 panels

**Rationale:** Dashboard authors need to prescribe which metrics are relevant for
a given investigation. Pre-configured panels with studies accelerate root cause
analysis by surfacing analytical overlays automatically.

---

## Time Range Selection Requirements

### REQ-MV-037: Select Investigation Time Window

WHEN user views the metrics viewer without an active dashboard
THE SYSTEM SHALL display a time range selector with options: 1 hour, 1 day, 1 week, all time

WHEN user selects a time range
THE SYSTEM SHALL load and display only containers and data within that range

WHEN a dashboard is active
THE SYSTEM SHALL disable the manual time range selector and indicate dashboard control

**Rationale:** Ad-hoc investigations require flexible time windows. Engineers may
need to see patterns over days or weeks, not just the default hour. Dashboard mode
already controls time range via container lifetimes (REQ-MV-035), so manual selection
is disabled to avoid conflicts.

---

### REQ-MV-038: Default to Recent Activity

WHEN user opens the metrics viewer without specifying a time range
THE SYSTEM SHALL default to showing the last 1 hour of data

WHEN user opens the metrics viewer with an active dashboard
THE SYSTEM SHALL use the dashboard's time range instead of the default

**Rationale:** Most ad-hoc investigations focus on recent activity. A 1-hour default
balances recency with startup performance while keeping initial data loads fast.

---

### REQ-MV-039: Preserve Selection Across Range Changes

WHEN user changes the time range
THE SYSTEM SHALL preserve selected containers that have data in the new range

WHEN a selected container has no data in the new range
THE SYSTEM SHALL automatically deselect that container

WHEN all selected containers are deselected due to range change
THE SYSTEM SHALL clear the chart and show guidance to select containers

**Rationale:** Users shouldn't lose their work when exploring different time windows.
Auto-deselecting unavailable containers prevents confusion from empty charts while
preserving valid selections.

---

### REQ-MV-040: Efficient Time Range Queries

WHEN user selects a time range
THE SYSTEM SHALL display the container list within 2 seconds

WHEN user queries overlapping time ranges
THE SYSTEM SHALL reuse cached file metadata to respond faster

WHEN a parquet file has been scanned for container metadata
THE SYSTEM SHALL cache that result permanently for all future queries

**Rationale:** Investigation workflows require rapid iteration. File-level caching
ensures each parquet file is scanned at most once, enabling quick switching between
time ranges without redundant work. This scales efficiently regardless of how many
weeks of data are stored.

---
