# Metrics Viewer - Technical Design

## Architecture Overview

Library + binary architecture enabling reuse, with in-cluster deployment as a
sidecar alongside the monitor.

```
src/metrics_viewer/
├── mod.rs           # Module exports
├── data.rs          # Parquet loading, metric discovery
├── lazy_data.rs     # Index-based queries, time-range file discovery
├── server.rs        # HTTP server, API handlers
├── studies/
│   ├── mod.rs       # Study trait, registry
│   └── periodicity.rs  # Periodicity detection study
└── static/          # Embedded frontend assets

src/bin/fgm-viewer.rs  # CLI binary wrapper
src/index.rs           # Index data structures and I/O
src/kubernetes.rs      # Kubernetes API client for metadata enrichment
```

### Deployment Architecture

The viewer runs as a sidecar container alongside the fine-grained-monitor
collector in the DaemonSet. Both containers share a volume for parquet file
access.

```
┌─────────────────────────────────────────────────────────────┐
│               DaemonSet Pod (per node)                       │
│                                                              │
│  ┌─────────────────┐           ┌─────────────────┐          │
│  │    monitor      │           │     viewer      │          │
│  │   container     │           │   container     │          │
│  │                 │           │   port 8050     │          │
│  └────────┬────────┘           └────────┬────────┘          │
│           │ write                       │ read              │
│           └─────────────┬───────────────┘                   │
│                         ▼                                   │
│                ┌───────────────┐                            │
│                │  /data volume │                            │
│                │  index.json   │                            │
│                │  *.parquet    │                            │
│                └───────────────┘                            │
└─────────────────────────────────────────────────────────────┘
```

---

## Core Viewer Implementation

### REQ-MV-001: View Metrics Timeseries

#### Frontend: uPlot

uPlot selected for canvas-based rendering with smooth pan/zoom on large datasets.
Lightweight (~40KB), plugin hooks for custom overlays, direct scale/series access.

#### Static Asset Embedding

Frontend assets embedded in binary via `include_str!` or `rust-embed` crate.
Single binary distribution, no external file dependencies.

#### Initial Display

On startup, load parquet file and serve HTTP on configurable port. Browser opens
automatically unless `--no-browser` flag. Empty chart shown until containers
selected.

### REQ-MV-002: Select Metrics to Display

#### Metric Discovery

On parquet load, scan `metric_name` column to build unique metric list.
Cache metric names in `AppState` for fast `/api/metrics` responses.

#### API: GET /api/metrics

Returns list of available metric names with sample counts:

```json
{
  "metrics": [
    {"name": "cgroup.v2.cpu.stat.usage_usec", "sample_count": 50000},
    {"name": "cgroup.v2.memory.current", "sample_count": 48000}
  ]
}
```

#### Context Preservation

Frontend maintains current container selection and zoom range when metric
changes. Only timeseries data is re-fetched.

### REQ-MV-003: Search and Select Containers

#### Frontend Implementation

Searchable multi-select list. Client-side filtering of container list by
name substring. "Top N" buttons trigger selection of first N containers
sorted by average value descending.

#### API: GET /api/containers

Query params: `metric` (required).

Returns containers with summary stats:

```json
{
  "containers": [
    {"id": "abc123...", "short_id": "abc123", "qos_class": "Guaranteed",
     "namespace": "default", "pod_name": "my-pod", "avg": 45.2, "max": 98.1, "samples": 1000}
  ]
}
```

### REQ-MV-004: Zoom and Pan Through Time

#### uPlot Configuration

Enable built-in zoom (drag select) and pan (shift+drag or touch). Scroll
wheel zoom centered on cursor. Configure via uPlot options:

```javascript
scales: { x: { time: true } },
cursor: { drag: { x: true, y: false } }
```

#### Reset Button

Frontend reset button calls `uplot.setScale('x', {min, max})` with original
data bounds.

### REQ-MV-005: Navigate with Range Overview

#### Implementation Options

1. **Second uPlot instance** - Synchronized mini chart showing full range
2. **Custom canvas overlay** - Draw simplified range indicator below main chart

Start with option 1 (simpler). Use uPlot's `setSeries` hooks to sync selection
rectangle with main chart zoom state.

### REQ-MV-006: Detect Periodic Patterns

#### Study Abstraction

Periodicity detection is implemented as a `Study` trait, establishing a pattern
for future analysis features:

```rust
pub trait Study: Send + Sync {
    fn name(&self) -> &str;
    fn analyze(&self, timeseries: &[TimeseriesPoint]) -> StudyResult;
}

pub struct StudyResult {
    pub study_name: String,
    pub windows: Vec<StudyWindow>,
    pub summary: String,
}

pub struct StudyWindow {
    pub start_time_ms: i64,
    pub end_time_ms: i64,
    pub metrics: HashMap<String, f64>,  // Study-specific metrics
    pub label: String,                   // Display label
}
```

`StudyRegistry` holds available studies. This pattern allows adding new
analysis types without API changes.

#### Per-Container Study Initiation

Studies are initiated per-container via a study icon in the container list,
rather than a global button. This makes the action intentional and ensures
single-container focus.

Container list item with study action:
```
┌─────────────────────────────────────────┐
│ ☑ my-pod           avg: 64.1  [📊]      │
│                             ^           │
│                     study icon button   │
└─────────────────────────────────────────┘
```

#### Selection State Management

When user clicks study icon on container X:

1. Save current container selection to `previousSelection`
2. Deselect all containers
3. Select container X
4. Set `studyActive = true` and `studyContainer = X`
5. Fetch periodicity data for container X
6. Preserve current time range (do not reset zoom/pan)

Frontend state:
```javascript
let previousSelection = [];     // Container IDs before study
let studyActive = false;
let studyContainer = null;      // Container ID being studied
```

#### Periodicity Detection Algorithm

Sliding window autocorrelation:

- Window size: 60 samples
- Step size: 30 samples (50% overlap)
- Period range: 2-30 seconds
- Detection threshold: periodicity score >= 0.6, amplitude >= 10%

Metrics returned per window: `period`, `periodicity_score`, `amplitude`.

#### API: GET /api/studies

Returns available studies:

```json
{
  "studies": [
    {"id": "periodicity", "name": "Periodicity Study",
     "description": "Detects periodic patterns using autocorrelation"}
  ]
}
```

#### API: GET /api/study/periodicity

Query params: `metric`, `container` (single container ID).

Returns periodicity detection results:

