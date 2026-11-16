# Observer Test Bench

Interactive UI for iterating on observer anomaly detection components.

## Architecture

```
Go Server (:8080)                 React UI (:5173)
┌──────────────────────┐         ┌──────────────────────┐
│ Load scenario        │         │ Select scenario      │
│ (parquet/logs/demo)  │  HTTP   │ Toggle detectors     │
│         ↓            │◄───────►│ Browse series tree   │
│ Run detectors        │  JSON   │ View charts + zoom   │
│ (BOCPD, scans)       │         │ Inspect anomalies    │
│         ↓            │         │ Click correlations   │
│ Detect correlations  │         └──────────────────────┘
└──────────────────────┘
```

**Key principle**: Deterministic. Same data + same code = same results. The observer acts as a queryable database—load a scenario, components compute results, UI displays them.

## Project Layout

```
internal/anomalydetection/testbench/
├── main.go              # Entry point, starts HTTP server
└── ui/
    ├── src/
    │   ├── App.tsx      # Main app, state management
    │   ├── api/client.ts
    │   ├── components/
    │   │   ├── SeriesTree.tsx          # Collapsible series selector
    │   │   ├── MetricsChart.tsx     # D3 chart with brush zoom
    │   │   └── ChartWithAnomalyDetails.tsx
    │   └── hooks/useObserver.ts        # API polling, reconnect
    └── package.json

internal/anomalydetection/testbench/bench/
├── bench.go             # Scenario loading and analysis orchestration
├── api.go               # REST API handlers
└── output.go            # Headless evaluation output
```

## Running

Build and launch the backend and UI through the repository task:

```bash
dda inv anomalydetection.launch-testbench --build
```

Use `--local --scenarios-dir /path/to/recordings` for existing recordings.
Open `http://127.0.0.1:5173` in your browser.

## UI controls

- The shared scenario picker leaves room for each view’s controls. **Reload** reruns
  the active recording. “Recorded” describes execution metadata, not detection accuracy.
- Metric inspection starts with the five recorded metric groups with the most hits
  from enabled detectors. Ranking counts raw detector hits; it does not identify a
  fault’s cause or use the EWMA scorer’s rolling-window count.
- **Inspect window** selects the full recording, Baseline, Disruption, Cooldown,
  or a dragged custom interval. Healthy controls offer their evaluation window.
  The rankings and hit counts update automatically; unavailable phases are disabled.
- **Drag a metric chart** to inspect a custom interval. Its purple band shows the
  inspected time while the chart keeps the surrounding context. **Zoom to window**
  zooms explicitly; **Show recording context** restores the full recording.
  In other tabs, dragging still zooms. Middle-drag or cmd-drag pans a zoomed chart.
- **Pin metric**, sidebar checkboxes, and bulk pin actions keep comparison metrics
  across reranking. Pins reserve slots; up to five additional ranked metrics appear
  initially. **Show more metrics** expands to a total of 50 including pins and
  disappears at capacity. Expansion persists across window changes. **Show fewer**
  restores five ranked metrics; **Show only pinned** hides automatic results.
- Metric-name search filters the pin list. Red **!** means detector hits in the
  inspected interval, not a loading or configuration error. Aggregation and metric
  type filters remain available. Tagged-series legends are searchable and scrollable.
- Anomaly lists start collapsed. Chart markers, including keyboard Enter/Space,
  open the matching details. With a separate inspection interval, list captions
  distinguish anomalies in the chart from those in the inspected window.
- Detector selections show **Updating…** until results refresh. Metric requests
  reuse cached data, fetch only displayed aggregations, and cancel obsolete work.
  Failed downloads offer **Retry metrics**.
- The scorer retains **Detection time** and **Change-point time** views. Bars show
  average active inputs per scorer second, separately from new detector hits.
  Input weights and episode severity have distinct legends. Configuration is read only.
  Severity intervals follow the selected time range, including severity carried in
  from before that range. Recording totals and First High remain explicitly scoped
  to the recording and current timing mode.
- Scorer contributors open the exact metric aggregation and tag set when uniquely
  identifiable, or source-filtered logs around onset. Ambiguous metric descriptors
  show **Series unavailable**. Temporary source focus preserves saved pins.
  **Back to scorer event** restores the previous time range and expanded event;
  **Resume metric inspection** returns to the previous recording context.

## Validation

```bash
npm run build
npm run test
```

The regression tests use Node’s native TypeScript support (Node 22.18 or later).
UI interactions are validated separately with Playwright against local recordings.
