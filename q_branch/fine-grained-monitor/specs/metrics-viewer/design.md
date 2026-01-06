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

#### Directory Input Support

The `fgm-viewer` binary accepts a directory path and globs for `*.parquet`:

```rust
// In fgm-viewer.rs
if path.is_dir() {
    let pattern = format!("{}/**/*.parquet", path.display());
    let files: Vec<PathBuf> = glob(&pattern)?.filter_map(Result::ok).collect();
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

### REQ-MV-021: Global Series Sidebar

#### Sidebar Structure

The sidebar displays all series grouped by panel. Each panel section shows:
- Panel header with metric name, edit button, and remove button
- Series entries for each container on that panel
- Add Study button

```html
<div id="series-sidebar">
  <div class="metric-search">
    <input type="text" id="metric-search-input" placeholder="Search metrics...">
    <div id="metric-search-results"></div>
  </div>

  <div id="panel-groups">
    <!-- Dynamically populated -->
  </div>
</div>
```

#### Series Entry Rendering

```javascript
function renderSeriesList() {
  const container = document.getElementById('panel-groups');
  container.innerHTML = '';

  panels.forEach(panel => {
    const group = document.createElement('div');
    group.className = 'panel-group';
    group.innerHTML = `
      <div class="panel-header">
        <span class="panel-metric">${panel.metric}</span>
        <button class="edit-btn" onclick="editPanel(${panel.id})">Edit</button>
        <button class="remove-btn" onclick="removePanel(${panel.id})"
                ${panels.length === 1 ? 'disabled' : ''}>✕</button>
      </div>
      <div class="series-list">
        ${selectedContainers.map(c => `
          <div class="series-entry">
            <span class="container-name">${getContainerName(c)}</span>
            <span class="metric-name">/ ${panel.metric}</span>
            ${renderStudyBadges(panel, c)}
          </div>
        `).join('')}
      </div>
      <button class="add-study-btn" onclick="addStudy(${panel.id})">
        + Add Study
      </button>
    `;
    container.appendChild(group);
  });
}
```

### REQ-MV-022: Add Panels via Metric Search

#### Fuzzy Search Implementation

Uses a lightweight fuzzy matching algorithm for metric filtering:

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

function filterMetrics(query) {
  return availableMetrics.filter(m => fuzzyMatch(query, m.name));
}
```

#### Search Input Behavior

```javascript
const searchInput = document.getElementById('metric-search-input');
searchInput.addEventListener('input', debounce(async (e) => {
  const query = e.target.value;
  if (query.length < 1) {
    hideSearchResults();
    return;
  }

  const matches = filterMetrics(query);
  showSearchResults(matches);
}, 150));

function showSearchResults(metrics) {
  const results = document.getElementById('metric-search-results');
  results.innerHTML = metrics.slice(0, 10).map(m => `
    <div class="search-result" onclick="addPanelWithMetric('${m.name}')">
      ${m.name}
    </div>
  `).join('');
  results.style.display = 'block';
}
```

#### Add Panel Action

```javascript
function addPanelWithMetric(metric) {
  if (panels.length >= maxPanels) return;

  const newPanel = {
    id: Date.now(),
    metric: metric,
    uplot: null,
    studies: []
  };
  panels.push(newPanel);

  renderPanels();
  renderSeriesList();
  loadPanelData(newPanel);
  hideSearchResults();
  updateSearchInputState();
}

function updateSearchInputState() {
  const input = document.getElementById('metric-search-input');
  input.disabled = panels.length >= maxPanels;
  input.placeholder = panels.length >= maxPanels
    ? 'Maximum panels reached'
    : 'Search metrics...';
}
```

### REQ-MV-023: Edit Panel Metric via Sidebar

#### Edit Modal

Clicking "Edit" opens a modal to change the panel's metric:

```javascript
function editPanel(panelId) {
  const panel = panels.find(p => p.id === panelId);
  if (!panel) return;

  showModal({
    title: 'Edit Panel Metric',
    content: `
      <select id="edit-metric-select">
        ${availableMetrics.map(m => `
          <option value="${m.name}" ${m.name === panel.metric ? 'selected' : ''}>
            ${m.name}
          </option>
        `).join('')}
      </select>
    `,
    onConfirm: () => {
      const newMetric = document.getElementById('edit-metric-select').value;
      if (newMetric !== panel.metric) {
        panel.metric = newMetric;
        panel.studies = [];  // Clear studies on metric change (REQ-MV-023)
        loadPanelData(panel);
        renderSeriesList();
      }
    }
  });
}
```

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

#### Study Selection Flow

```javascript
function addStudy(panelId) {
  const panel = panels.find(p => p.id === panelId);
  if (!panel) return;

  showModal({
    title: 'Add Study',
    content: `
      <div class="study-selector">
        <label>Study Type:</label>
        <select id="study-type-select">
          <option value="periodicity">Periodicity</option>
          <option value="changepoint">Changepoint</option>
        </select>
      </div>
      <div class="container-selector">
        <label>Target Container:</label>
        <select id="study-container-select">
          ${selectedContainers.map(c => `
            <option value="${c}">${getContainerName(c)}</option>
          `).join('')}
        </select>
      </div>
    `,
    onConfirm: async () => {
      const studyType = document.getElementById('study-type-select').value;
      const container = document.getElementById('study-container-select').value;

      await runStudyOnPanel(panel, studyType, container);
    }
  });
}

async function runStudyOnPanel(panel, studyType, containerId) {
  const response = await fetch(
    `/api/study/${studyType}?metric=${panel.metric}&container=${containerId}`
  );
  const result = await response.json();

  panel.studies.push({
    type: studyType,
    container: containerId,
    result: result
  });

  renderStudyOverlay(panel);
  renderSeriesList();
}
```

### REQ-MV-030: Study Series in Sidebar

Studies appear as entries in the sidebar under their panel:

```javascript
function renderStudyBadges(panel, containerId) {
  const containerStudies = panel.studies.filter(s => s.container === containerId);
  if (containerStudies.length === 0) return '';

  return containerStudies.map(study => `
    <span class="study-badge ${study.type}">
      ${study.type}
      <button class="remove-study" onclick="removeStudy(${panel.id}, '${study.type}', '${containerId}')">
        ✕
      </button>
    </span>
  `).join('');
}

function removeStudy(panelId, studyType, containerId) {
  const panel = panels.find(p => p.id === panelId);
  if (!panel) return;

  panel.studies = panel.studies.filter(
    s => !(s.type === studyType && s.container === containerId)
  );

  renderStudyOverlay(panel);
  renderSeriesList();
}
```

### REQ-MV-031: Studies Do Not Consume Panel Slots

Studies are overlays on existing panels, tracked in `panel.studies[]`. The
5-panel limit applies only to chart panels, not study overlays:

```javascript
function canAddPanel() {
  return panels.length < maxPanels;  // Studies don't count
}
```

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