```json
{
  "study": "periodicity",
  "container": "abc123",
  "windows": [
    {"start_time_ms": 1000, "end_time_ms": 5000,
     "metrics": {"period": 10.0, "score": 0.85, "amplitude": 25.3},
     "label": "10s period (85% confidence)"}
  ],
  "summary": {
    "window_count": 3,
    "dominant_period": 10.0,
    "avg_confidence": 0.82
  }
}
```

### REQ-MV-007: Visualize Periodicity Patterns

#### Results Panel

When periodicity study is active, the Studies section in the sidebar transforms
into a results panel:

```
┌─────────────────────────────┐
│ Periodicity Study     [✕]  │
│ Container: my-pod          │
├─────────────────────────────┤
│ 52 windows detected        │
│ Dominant period: 20s       │
│ Avg confidence: 81%        │
├─────────────────────────────┤
│ [Restore previous view]    │
└─────────────────────────────┘
```

Panel elements:
- Header with close button [✕] to exit study mode
- Target container identifier
- Summary statistics computed from StudyResult
- Restoration button (visible when previousSelection is non-empty)

#### Exit Study Flow

When user clicks [✕] or "Restore previous view":

1. Set `studyActive = false`
2. Clear periodicity overlay data
3. If restoring: select containers from `previousSelection`
4. If not restoring: keep current single-container selection
5. Preserve current time range (critical: do not reset zoom/pan)
6. Clear `previousSelection`

#### Frontend Visualization Plugin

uPlot hooks API for custom drawing. Periodicity visualization plugin receives
`StudyResult` and draws overlays during `drawAxes` or `drawSeries` hooks.

#### Periodicity Markers

Vertical dashed lines at period intervals within each detected window.
Lines colored to match associated container trace. Semi-transparent to
avoid obscuring data. Detected regions highlighted with subtle background
shading.

#### Tooltip Interaction

Hover detection on periodicity markers and regions. Tooltip displays:
- Period duration (e.g., "10.2s period")
- Confidence score (e.g., "85% confidence")
- Amplitude (e.g., "±25.3 amplitude")

Tooltip positioned near cursor, auto-repositions to stay within chart bounds.

### REQ-MV-017: Detect Changepoints in Metrics

#### Changepoint Detection Algorithm

Uses Bayesian Online Changepoint Detection (BOCPD) via the `augurs-changepoint`
crate. BOCPD maintains a probability distribution over run lengths (time since
last changepoint) and updates it with each observation.

Implementation uses `NormalGammaDetector` with tunable hazard rate:

- **Hazard lambda:** 250.0 (expected mean run length between changepoints)
- **Prior:** NormalGamma(0.0, 1.0, 1.0, 1.0) - uninformative prior

The detector processes the full timeseries and returns indices where changepoints
were detected. These indices map back to timestamps via the timeseries data.

#### Study Implementation

New file `src/metrics_viewer/studies/changepoint.rs` implements the `Study` trait:

- `id()`: "changepoint"
- `name()`: "Changepoint Study"
- `description()`: "Detects abrupt changes using Bayesian Online Changepoint Detection"
- `analyze()`: Runs BOCPD, returns `StudyResult` with one `StudyWindow` per changepoint

Each `StudyWindow` represents a single changepoint with metrics:
- `timestamp_ms`: Time of the changepoint
- `value_before`: Average value in window before changepoint
- `value_after`: Average value in window after changepoint
- `magnitude`: Absolute difference between before/after averages
- `direction`: "increase" or "decrease"

#### API: GET /api/study/changepoint

Query params: `metric`, `container` (single container ID).

Returns changepoint detection results:

```json
{
  "study": "changepoint",
  "container": "abc123",
  "windows": [
    {"start_time_ms": 5000, "end_time_ms": 5000,
     "metrics": {"value_before": 45.2, "value_after": 78.5, "magnitude": 33.3},
     "label": "+33.3 at 10:05:00"}
  ],
  "summary": {
    "changepoint_count": 3,
    "largest_magnitude": 33.3
  }
}
```

### REQ-MV-018: Visualize Changepoint Locations

#### Results Panel

When changepoint study is active, displays in the Studies sidebar section:

```
┌─────────────────────────────┐
│ Changepoint Study     [✕]   │
│ Container: my-pod           │
├─────────────────────────────┤
│ 3 changepoints detected     │
│ Largest change: +33.3       │
├─────────────────────────────┤
│ [Restore previous view]     │
└─────────────────────────────┘
```

#### Changepoint Markers

Vertical solid lines at each detected changepoint. Color matches container trace.
Line style distinguishes from periodicity markers (solid vs dashed).

Marker rendering in uPlot draw hook:
- Full-height vertical line at changepoint timestamp
- Small arrow indicator showing direction (up for increase, down for decrease)

#### Tooltip Content

On hover over changepoint marker:
- Timestamp of change
- Value before (5-sample average)
- Value after (5-sample average)
- Magnitude and direction
- "Click to zoom" hint

#### Click-to-Zoom Behavior

Clicking a changepoint marker zooms to show ±30 seconds around the changepoint,
providing context for the transition.

---

### REQ-MV-008: Automatic Y-Axis Scaling

#### Implementation

uPlot configured with custom range function:
```javascript
y: { range: (u, min, max) => [0, max * 1.1] }
```

This automatically scales Y-axis to fit visible data (0 to max + 10% padding)
whenever the visible time range or displayed data changes. No manual button
required.

#### Reset Behavior

When user clicks reset (full time range), Y-axis auto-scales to fit all data.

### REQ-MV-009: Graceful Empty Data Display

#### Empty Data Detection

After building series arrays in `renderChart()`, check for valid data:

1. Count non-null values across all series
2. If all values are null → show "No data available" message
3. If all values are zero/constant → ensure minimum Y-axis range

#### Y-Axis Minimum Range

When `yMax` is 0 or very small, use a fallback range to prevent chart collapse:

```javascript
// Ensure minimum Y-axis range for constant/zero data
if (yMax === 0) {
    yMax = 1;  // Fallback for all-zero data
}
originalYRange = [0, yMax * 1.1];
```

#### Empty State Message

New variant of `showEmptyState()` for metrics with timestamps but no values:

```javascript
function showNoDataState(metricName) {
    // Shows: "No data recorded for [metric]"
    // Explains: metric exists but has no non-null values
}
```

---

## Cluster Deployment Implementation

### REQ-MV-010: Cluster Access Implementation

