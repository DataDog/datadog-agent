# Metrics Viewer - Technical Design

## Architecture Overview

Library + binary architecture enabling reuse:

- `src/metrics_viewer/` - Core library module
- `src/bin/fgm-viewer.rs` - CLI binary wrapper

```
src/metrics_viewer/
├── mod.rs           # Module exports
├── data.rs          # Parquet loading, metric discovery
├── server.rs        # HTTP server, API handlers
├── studies/
│   ├── mod.rs       # Study trait, registry
│   └── periodicity.rs  # Periodicity detection study
└── static/          # Embedded frontend assets
```

## REQ-MV-001: View Metrics Timeseries

### Frontend: uPlot

uPlot selected for canvas-based rendering with smooth pan/zoom on large datasets.
Lightweight (~40KB), plugin hooks for custom overlays, direct scale/series access.

### Static Asset Embedding

Frontend assets embedded in binary via `include_str!` or `rust-embed` crate.
Single binary distribution, no external file dependencies.

### Initial Display

On startup, load parquet file and serve HTTP on configurable port. Browser opens
automatically unless `--no-browser` flag. Empty chart shown until containers
selected.

## REQ-MV-002: Select Metrics to Display

### Metric Discovery

On parquet load, scan `metric_name` column to build unique metric list.
Cache metric names in `AppState` for fast `/api/metrics` responses.

### API: GET /api/metrics

Returns list of available metric names with sample counts:

```json
{
  "metrics": [
    {"name": "cgroup.v2.cpu.stat.usage_usec", "sample_count": 50000},
    {"name": "cgroup.v2.memory.current", "sample_count": 48000}
  ]
}
```

### Context Preservation

Frontend maintains current container selection and zoom range when metric
changes. Only timeseries data is re-fetched.

## REQ-MV-003: Filter Containers by Attributes

### Label Extraction

During parquet load, extract unique values for filter-relevant labels:
`qos_class`, `namespace`, `pod_name`. Store in `AppState` for filter dropdowns.

### API: GET /api/containers

Query params: `metric` (required), `qos_class`, `namespace` (optional filters).

Returns containers matching filters with summary stats:

```json
{
  "containers": [
    {"id": "abc123...", "short_id": "abc123", "qos_class": "Guaranteed",
     "namespace": "default", "avg": 45.2, "max": 98.1, "samples": 1000}
  ]
}
```

## REQ-MV-004: Search and Select Containers

### Frontend Implementation

Searchable multi-select list. Client-side filtering of container list by
name substring. "Top N" buttons trigger selection of first N containers
sorted by average value descending.

## REQ-MV-005: Zoom and Pan Through Time

### uPlot Configuration

Enable built-in zoom (drag select) and pan (shift+drag or touch). Scroll
wheel zoom centered on cursor. Configure via uPlot options:

```javascript
scales: { x: { time: true } },
cursor: { drag: { x: true, y: false } }
```

### Reset Button

Frontend reset button calls `uplot.setScale('x', {min, max})` with original
data bounds.

## REQ-MV-006: Navigate with Range Overview

### Implementation Options

1. **Second uPlot instance** - Synchronized mini chart showing full range
2. **Custom canvas overlay** - Draw simplified range indicator below main chart

Start with option 1 (simpler). Use uPlot's `setSeries` hooks to sync selection
rectangle with main chart zoom state.

## REQ-MV-007: Detect Periodic Patterns

### Study Abstraction

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

### Per-Container Study Initiation

Studies are initiated per-container via a study icon in the container list,
rather than a global button. This makes the action intentional and ensures
single-container focus.

Container list item with study action:
```
┌─────────────────────────────────────────┐
│ ☑ d7cd12621111  avg: 64.1  [📊]        │
│                             ^           │
│                     study icon button   │
└─────────────────────────────────────────┘
```

### Selection State Management

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

### Periodicity Detection Algorithm

Sliding window autocorrelation:

- Window size: 60 samples
- Step size: 30 samples (50% overlap)
- Period range: 2-30 seconds
- Detection threshold: periodicity score >= 0.6, amplitude >= 10%

Metrics returned per window: `period`, `periodicity_score`, `amplitude`.

### API: GET /api/studies

Returns available studies:

```json
{
  "studies": [
    {"id": "periodicity", "name": "Periodicity Study",
     "description": "Detects periodic patterns using autocorrelation"}
  ]
}
```

### API: GET /api/study/periodicity

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

## REQ-MV-008: Visualize Periodicity Patterns

### Results Panel

When periodicity study is active, the Studies section in the sidebar transforms
into a results panel:

```
┌─────────────────────────────┐
│ Periodicity Study     [✕]  │
│ Container: d7cd12621111    │
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

### Exit Study Flow

When user clicks [✕] or "Restore previous view":

1. Set `studyActive = false`
2. Clear periodicity overlay data
3. If restoring: select containers from `previousSelection`
4. If not restoring: keep current single-container selection
5. Preserve current time range (critical: do not reset zoom/pan)
6. Clear `previousSelection`

### Frontend Visualization Plugin

uPlot hooks API for custom drawing. Periodicity visualization plugin receives
`StudyResult` and draws overlays during `drawAxes` or `drawSeries` hooks.

### Periodicity Markers

Vertical dashed lines at period intervals within each detected window.
Lines colored to match associated container trace. Semi-transparent to
avoid obscuring data. Detected regions highlighted with subtle background
shading.

### Tooltip Interaction

Hover detection on periodicity markers and regions. Tooltip displays:
- Period duration (e.g., "10.2s period")
- Confidence score (e.g., "85% confidence")
- Amplitude (e.g., "±25.3 amplitude")

Tooltip positioned near cursor, auto-repositions to stay within chart bounds.

## REQ-MV-009: Automatic Y-Axis Scaling

### Implementation

uPlot configured with custom range function:
```javascript
y: { range: (u, min, max) => [0, max * 1.1] }
```

This automatically scales Y-axis to fit visible data (0 to max + 10% padding)
whenever the visible time range or displayed data changes. No manual button
required.

### Reset Behavior

When user clicks reset (full time range), Y-axis auto-scales to fit all data.

## REQ-MV-010: Graceful Empty Data Display

### Empty Data Detection

After building series arrays in `renderChart()`, check for valid data:

1. Count non-null values across all series
2. If all values are null → show "No data available" message
3. If all values are zero/constant → ensure minimum Y-axis range

### Y-Axis Minimum Range

When `yMax` is 0 or very small, use a fallback range to prevent chart collapse:

```javascript
// Ensure minimum Y-axis range for constant/zero data
if (yMax === 0) {
    yMax = 1;  // Fallback for all-zero data
}
originalYRange = [0, yMax * 1.1];
```

### Empty State Message

New variant of `showEmptyState()` for metrics with timestamps but no values:

```javascript
function showNoDataState(metricName) {
    // Shows: "No data recorded for [metric]"
    // Explains: metric exists but has no non-null values
}
```

## Data Flow Summary

```
Startup:
  load_parquet() -> AppState { metrics, containers_by_metric, timeseries_cache }

User flow:
  1. /api/metrics -> populate metric dropdown
  2. Select metric -> /api/containers?metric=X -> populate container list
  3. Select containers -> /api/timeseries?metric=X&containers=a,b,c -> render chart

Study flow:
  1. Click study icon on container Y
  2. Save current selection to previousSelection
  3. Deselect all, select Y -> /api/timeseries?metric=X&containers=Y -> render chart
  4. /api/study/periodicity?metric=X&container=Y -> display results panel + overlay
  5. Exit study -> restore previousSelection OR keep Y -> preserve time range
```