#### Container Configuration

The viewer runs as a second container in the DaemonSet pod:

- **Image:** Same `fine-grained-monitor` image (contains both binaries)
- **Command:** `/usr/local/bin/fgm-viewer`
- **Arguments:** `/data --port=8050 --no-browser`
- **Port:** 8050 (TCP)

#### Access Method

Users access via kubectl port-forward:

```bash
kubectl port-forward ds/fine-grained-monitor 8050:8050
```

This connects to an arbitrary pod in the DaemonSet. For specific node access:

```bash
kubectl port-forward pod/fine-grained-monitor-<hash> 8050:8050
```

#### Image Changes

The Dockerfile copies both binaries to the final image:

```dockerfile
COPY --from=builder /build/target/release/fine-grained-monitor /usr/local/bin/
COPY --from=builder /build/target/release/fgm-viewer /usr/local/bin/
```

### REQ-MV-011: Node-Local Data Access

#### Shared Volume

Both containers mount the same volume:

| Container | Mount | Access |
|-----------|-------|--------|
| monitor | /data | read-write |
| viewer | /data | read-only |

#### Volume Type

Use `hostPath` volume pointing to `/var/lib/fine-grained-monitor`:

- Persists across pod restarts for post-mortem analysis
- Node-local storage (no cross-node access)
- Already configured in existing DaemonSet

#### Time Range Scoped Loading

Files are loaded based on the active time range (see REQ-MV-037). The viewer only
reads parquet files whose timestamps fall within the selected range:

- Default: last 1 hour
- User-selected: 1h, 1d, 1w, or all
- Dashboard: computed from container lifetimes

This scoping ensures queries remain performant even with weeks of accumulated data.

#### Directory Input Support

The `fgm-viewer` binary accepts a directory path. File discovery uses time-range
based path computation rather than expensive globbing:

```rust
// Time-range based discovery (efficient)
fn discover_files_by_time_range(data_dir: &Path, range: TimeRange) -> Vec<PathBuf> {
    // Compute date directories to scan based on range
    // List files within those directories
    // Filter by filename timestamps
}
```

### REQ-MV-012: Fast Startup via Index

#### Problem

Scanning all parquet files at startup to build metadata (metrics list, container
info) is O(n) with file count. With 90-second rotation and days of accumulated
data, file count reaches thousands, causing 30+ minute startup times.

#### Solution: Separate Index File

The collector maintains a lightweight `index.json` that the viewer loads
instantly. Data files are loaded on-demand based on query time range.

```
/data/
  index.json                              # Metadata (~10-50 KB)
  dt=2025-12-30/
    identifier=pod-xyz/
      metrics-20251230T160000Z.parquet    # Pure timeseries data
      metrics-20251230T160130Z.parquet
```

#### Index File Schema

```json
{
  "schema_version": 2,
  "updated_at": "2025-12-30T16:05:00Z",

  "containers": {
    "abc123def456": {
      "full_id": "abc123def456789abcdef...",
      "pod_name": "coredns-5dd5756b68-xyz",
      "namespace": "kube-system",
      "qos_class": "Burstable",
      "first_seen": "2025-12-28T10:00:00Z",
      "last_seen": "2025-12-30T16:05:00Z",
      "labels": {"app": "coredns"}
    }
  },

  "data_range": {
    "earliest": "2025-12-22T00:00:00Z",
    "latest": "2025-12-30T16:05:00Z",
    "rotation_interval_sec": 90
  }
}
```

**Note:** Metric names are derived from parquet file schema, not stored in index.
This avoids hard-coding and ensures the metric list expands naturally as new
metrics are added to the collector.

#### Collector Index Management

```
On startup:
  - Load existing index.json or create empty
  - Track known_containers: HashSet<ContainerId>

On each collection cycle (every 1s):
  - current_containers = containers observed this cycle

  If current_containers != known_containers:
    - New containers: Add to index with first_seen = now
    - Gone containers: Update last_seen timestamp
    - Write index atomically (write .tmp, rename)
    - known_containers = current_containers

On rotation (every 90s):
  - Write parquet file with predictable name: metrics-{ISO8601}Z.parquet
  - Update index.data_range.latest
  - Update last_seen for all active containers
```

Container churn is infrequent (minutes/hours), so index writes are rare.

#### Viewer Startup Sequence

```
1. Attempt to load /data/index.json
2. If index exists:
   - Load container metadata from index
   - Read metric names from schema of most recent parquet file
   - Start server immediately
3. If index missing:
   - Poll for index.json every 5 seconds
   - Timeout after 3 minutes with error message
   - If parquet files exist but no index, rebuild index from files (fallback)
4. Serve UI on port 8050
```

#### Time-Range Based File Discovery

Instead of globbing all files, the viewer computes file paths from time range:

```rust
fn find_files_for_range(data_dir: &Path, start: DateTime, end: DateTime) -> Vec<PathBuf> {
    // Predictable naming: /data/dt={date}/identifier={id}/metrics-{timestamp}Z.parquet
    // Compute expected file timestamps based on rotation interval (90s)
    // Return paths that fall within [start, end]
}
```

This avoids expensive glob operations over thousands of files.

#### Atomic Index Writes

```rust
fn write_index(path: &Path, index: &Index) -> Result<()> {
    let tmp_path = path.with_extension("json.tmp");
    let json = serde_json::to_string_pretty(index)?;
    std::fs::write(&tmp_path, json)?;
    std::fs::rename(&tmp_path, path)?;  // Atomic on POSIX
    Ok(())
}
```

#### Edge Case: No Data Files

When viewer starts with no index and no parquet files:

1. Display "Waiting for metrics data..." page
2. Poll every 5 seconds for either index.json or parquet files
3. After 3 minutes, display timeout error with troubleshooting guidance
4. If parquet files appear before index, rebuild index from files

#### Currently-Writing Files

The collector's in-progress parquet file (not yet rotated) is excluded from
queries. Users see data with 0-90 second lag, which is acceptable for debugging
use cases.

### REQ-MV-013: Container Independence

#### Sidecar Pattern

Using Kubernetes sidecar pattern ensures:

- Containers share pod lifecycle but run independently
- Shared volumes enable data exchange
- Resource limits apply per-container
- Restart policies apply per-container

#### Resource Allocation

| Container | Memory Request | Memory Limit | CPU Request | CPU Limit |
|-----------|---------------|--------------|-------------|-----------|
| monitor | 64Mi | 256Mi | 100m | 500m |
| viewer | 32Mi | 128Mi | 10m | 100m |

#### Failure Isolation

- Monitor crash: Viewer continues serving existing data
- Viewer crash: Monitor continues collecting (Kubernetes restarts viewer)
- Both share termination grace period for clean shutdown

---

## Metadata Enrichment Implementation

### REQ-MV-014: Display Pod Names

The viewer displays pod names from the index instead of container short IDs.
The container list and tooltips show human-readable pod names with fallback
to short IDs when metadata is unavailable.

### REQ-MV-015: Kubernetes API Client

New module `src/kubernetes.rs`:

- Uses `kube` crate with in-cluster config
- Queries pods filtered by node name (from `NODE_NAME` env var)
- Extracts container ID to pod metadata mapping
- Refresh interval: 30 seconds

#### Container ID Matching

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

#### Metadata Extraction

For each pod, extract:
- `pod_name`: `pod.metadata.name`
- `namespace`: `pod.metadata.namespace`
- `labels`: `pod.metadata.labels` (optional HashMap)

Map each container status to these values using the container ID.

#### Graceful Degradation

If Kubernetes API unavailable:
1. Log info message at startup: "Kubernetes API not available, running without pod metadata enrichment"
2. Continue with cgroup-only discovery
3. Containers display as short IDs (existing behavior)
4. No error state - this is expected for non-k8s environments

#### RBAC Requirements

Minimal permissions needed (pods list only):

```yaml
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["list"]
```

### REQ-MV-016: Persist Metadata in Index

`ContainerEntry` includes metadata fields (schema version 2):

```rust
pub struct ContainerEntry {
    pub full_id: String,
    pub pod_uid: Option<String>,
    pub qos_class: String,
    pub first_seen: DateTime<Utc>,
    pub last_seen: DateTime<Utc>,
    // Metadata enrichment fields
    pub pod_name: Option<String>,
    pub namespace: Option<String>,
    pub labels: Option<HashMap<String, String>>,
}
```

Schema version bumped to 2 for forward compatibility. Fields are optional
to support graceful degradation when API unavailable.

---

## Data Flow Summary

```
Startup:
  load_index() -> ContainerMetadata from index.json
  read_schema() -> metric names from parquet file schema
  Start server immediately (5 second startup target)

User flow:
  1. /api/metrics -> populate metric dropdown
  2. Select metric -> /api/containers?metric=X -> populate container list (with pod names)
  3. Select containers -> /api/timeseries?metric=X&containers=a,b,c -> render chart

Study flow:
  1. Click study icon on container Y
  2. Save current selection to previousSelection
  3. Deselect all, select Y -> /api/timeseries?metric=X&containers=Y -> render chart
  4. /api/study/periodicity?metric=X&container=Y -> display results panel + overlay
  5. Exit study -> restore previousSelection OR keep Y -> preserve time range
```

---

## Multi-Panel Comparison Implementation

### REQ-MV-020: View Multiple Metrics Simultaneously

#### Panel Architecture

The viewer supports up to 5 chart panels stacked vertically. Each panel is an
independent uPlot instance sharing a synchronized time axis.

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Global Series Sidebar                        │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │ 🔍 Search metrics...                                         │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                                                                      │
│  Panel 1: cpu_usage                                    [Edit][✕]    │
│    • pod-frontend / cpu_usage                                        │
│    • pod-backend / cpu_usage                                         │
│    [+ Add Study]                                                     │
│                                                                      │
│  Panel 2: memory_current                               [Edit][✕]    │
│    • pod-frontend / memory_current                                   │
│    • pod-backend / memory_current                                    │
│    [+ Add Study]                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

#### Frontend State Model

```javascript
// Panel state
let panels = [
  {
    id: 1,
    metric: 'cpu_usage',
    uplot: null,           // uPlot instance
    studies: []            // Active studies on this panel
  }
];
let maxPanels = 5;

// Shared state (applies to all panels)
let selectedContainers = [];  // Container IDs
let timeRange = { min: null, max: null };  // Synchronized across panels
```

#### Panel Layout Rendering

```javascript
function renderPanels() {
  const chartArea = document.getElementById('chart-area');
  chartArea.innerHTML = '';

  panels.forEach((panel, idx) => {
    const panelDiv = document.createElement('div');
    panelDiv.className = 'chart-panel';
    panelDiv.id = `panel-${panel.id}`;
    panelDiv.style.height = `${100 / panels.length}%`;
    chartArea.appendChild(panelDiv);

    // Initialize uPlot for this panel
    panel.uplot = createUPlot(panelDiv, panel.metric);
  });
}
```

### REQ-MV-021: Panel Cards in Sidebar

#### Panel Card Component

Each panel displays as a card in the sidebar showing metric and study configuration.
The card provides a compact view of panel state without listing individual container series.

#### Card Structure

```html
<div class="panel-card" data-panel-id="${panel.id}">
  <div class="panel-card-header">
    <span class="panel-number">${panelIndex + 1}</span>
    <button class="panel-remove" data-remove-panel="${panel.id}">×</button>
  </div>
  <div class="panel-card-body">
    <div class="panel-metric-row">
      <label>Metric:</label>
      <span class="panel-metric-value" data-edit-metric="${panel.id}">
        ${panel.metric}
      </span>
    </div>
    <div class="panel-study-row">
      <label>Study:</label>
      <span class="panel-study-value" data-edit-study="${panel.id}">
        ${panel.study || 'none'}
      </span>
      ${panel.study ? '<button class="study-clear">×</button>' : ''}
    </div>
  </div>
</div>
```

#### Component Rendering

Panel cards render via `Components.PanelCards(state)` which maps over `state.panels`.
Each card shows current configuration without enumerating individual timeseries.

#### Interaction Handlers

- Click metric value → show inline autocomplete (REQ-MV-023)
- Click study value → show inline autocomplete (REQ-MV-029)
- Click X next to study → remove study
- Click × in header → remove panel (REQ-MV-024)

### REQ-MV-022: Add Panels via Inline Autocomplete

#### Add Panel Button

The "+ Add Panel" button appears at the bottom of the panel cards list when fewer than 5 panels exist.

```html
<div id="panel-cards">
  <!-- Panel cards rendered here -->
</div>
<button id="add-panel-btn" style="display: ${canAddPanel ? 'block' : 'none'}">
  + Add Panel
</button>
```

#### Inline Autocomplete for New Panel

Clicking "+ Add Panel" creates a temporary panel card with an autocomplete input:

```javascript
function handleAddPanel() {
  if (state.panels.length >= state.maxPanels) return;

  // Create temporary panel card with inline autocomplete
  const tempCard = createTempPanelCard();
  showInlineAutocomplete(tempCard, {
    items: state.metrics.map(m => m.name),
    fuzzyMatch: true,
    onSelect: (metric) => {
      dispatch({ type: Actions.ADD_PANEL, metric });
    },
    onCancel: () => {
      removeTempPanelCard();
    }
  });
}
```

#### Fuzzy Matching

Uses lightweight fuzzy matching for metric filtering (sequential character matching):

```javascript
function fuzzyMatch(query, text) {
  query = query.toLowerCase();
  text = text.toLowerCase();
  let qi = 0;
  for (let ti = 0; ti < text.length && qi < query.length; ti++) {
    if (text[ti] === query[qi]) qi++;
  }
  return qi === query.length;
}
```

Autocomplete shows top 10 matches, updated on each keystroke with 150ms debounce.

### REQ-MV-023: Edit Panel Metric Inline

#### Inline Autocomplete Activation

Clicking the metric name in a panel card replaces the text with an autocomplete input:

```javascript
function handleMetricClick(panelId) {
  const panel = state.panels.find(p => p.id === panelId);
  if (!panel) return;

  const metricSpan = document.querySelector(`[data-edit-metric="${panelId}"]`);
  showInlineAutocomplete(metricSpan, {
    items: state.metrics.map(m => m.name),
    currentValue: panel.metric,
    fuzzyMatch: true,
    placeholder: 'Type to search metrics...',
    onSelect: (newMetric) => {
      if (newMetric !== panel.metric) {
        dispatch({ type: Actions.SET_PANEL_METRIC, panelId, metric: newMetric });
      }
    },
    onCancel: () => {
      // Revert to showing metric name
      renderPanelCards();
    }
  });
}
```

#### Inline Autocomplete Component

Shared autocomplete component used for both metric and study selection. Renders
dropdown below the clicked element with keyboard navigation (arrow keys, enter, escape).

Autocomplete closes on:
- Selection (Enter key or click)
- Cancel (Escape key or click outside)
- Blur event

### REQ-MV-024: Remove Panels via Sidebar

```javascript
function removePanel(panelId) {
  if (panels.length <= 1) return;  // Prevent removing last panel

  panels = panels.filter(p => p.id !== panelId);
  renderPanels();
  renderSeriesList();
  updateSearchInputState();
}
```

### REQ-MV-025: Synchronized Time Axis Across Panels

#### Time Sync Hook

When any panel's time range changes, propagate to all panels:

```javascript
function createUPlot(container, metric) {
  const opts = {
    // ... other options ...
    hooks: {
      setScale: [
        (u, key) => {
          if (key === 'x') {
            syncTimeRange(u.scales.x.min, u.scales.x.max);
          }
        }
      ]
    }
  };
  return new uPlot(opts, data, container);
}

function syncTimeRange(min, max) {
  if (timeRange.min === min && timeRange.max === max) return;

  timeRange = { min, max };

  panels.forEach(panel => {
    if (panel.uplot) {
      panel.uplot.setScale('x', { min, max });
    }
  });

  // Update shared range overview
  updateRangeOverview(min, max);
}
```

#### Reset All Panels

```javascript
function resetAllPanels() {
  const fullRange = getFullDataRange();
  syncTimeRange(fullRange.min, fullRange.max);
}
```

### REQ-MV-026: Shared Container Selection Across Panels

Container selection is global. When selection changes, all panels update:

```javascript
function setSelectedContainers(containerIds) {
  selectedContainers = containerIds;

  // Reload data for all panels
  panels.forEach(panel => {
    loadPanelData(panel);
  });

  renderSeriesList();
}

async function loadPanelData(panel) {
  if (selectedContainers.length === 0) {
    panel.uplot.setData([[], ...selectedContainers.map(() => [])]);
    return;
  }

  const response = await fetch(
    `/api/timeseries?metric=${panel.metric}&containers=${selectedContainers.join(',')}`
  );
  const data = await response.json();
  panel.uplot.setData(formatForUPlot(data));
}
```

### REQ-MV-027: Panel-Specific Y-Axis Scaling

Each panel configures its own Y-axis range based on visible data:

```javascript
function createUPlot(container, metric) {
  const opts = {
    scales: {
      x: { time: true },
      y: {
        range: (u, dataMin, dataMax) => {
          // Panel-specific auto-scaling
          const padding = (dataMax - dataMin) * 0.1 || 1;
          return [Math.max(0, dataMin - padding), dataMax + padding];
        }
      }
    },
    // ... other options
  };
  return new uPlot(opts, data, container);
}
```

### REQ-MV-028: Shared Range Overview in Multi-Panel Mode

A single range overview sits below all panels and controls the time axis:

```
┌────────────────────────────────────────┐
│           Panel 1: cpu_usage           │
├────────────────────────────────────────┤
│           Panel 2: memory              │
├────────────────────────────────────────┤
│       [=======] Range Overview         │  <- Single overview
└────────────────────────────────────────┘
```

#### Overview Synchronization

```javascript
let overviewUPlot = null;

function createRangeOverview() {
  const container = document.getElementById('range-overview');
  overviewUPlot = new uPlot({
    width: container.clientWidth,
    height: 50,
    cursor: { drag: { x: true, y: false } },
    hooks: {
      setScale: [
        (u, key) => {
          if (key === 'x') {
            syncTimeRange(u.scales.x.min, u.scales.x.max);
          }
        }
      ]
    }
  }, overviewData, container);
}

function updateRangeOverview(min, max) {
  if (overviewUPlot) {
    // Draw selection box on overview
    overviewUPlot.setSelect({ left: min, width: max - min });
  }
}
```

### REQ-MV-029: Add Study to Panel

#### Inline Study Selection

Clicking "Study: none" in a panel card shows an inline autocomplete with study types:

```javascript
function handleStudyClick(panelId) {
  const panel = state.panels.find(p => p.id === panelId);
  if (!panel || state.selectedContainerIds.length === 0) return;

  const studySpan = document.querySelector(`[data-edit-study="${panelId}"]`);
  showInlineAutocomplete(studySpan, {
    items: ['periodicity', 'changepoint'],
    currentValue: panel.study,
    fuzzyMatch: false,  // Only 2 options, no fuzzy match needed
    placeholder: 'Select study type...',
    onSelect: (studyType) => {
      dispatch({ type: Actions.ADD_STUDY, panelId, studyType });
    },
    onCancel: () => {
      renderPanelCards();
    }
  });
}
```

#### Study Execution for All Containers

When a study is added to a panel, the system runs analysis on ALL selected containers
and aggregates results:

```javascript
// In effects.js
registerHandler(Effects.FETCH_STUDY, async (effect, context) => {
  const { panelId, metric, studyType, containerIds } = effect;

  // Fetch study results for all containers
  const results = await Promise.all(
    containerIds.map(cid =>
      Api.fetchStudy(studyType, metric, cid)
    )
  );

  // Store aggregated results
  const key = DataStore.studyKey(metric, studyType);
  DataStore.setStudyResult(key, { results, containerIds });

  dispatch({
    type: Actions.SET_STUDY_LOADING,
    panelId,
    loading: false
  });

  context.renderPanel(panelId);
});
```

### REQ-MV-030: Study Visualization on Chart

#### Panel Card Display

When a study is active, the panel card shows the study type:

```javascript
function renderPanelCard(panel) {
  return `
    <div class="panel-study-row">
      <label>Study:</label>
      <span class="panel-study-value" data-edit-study="${panel.id}">
        ${panel.study || 'none'}
      </span>
      ${panel.study ? '<button class="study-clear" data-clear-study="${panel.id}">×</button>' : ''}
    </div>
  `;
}
```

#### Chart Annotation Rendering

Visual markers overlay on the uPlot chart using plugins:

```javascript
function renderStudyMarkers(panel) {
  if (!panel.study) return [];

  const studyKey = DataStore.studyKey(panel.metric, panel.study);
  const studyData = DataStore.getStudyResult(studyKey);
  if (!studyData) return [];

  if (panel.study === 'changepoint') {
    // Vertical lines at each changepoint
    return studyData.results.flatMap(r =>
      r.changepoints.map(cp => ({
        type: 'vertical-line',
        time: cp.time_ms,
        color: '#ff6b6b',
        tooltip: `Changepoint: ${cp.magnitude.toFixed(2)} (${(cp.confidence * 100).toFixed(1)}%)`
      }))
    );
  } else if (panel.study === 'periodicity') {
    // Shaded regions for periodic windows
    return studyData.results.flatMap(r =>
      r.windows.map(w => ({
        type: 'shaded-region',
        startTime: w.start_ms,
        endTime: w.end_ms,
        color: 'rgba(100, 149, 237, 0.2)',
        tooltip: `Period: ${w.period_seconds}s (${(w.confidence * 100).toFixed(1)}%)`
      }))
    );
  }
}
```

#### Tooltip Interaction

Click or hover on a marker shows tooltip with study details. Click on tooltip
can trigger zoom to that time range.

### REQ-MV-031: Studies Do Not Consume Panel Slots

Studies are overlays on existing panels, tracked in `panel.study` field. The
5-panel limit applies only to chart panels, not study overlays:

```javascript
function canAddPanel(state) {
  return state.panels.length < state.maxPanels;  // Studies don't count
}
```

Each panel can have one study active. The study overlays on the panel's chart
without consuming an additional panel slot.

---

## Multi-Panel API Changes

No backend API changes required. The existing endpoints support multi-panel:

- `GET /api/metrics` - Returns all available metrics (used by metric search)
- `GET /api/containers?metric=X` - Returns containers (shared selection)
- `GET /api/timeseries?metric=X&containers=a,b` - Called once per panel
- `GET /api/study/{type}?metric=X&container=Y` - Called for each study

The frontend manages panel state and makes parallel API calls for each panel's
data.

---

## Dashboard System Implementation

Dashboards are declarative JSON files that configure the viewer's initial state:
container filters, time range, and panels. They enable reproducible incident
investigations and shareable analysis configurations.

### REQ-MV-032: Filter Containers by Labels

#### API Change

Add `labels` query parameter to `/api/containers`:

```
GET /api/containers?metric=X&labels=key1:value1,key2:value2
```

#### Implementation

Extend `ContainersQuery` struct in `server.rs`:

```rust
#[derive(Deserialize)]
struct ContainersQuery {
    metric: String,
    #[serde(default)]
    qos_class: Option<String>,
    #[serde(default)]
    namespace: Option<String>,
    #[serde(default)]
    search: Option<String>,
    #[serde(default)]
    labels: Option<String>,  // NEW: comma-separated key:value pairs
}
```

Parse and filter in handler:

```rust
// Parse labels parameter
if let Some(ref labels_str) = query.labels {
    let label_filters: Vec<(&str, &str)> = labels_str
        .split(',')
        .filter_map(|kv| kv.split_once(':'))
        .collect();

    // Filter containers matching ALL labels
    containers.retain(|c| {
        if let Some(ref container_labels) = c.labels {
            label_filters.iter().all(|(k, v)| {
                container_labels.get(*k).map(|lv| lv == *v).unwrap_or(false)
            })
        } else {
            false
        }
    });
}
```

### REQ-MV-033: Load Dashboard Configuration

#### Dashboard JSON Schema v1

```json
{
  "schema_version": 1,
  "name": "Dashboard Title",
  "description": "Optional description",

  "containers": {
    "namespace": "default",
    "label_selector": { "key": "value" },
    "name_pattern": "pod-*/container-*"
  },

  "time_range": {
    "mode": "from_containers",
    "padding_seconds": 30
  },

  "panels": [
    { "metric": "cpu_percentage", "title": "CPU Usage" },
    { "metric": "cgroup.v2.memory.current", "title": "Memory" }
  ]
}
```

#### URL Parameters

- `?dashboard=<url>` - Fetch dashboard from URL (relative or absolute)
- `?dashboard_inline=<base64>` - Decode inline dashboard JSON
- `?run_id=<value>` - Template variable substitution for `{{RUN_ID}}`

#### Frontend Loading Flow

In `app.js` initialization:

```javascript
async function init() {
    const params = new URLSearchParams(window.location.search);

    // Check for dashboard parameter
    const dashboardUrl = params.get('dashboard');
    const dashboardInline = params.get('dashboard_inline');

    if (dashboardUrl || dashboardInline) {
        try {
            const dashboard = dashboardUrl
                ? await Effects.loadDashboard(dashboardUrl, params)
                : JSON.parse(atob(dashboardInline));

            await applyDashboard(dashboard, params);
        } catch (e) {
            console.error('Failed to load dashboard:', e);
            showError(`Dashboard load failed: ${e.message}`);
            // Fall back to default empty view
        }
        return;
    }

    // Default initialization...
}
```

#### Template Variable Substitution

```javascript
function substituteTemplateVars(dashboard, params) {
    const json = JSON.stringify(dashboard);
    const substituted = json.replace(/\{\{(\w+)\}\}/g, (match, varName) => {
        const value = params.get(varName.toLowerCase());
        return value || match;  // Keep original if no substitution
    });
    return JSON.parse(substituted);
}
```

### REQ-MV-034: Filter Containers via Dashboard

#### Apply Container Filters

```javascript
async function applyDashboard(dashboard, params) {
    // Substitute template variables
    dashboard = substituteTemplateVars(dashboard, params);

    // Build API query from dashboard filters
    const filterParams = new URLSearchParams();
    filterParams.set('metric', dashboard.panels[0]?.metric || 'cpu_percentage');

    if (dashboard.containers?.namespace) {
        filterParams.set('namespace', dashboard.containers.namespace);
    }
    if (dashboard.containers?.label_selector) {
        const labels = Object.entries(dashboard.containers.label_selector)
            .map(([k, v]) => `${k}:${v}`)
            .join(',');
        filterParams.set('labels', labels);
    }

    // Fetch filtered containers
    const containers = await Effects.fetchContainers(filterParams);

    // Apply name_pattern filter client-side (glob matching)
    const filtered = dashboard.containers?.name_pattern
        ? containers.filter(c => globMatch(c.pod_name, dashboard.containers.name_pattern))
        : containers;

    // Select all matching containers
    StateMachine.dispatch({
        type: 'SET_SELECTED_CONTAINERS',
        containers: filtered.map(c => c.short_id)
    });
}
```

### REQ-MV-035: Automatic Time Range from Containers

#### Time Range Computation

```javascript
function computeTimeRange(containers, config) {
    if (config.mode !== 'from_containers') {
        return null;  // Use default behavior
    }

    // Find earliest first_seen and latest last_seen
    let earliest = Infinity;
    let latest = -Infinity;

    containers.forEach(c => {
        if (c.first_seen) earliest = Math.min(earliest, c.first_seen);
        if (c.last_seen) latest = Math.max(latest, c.last_seen);
    });

    if (earliest === Infinity || latest === -Infinity) {
        // No valid bounds, fall back to last hour
        const now = Date.now();
        return { min: now - 3600000, max: now };
    }

    // Apply padding
    const padding = (config.padding_seconds || 0) * 1000;
    return {
        min: earliest - padding,
        max: latest + padding
    };
}
```

#### Backend Time Range Integration

The computed time range must be passed to the backend API for data loading.
Without this, the backend defaults to 1-hour lookback and dashboards for older
events would show empty charts.

```javascript
// In Api.fetchTimeseries - pass computed time range
async fetchTimeseries(metricName, containerIds, timeRange = null) {
    const params = new URLSearchParams({
        metric: metricName,
        containers: containerIds.join(',')
    });

    // Pass time range to backend (required for dashboards with historical data)
    if (timeRange) {
        params.set('start', timeRange.min);
        params.set('end', timeRange.max);
    }

    const res = await fetch(`/api/timeseries?${params.toString()}`);
    return res.json();
}
```

The backend uses this range to scope file discovery (see REQ-MV-040).

### REQ-MV-036: Configure Panels from Dashboard

#### Panel Creation

```javascript
async function createPanelsFromDashboard(dashboard) {
    // Limit to 5 panels maximum
    const panelConfigs = (dashboard.panels || []).slice(0, 5);

    // Clear existing panels
    StateMachine.dispatch({ type: 'CLEAR_PANELS' });

    // Create each panel
    for (const config of panelConfigs) {
        StateMachine.dispatch({
            type: 'ADD_PANEL',
            metric: config.metric,
            title: config.title || config.metric
        });

        // Run study if configured
        if (config.study) {
            // Studies require single container - apply pattern filter
            const targetContainer = findContainerByPattern(config.study.container_pattern);
            if (targetContainer) {
                await runStudy(config.study.type, config.metric, targetContainer);
            }
        }
    }
}
```

### Dashboard Loading Effect

In `effects.js`:

```javascript
async function loadDashboard(url, params) {
    // Handle relative URLs
    const fullUrl = url.startsWith('http') ? url : `${window.location.origin}${url}`;

    const response = await fetch(fullUrl);
    if (!response.ok) {
        throw new Error(`Failed to fetch dashboard: ${response.status}`);
    }

    const dashboard = await response.json();

    // Validate schema version
    if (dashboard.schema_version !== 1) {
        throw new Error(`Unsupported dashboard schema version: ${dashboard.schema_version}`);
    }

    return substituteTemplateVars(dashboard, params);
}
```

### Example Dashboard: sigpipe-crash

Located at `dashboards/sigpipe-crash.json`:

```json
{
  "schema_version": 1,
  "name": "SIGPIPE Crash Pattern",
  "description": "Container crashes from SIGPIPE (exit 141) when UDS server restarts",

  "containers": {
    "namespace": "default",
    "label_selector": { "fgm-scenario": "{{RUN_ID}}" }
  },

  "time_range": {
    "mode": "from_containers",
    "padding_seconds": 30
  },

  "panels": [
    { "metric": "cpu_percentage", "title": "CPU Usage %" },
    { "metric": "cgroup.v2.memory.current", "title": "Memory" },
    { "metric": "cgroup.v2.pids.current", "title": "PIDs" }
  ]
}
```

Usage: `http://localhost:8050/?dashboard=/dashboards/sigpipe-crash.json&run_id=abc123`

### Serving Dashboard Files

Dashboard JSON files in `dashboards/` are served as static files. Add route in
`server.rs`:

```rust
.route("/dashboards/:name", get(dashboard_file_handler))

async fn dashboard_file_handler(Path(name): Path<String>) -> Response {
    let static_dir = get_static_dir();
    let path = static_dir.parent().unwrap().join("dashboards").join(&name);

    match std::fs::read_to_string(&path) {
        Ok(content) => (
            [(header::CONTENT_TYPE, "application/json")],
            content,
        ).into_response(),
        Err(_) => (StatusCode::NOT_FOUND, "Dashboard not found").into_response()
    }
}
```

---

## Time Range Selection Implementation

### REQ-MV-037: Select Investigation Time Window

#### TimeRange Enum

```rust
/// Time range for queries.
/// Uses short format for API: 1h, 1d, 1w, all
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Default)]
pub enum TimeRange {
    #[default]
    Hour1,
    Day1,
    Week1,
    All,
}

impl TimeRange {
    pub fn to_duration(&self) -> Option<chrono::Duration> {
        match self {
            TimeRange::Hour1 => Some(chrono::Duration::hours(1)),
            TimeRange::Day1 => Some(chrono::Duration::days(1)),
            TimeRange::Week1 => Some(chrono::Duration::weeks(1)),
            TimeRange::All => None,
        }
    }

    pub fn as_str(&self) -> &'static str {
        match self {
            TimeRange::Hour1 => "1h",
            TimeRange::Day1 => "1d",
            TimeRange::Week1 => "1w",
            TimeRange::All => "all",
        }
    }
}
```

#### Frontend: Time Range Selector

Located in sidebar above the Containers section:

```html
<div class="time-range-section">
    <h3>Time Range</h3>
    <select id="time-range-select" :disabled="dashboardActive">
        <option value="1h" selected>1 hour</option>
        <option value="1d">1 day</option>
        <option value="1w">1 week</option>
        <option value="all">All time</option>
    </select>
    <span v-if="dashboardActive" class="dashboard-indicator">
        Dashboard controlled
    </span>
</div>
```

When dashboard is active, the selector is disabled and shows "Dashboard controlled"
to indicate time range is determined by REQ-MV-035.

#### API Changes

All data endpoints accept optional `range` parameter:

```rust
#[derive(Deserialize)]
struct ContainersQuery {
    metric: String,
    #[serde(default)]
    range: TimeRange,  // Defaults to Hour1
    // ... other filters
}

#[derive(Deserialize)]
struct TimeseriesQuery {
    metric: String,
    containers: String,
    #[serde(default)]
    range: TimeRange,
    // Alternative: explicit start/end for dashboard computed ranges
    #[serde(default)]
    start: Option<i64>,
    #[serde(default)]
    end: Option<i64>,
}
```

### REQ-MV-038: Default to Recent Activity

#### Frontend State

```javascript
const AppState = {
    timeRange: '1h',  // Default
    dashboardActive: false,
    // ...
};
```

On initialization, if no dashboard is active, `timeRange` defaults to `'1h'`.

### REQ-MV-039: Preserve Selection Across Range Changes

#### Auto-Deselect Logic

```javascript
Actions.onTimeRangeChange = async function() {
    // Re-fetch containers for new range
    await Actions.loadContainers();

    // Auto-deselect containers that no longer have data in this range
    const availableIds = new Set(AppState.containers.map(c => c.short_id));
    const validSelection = new Set(
        [...AppState.selectedContainerIds].filter(id => availableIds.has(id))
    );

    if (validSelection.size !== AppState.selectedContainerIds.size) {
        updateState({ selectedContainerIds: validSelection });
    }

    // Re-fetch timeseries if containers are still selected
    if (validSelection.size > 0) {
        await Actions.loadTimeseries();
    } else {
        // Clear chart if no containers selected
        updateState({ timeseries: [] });
    }
};
```

### REQ-MV-040: Efficient Time Range Queries

#### FileIndex Architecture

Replace `MetadataIndex` with file-level caching structure:

```rust
/// File-level index for efficient time range queries.
/// Files are the memoization unit - scanned once, cached forever.
pub struct FileIndex {
    /// File → time bounds (parsed from filename, O(1) per file)
    file_times: HashMap<PathBuf, (DateTime<Utc>, DateTime<Utc>)>,

    /// File → containers in that file
    /// None = not yet scanned, Some = scanned and cached forever
    file_containers: HashMap<PathBuf, Option<HashSet<String>>>,

    /// Global container info (merged from all scanned files)
    containers: HashMap<String, ContainerInfo>,

    /// Available metrics (from schema, stable across files)
    metrics: Vec<MetricInfo>,

    /// Unique QoS classes found
    qos_classes: HashSet<String>,

    /// Unique namespaces found
    namespaces: HashSet<String>,
}
```

#### Query Flow

```
GET /api/containers?range=1d

1. Filter files by time
   ─────────────────────
   files_in_range = file_times
       .filter(|(_, (start, end))| end > now - 1d)
       .keys()

2. Ensure files are scanned (lazy, cached)
   ────────────────────────────────────────
   for file in files_in_range:
       if file_containers[file].is_none():
           scan_file(file)  // populates file_containers AND containers

3. Aggregate containers from selected files
   ────────────────────────────────────────
   container_ids = files_in_range
       .flat_map(|f| file_containers[f])
       .collect::<HashSet>()

4. Return container info for those IDs
   ────────────────────────────────────
   containers.filter(|id| container_ids.contains(id))
```

#### Caching Property

Each parquet file is scanned at most once. Results are reused for any time range
query that includes that file:

| Action | Work Done | Cached For |
|--------|-----------|------------|
| First "1 week" query | Scan ~168 files | Forever |
| First "1 day" query | 0 files (already scanned) | N/A |
| First "1 hour" query | 0 files (already scanned) | N/A |
| Query after new file arrives | Scan 1 file | Forever |

Total work = O(number of unique files), not O(queries × files).

#### LazyDataStore Updates

```rust
pub struct LazyDataStore {
    data_dir: PathBuf,

    /// File-level index (mutable for lazy scanning)
    file_index: RwLock<FileIndex>,

    /// Cached timeseries: metric → container → points
    timeseries_cache: RwLock<HashMap<String, HashMap<String, Vec<TimeseriesPoint>>>>,

    /// Last file discovery time
    last_discovery: RwLock<Instant>,
}

impl LazyDataStore {
    pub fn get_containers_for_range(&self, range: TimeRange) -> Vec<ContainerInfo> {
        let mut index = self.file_index.write().unwrap();
        index.get_containers_for_range(range)
    }

    pub fn get_timeseries(
        &self,
        metric: &str,
        container_ids: &[&str],
        range: TimeRange,
    ) -> Result<Vec<(String, Vec<TimeseriesPoint>)>> {
        // Get files for this range
        let files = {
            let index = self.file_index.read().unwrap();
            index.files_in_range(range)
        };

        // Load data from those files only
        load_metric_data(&files, metric, container_ids)
    }
}
```

